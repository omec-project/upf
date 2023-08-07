#!/usr/bin/env python
# SPDX-License-Identifier: Apache-2.0
# Copyright 2019 Intel Corporation

import argparse
import ipaddress
import logging
import signal
import sys
import time
from collections import defaultdict
from dataclasses import dataclass, field
from threading import Lock, Thread
from typing import Dict, Optional

from pybess.bess import *  # type: ignore[import]  # noqa: F403
from pyroute2 import IPDB, IPRoute  # type: ignore[import]
from scapy.all import ICMP, IP, send  # type: ignore[import]

LOG_FORMAT = "%(asctime)s %(levelname)s %(message)s"
logging.basicConfig(format=LOG_FORMAT, level=logging.INFO)
logger = logging.getLogger(__name__)

DESTINATION_IP = "NDA_DST"
LINK_LAYER_ADDRESS = "NDA_LLADDR"
MAX_RETRIES = 5
SLEEP_S = 2
NEW_NEIGHBORS_ACTION = "RTM_NEWNEIGH"
DELETE_ROUTE_ACTION = "RTM_DELROUTE"
NEW_ROUTE_ACTION = "RTM_NEWROUTE"
INTERFACE_ATTR = "RTA_OIF"
MATCH_DESTINATION_ATTR = "RTA_DST"
GATEWAY_ATTR = "RTA_GATEWAY"
MATCH_DESTINATION_PREFIX_LEN_ATTR = "dst_len"
UPDATE_MODULE_CLASS = "Update"


@dataclass
class RouteEntry:
    """A representation of a neighbor in route entry."""

    next_hop_ip: str = field(default=None)  # type: ignore[assignment]
    interface: str = field(default=None)  # type: ignore[assignment]
    dest_prefix: str = field(default=None)  # type: ignore[assignment]
    prefix_len: int = field(default=0)
    route_count: int = field(default=0, repr=False)
    gate_idx: int = field(default=0, repr=False)
    macstr: str = field(default=None, repr=False)  # type: ignore[assignment]


class RouteEntryParser:
    """A parser that reads messages into RouteEntry objects."""

    def __init__(self, ipdb):
        self.ipdb = ipdb

    def parse(self, route_entry: dict) -> Optional[RouteEntry]:
        """Parses a route entry message.
        If the entry passes the checks, it is returned as a NeighborEntry.

        Args:
            route_entry (dict): A netlink route entry message.

        Returns:
            NeighborEntry: A neighbor entry.
        """
        attr_dict = dict(route_entry["attrs"])  # type: ignore[arg-type]

        if not attr_dict.get(INTERFACE_ATTR):
            return None
        interface_index = int(attr_dict.get(INTERFACE_ATTR))  # type: ignore[arg-type]  # noqa: E501
        interface = self.ipdb.interfaces[interface_index].ifname

        dest_prefix = None
        if route_entry.get(MATCH_DESTINATION_PREFIX_LEN_ATTR) == 0:
            dest_prefix = "0.0.0.0"

        if attr_dict.get(MATCH_DESTINATION_ATTR):
            dest_prefix = attr_dict.get(MATCH_DESTINATION_ATTR)

        if not dest_prefix:
            return None

        if not (next_hop_ip := attr_dict.get(GATEWAY_ATTR)):
            return None

        return RouteEntry(
            dest_prefix=dest_prefix,
            next_hop_ip=next_hop_ip,
            interface=interface,
            prefix_len=route_entry[MATCH_DESTINATION_PREFIX_LEN_ATTR],
        )


class BessController:
    def __init__(self, bess_ip, bess_port) -> None:
        self.bess = self.get_bess(ip=bess_ip, port=bess_port)

    def get_bess(self, ip: str, port: str) -> "BESS":  # type: ignore[name-defined]  # noqa: name 'BESS' may be undefined
        """Connects to the BESS daemon."""
        bess = BESS()  # type: ignore[name-defined]  # noqa: name 'BESS' may be undefined
        logger.info("Connecting to BESS daemon...")
        for _ in range(MAX_RETRIES):
            try:
                if not bess.is_connected():
                    bess.connect(grpc_url=ip + ":" + port)
            except BESS.RPCError:  # type: ignore[name-defined]  # noqa: name 'BESS' may be undefined
                logger.error(
                    "Error connecting to BESS daemon. Retrying in %s sec...",
                    SLEEP_S,
                )
                time.sleep(SLEEP_S)
            except Exception as e:
                logger.exception("Error connecting to BESS daemon")
                raise Exception("BESS connection failure.", e)
            else:
                logger.info("Connected to BESS daemon")
                return bess
        else:
            raise Exception(
                "BESS connection failure after {} attempts.".format(MAX_RETRIES)  # noqa: E501
            )

    def add_route_to_module(
        self, route_entry: RouteEntry, gate_idx: int, module_name: str
    ) -> None:
        """Adds a route entry to BESS.

        Args:
            route_entry (NeighborEntry): Entry to be added to BESS module.
            gate_idx (int): Gate of the module used in the route.
            module_name (str): The name of the module.
        """
        logger.info(
            "Adding route entry %s/%i for %s",
            route_entry.dest_prefix,
            route_entry.prefix_len,
            module_name,
        )
        for _ in range(MAX_RETRIES):
            try:
                self.bess.pause_all()
                self.bess.run_module_command(
                    module_name,
                    "add",
                    "IPLookupCommandAddArg",
                    {
                        "prefix": route_entry.dest_prefix,
                        "prefix_len": int(route_entry.prefix_len),
                        "gate": gate_idx,
                    },
                )
            except Exception:
                logger.exception(
                    "Error adding route entry %s/%i in %s. Retrying in %i sec...",  # noqa: E501
                    route_entry.dest_prefix,
                    route_entry.prefix_len,
                    module_name,
                    SLEEP_S,
                )
                time.sleep(SLEEP_S)
            else:
                logger.info(
                    "Route entry %s/%i added to %s",
                    route_entry.dest_prefix,
                    route_entry.prefix_len,
                    module_name,
                )
                break
            finally:
                self.bess.resume_all()
        else:
            raise Exception(
                "BESS route entry ({}/{}) insertion failure in module {}".format(  # noqa: E501
                    route_entry.dest_prefix,
                    route_entry.prefix_len,
                    module_name,
                )
            )

    def del_module_route_entry(self, route_entry: RouteEntry) -> None:
        """Deletes a route entry from BESS module.

        Args:
            neighbor (NeighborEntry): The neighbor entry.
        """
        route_module = route_entry.interface + "Routes"
        for _ in range(MAX_RETRIES):
            try:
                self.bess.pause_all()
                self.bess.run_module_command(
                    route_module,
                    "delete",
                    "IPLookupCommandDeleteArg",
                    {
                        "prefix": route_entry.dest_prefix,
                        "prefix_len": int(route_entry.prefix_len),
                    },
                )
            except Exception:
                logger.exception(
                    "Error deleting route entry for %s. Retrying in %i sec...",
                    route_module,
                    SLEEP_S,
                )
                time.sleep(SLEEP_S)
            else:
                break
            finally:
                self.bess.resume_all()
        else:
            raise Exception(
                "BESS route entry ({}/{}) deletion failure in module {}".format(  # noqa: E501
                    route_entry.dest_prefix,
                    route_entry.prefix_len,
                    route_module,
                )
            )

    def create_module(
        self, module_name: str, module_class: str, gateway_mac: str
    ) -> None:
        """Creates a BESS module.

        Args:
            gateway_mac (str): The MAC address of the gateway.
            update_module_name (str): The name of the module.
            module_class (str): The class of the module.
        """
        for _ in range(MAX_RETRIES):
            try:
                logger.info("Inserting %s", module_name)
                self.bess.pause_all()
                self.bess.create_module(
                    module_class,
                    module_name,
                    {"fields": [{"offset": 0, "size": 6, "value": gateway_mac}]},  # noqa: E501
                )
            except BESS.Error as e:  # type: ignore[name-defined]  # noqa: name 'BESS' may be undefined
                if e.code == errno.EEXIST:  # type: ignore[name-defined]  # noqa: name 'BESS' may be undefined
                    logger.error("Module %s already exists", module_name)
                    break
                else:
                    raise Exception(
                        "Unknown error when inserting {}: {}".format(
                            module_name, e
                        )
                    )
            except Exception:
                logger.exception(
                    "Error creating update module %s, retrying in %i secs",
                    module_name,
                    SLEEP_S,
                )
                time.sleep(SLEEP_S)
            else:
                logger.info("Add Update module %s successfully", module_name)
                break
            finally:
                self.bess.resume_all()
        else:
            raise Exception(
                "BESS module {} creation failure.".format(module_name)
            )

    def link_modules(self, module, next_module, ogate, igate) -> None:
        """Links two BESS modules together.

        Args:
            module (str): The name of the first module.
            next_module (str): The name of the second module.
            ogate (int, optional): The output gate of the first module.
            igate (int, optional): The input gate of the second module.
        """
        logger.info("Linking %s module to %s module", module, next_module)
        for _ in range(MAX_RETRIES):
            try:
                self.bess.pause_all()
                self.bess.connect_modules(module, next_module, ogate, igate)
            except BESS.Error as e:  # type: ignore[name-defined]  # noqa: name 'BESS' may be undefined
                logger.exception("Got BESS error")
                if e.code == errno.EBUSY:  # type: ignore[name-defined]  # noqa: name 'BESS' may be undefined
                    logger.error(
                        "Got code EBUSY. Retrying in %i secs...", SLEEP_S
                    )
                    time.sleep(SLEEP_S)
                else:
                    raise Exception(
                        "Unknown error when linking modules: {}".format(e)
                    )
            except Exception:
                logger.exception(
                    "Error connecting module: %s:%i->%i/%s. Retrying in %s secs...",  # noqa: E501
                    module,
                    ogate,
                    igate,
                    next_module,
                    SLEEP_S,
                )
                time.sleep(SLEEP_S)
            else:
                logger.info(
                    "Module %s:%i->%i/%s connected",
                    module,
                    ogate,
                    igate,
                    next_module,
                )
                break
            finally:
                self.bess.resume_all()
        else:
            raise Exception(
                "BESS module connection ({}:{}->{}:{}) failure.".format(
                    module, ogate, igate, next_module
                )
            )

    def del_module(self, module_name) -> None:
        """Deletes a BESS module.

        Args:
            update_module (str): The name of the module to delete.
        """
        for i in range(MAX_RETRIES):
            try:
                self.bess.pause_all()
                self.bess.destroy_module(module_name)
            except Exception:
                logger.exception(
                    "Error destroying module %s. Retrying in %i sec...",
                    module_name,
                    SLEEP_S,
                )
                time.sleep(SLEEP_S)
            else:
                logger.info("Module %s destroyed", module_name)
                logger.info("BESS resume all")
                break
            finally:
                self.bess.resume_all()
        else:
            raise Exception("Module {} deletion failure.".format(module_name))


class RouteController:
    def __init__(
        self,
        bess_controller: BessController,
        route_parser: RouteEntryParser,
        ipdb: IPDB,
        ipr: IPRoute,
    ):
        """
        Initializes the route controller.

        Args:
            bess_controller (BessController):
                Controller for BESS (Berkeley Extensible Software Switch).
            route_parser (RouteEntryParser): Parser for route entries.
            ipdb (IPDB): IP database to manage IP configurations.
            ipr (IPRoute): IP routing control object.

        Attributes:
            unresolved_arp_queries_cache (dict[str, RouteEntry]):
                A cache to store unresolved ARP queries.
            neighbor_cache (dict[str, RouteEntry]):
                A cache to keep track of entries add in Bess.
            module_gate_count_cache (Dict[str, int]):
                A cache for counting module gate occurrences.
        """
        self.unresolved_arp_queries_cache: Dict[str, RouteEntry] = {}
        self.neighbor_cache: Dict[str, RouteEntry] = {}
        self.module_gate_count_cache: Dict[str, int] = defaultdict(lambda: 0)

        self.lock = Lock()

        self.ipr = ipr
        self.ipdb = ipdb
        self.bess_controller = bess_controller
        self.route_parser = route_parser
        self.ping_missing = Thread(target=self._ping_missing_entries, daemon=True)  # noqa: E501
        self.event_callback = None

    def register_callbacks(self) -> None:
        """Register callback function."""
        self.event_callback = self.ipdb.register_callback(self._netlink_event_listener)  # noqa: E501

    def start_pinging_missing_entries(self) -> None:
        """Starts a new thread for ping missing entries."""
        if not self.ping_missing or not self.ping_missing.is_alive():
            self.ping_missing.start()

    def bootstrap_routes(self) -> None:
        """Goes through all routes and handles new ones."""
        routes = self.ipr.get_routes()
        for route in routes:
            if route["event"] == NEW_ROUTE_ACTION:
                if route_entry := self.route_parser.parse(route):
                    with self.lock:
                        self._handle_new_route_entry(route_entry)

    def _handle_new_route_entry(self, route_entry: RouteEntry) -> None:
        """Handles a new route entry.

        Args:
            route_entry (RouteEntry): The route entry.
        """
        if not (next_hop_mac := fetch_mac(self.ipr, route_entry.next_hop_ip)):
            logger.info(
                "mac address of the next hop %s is not stored in ARP table. Probing...",  # noqa: E501
                route_entry.next_hop_ip,
            )
            self._probe_addr(route_entry)
            return

        next_hop_mac_hex = mac_to_hex_int(next_hop_mac)
        self._add_neighbor(route_entry, next_hop_mac_hex)

    def _add_neighbor(self, route_entry: RouteEntry, next_hop_mac_hex) -> None:
        """Adds the route to next hop in BESS.
        Creates required BESS modules.

        Args:
            route_entry (RouteEntry)
            next_hop_mac_hex (str): The MAC address of the next hop.
        """
        route_module_name = route_entry.interface + "Routes"
        try:
            self.bess_controller.add_route_to_module(
                route_entry,
                self._get_gate_idx(route_entry, route_module_name),
                module_name=route_module_name,
            )
        except Exception:
            logger.exception(
                "Error adding route entry to BESS: %s", route_entry
            )
            return
        logger.info(
            "Trying to retrieve next hop entry %s from neighbor cache",
            route_entry.next_hop_ip,
        )
        if not self.neighbor_cache.get(route_entry.next_hop_ip):
            logger.info("Neighbor entry does not exist, creating modules.")
            self._create_module_links(next_hop_mac_hex, route_entry)
        else:
            logger.info("Neighbor already exists")

        self.neighbor_cache[route_entry.next_hop_ip].route_count += 1

    def _netlink_event_listener(
        self, netlink_message: dict, action: str
    ) -> None:
        """Listens for netlink events and handles them.

        Args:
            netlink_message (dict): The netlink message.
            action (str): The action.
        """
        route_entry = self.route_parser.parse(netlink_message)
        if action == NEW_ROUTE_ACTION and route_entry:
            logger.info("%s event received.", NEW_ROUTE_ACTION)
            with self.lock:
                self._handle_new_route_entry(route_entry)

        elif action == DELETE_ROUTE_ACTION and route_entry:
            logger.info("%s event received.", DELETE_ROUTE_ACTION)
            with self.lock:
                self._del_route(route_entry)

        elif action == NEW_NEIGHBORS_ACTION:
            logger.info("%s event received.", NEW_NEIGHBORS_ACTION)
            with self.lock:
                self._parse_new_neighbor(netlink_message)

    def _ping_missing_entries(self):
        """Pings missing entries every 10 seconds.
        The goal is to populate the ARP cache.
        If the target host does not respond it will be pinged again.
        """
        while True:
            with self.lock:
                missing_arp_entries = list(self.unresolved_arp_queries_cache.keys())  # noqa: E501
                logger.info("Missing ARP entries: %s", missing_arp_entries)
            for ip in missing_arp_entries:
                try:
                    logger.info("Pinging %s", ip)
                    send_ping(ip)
                except Exception as e:
                    logger.exception("Error when pinging %s: %s", ip, e)
            logger.info("Finished pinging missing ARP entries. Sleeping...")
            time.sleep(10)

    def _probe_addr(self, route_entry: RouteEntry) -> None:
        """Probes the MAC address of a neighbor.
        Pings the neighbor to trigger the update of the ARP table.

        Args:
            neighbor (NeighborEntry): The neighbor entry.
        """

        self.unresolved_arp_queries_cache[
            route_entry.next_hop_ip
        ] = route_entry
        logger.info("Adding entry %s in arp probe table", route_entry)
        if not validate_ipv4(route_entry.next_hop_ip):
            logger.error(
                "The IP address %s is invalid", route_entry.next_hop_ip
            )
            return
        logger.info("%s is a valid IPv4 address", route_entry.next_hop_ip)
        send_ping(route_entry.next_hop_ip)

    def _parse_new_neighbor(self, netlink_message: dict) -> None:
        """Handle new neighbor event.

        Args:
            netlink_message (dict): The netlink message.
        """
        attr_dict = dict(netlink_message["attrs"])
        unresolved_next_hop = self.unresolved_arp_queries_cache.get(
            attr_dict[DESTINATION_IP]
        )
        gateway_mac = attr_dict[LINK_LAYER_ADDRESS]
        if unresolved_next_hop:
            logger.info(
                "Linking module %sRoutes with %sMerge (Dest MAC: %s)",
                unresolved_next_hop.interface,
                unresolved_next_hop.interface,
                gateway_mac,
            )

            self._add_neighbor(
                unresolved_next_hop, mac_to_hex_int(gateway_mac)
            )

            del self.unresolved_arp_queries_cache[
                unresolved_next_hop.next_hop_ip
            ]  # noqa: E501

    def _get_gate_idx(self, route_entry: RouteEntry, module_name: str) -> int:
        """Get gate index for a route module.

        If the item is cached, return the cached gate index.
        If the item is new, increment the gate count
            and return the new gate index.

        Args:
            route_entry (RouteEntry)
            module_name (str): The name of the module.
        Returns:
            int: The gate index.
        """
        if (
            cached_entry := self.neighbor_cache.get(route_entry.next_hop_ip)
        ) is not None:  # noqa: E501
            return cached_entry.gate_idx
        return self.module_gate_count_cache[module_name]

    def _create_module_links(
        self, gateway_mac: str, route_entry: RouteEntry
    ) -> None:
        """Create update module and link modules.

        Args:
            gateway_mac (str): The MAC address of the gateway.
            neighbor (NeighborEntry): The neighbor entry.
        """
        route_module_name = route_entry.interface + "Routes"
        merge_module_name = route_entry.interface + "Merge"
        gateway_mac_hex = "{:X}".format(mac_to_hex_int(gateway_mac))
        update_module_name = route_module_name + "DstMAC" + gateway_mac_hex

        try:
            self.bess_controller.create_module(
                module_name=update_module_name,
                module_class=UPDATE_MODULE_CLASS,
                gateway_mac=gateway_mac,
            )
        except Exception:
            logger.exception(
                "Error creating update module %s", update_module_name
            )
            return

        gate_idx = self._get_gate_idx(route_entry, route_module_name)

        logger.info(
            "Linking module %s to module %s",
            update_module_name,
            route_module_name,
        )
        try:
            self.bess_controller.link_modules(
                route_module_name, update_module_name, gate_idx, 0
            )
        except Exception:
            logger.exception(
                "Error linking module % s to module % s",
                update_module_name,
                route_module_name,
            )
            return
        logger.info(
            "Linking module %s to module %s",
            update_module_name,
            merge_module_name,
        )
        try:
            self.bess_controller.link_modules(
                update_module_name, merge_module_name, 0, 0
            )
        except Exception:
            logger.exception(
                "Error linking module %s to module %s",
                update_module_name,
                merge_module_name,
            )
            return

        route_entry.gate_idx = gate_idx
        route_entry.macstr = gateway_mac_hex
        self.neighbor_cache[route_entry.next_hop_ip] = route_entry
        self.module_gate_count_cache[route_module_name] += 1

    def _del_route(self, route_entry: RouteEntry) -> None:
        """Deletes a route entry from BESS and the neighbor cache."""
        logger.info("Deleting route entry for %s", route_entry)
        next_hop = self.neighbor_cache.get(route_entry.next_hop_ip)

        if next_hop:
            try:
                self.bess_controller.del_module_route_entry(route_entry)
            except Exception:
                logger.exception(
                    "Error deleting route entry %s", route_entry
                )
                return

            next_hop.route_count -= 1

            if next_hop.route_count == 0:
                route_module = route_entry.interface + "Routes"
                update_module_name = route_module + "DstMAC" + next_hop.macstr

                try:
                    self.bess_controller.del_module(update_module_name)
                except Exception:
                    logger.exception(
                        "Error deleting update module %s",
                        update_module_name,
                    )
                    return

                logger.info("Module deleted %s", update_module_name)

                del self.neighbor_cache[route_entry.next_hop_ip]
                logger.info("Deleting item from neighbor cache")
            else:
                logger.info(
                    "Route count for %s decremented to %i",
                    route_entry.next_hop_ip,
                    next_hop.route_count,
                )
                self.neighbor_cache[route_entry.next_hop_ip] = next_hop
        else:
            logger.info("Neighbor %s does not exist", route_entry.next_hop_ip)

    def cleanup(self, number: int) -> None:
        """Unregisters the netlink event listener callback and exits."""
        self.ipdb.unregister_callback(self.event_callback)
        logger.info("Unregistered netlink event listener callback")
        logger.info("Received: %i Exiting", number)
        sys.exit()

    def reconfigure(self, number: int) -> None:
        """Reconfigures the route controller.
        Clears caches and bootstraps routes.
        """
        logger.info("Received: %i Reconfiguring", number)
        with self.lock:
            self.unresolved_arp_queries_cache.clear()
            self.neighbor_cache.clear()
            self.module_gate_count_cache.clear()
            self.bootstrap_routes()
        signal.pause()


def validate_ipv4(ip):
    """Validate the given IP address."""
    try:
        return isinstance(ipaddress.ip_address(ip), ipaddress.IPv4Address)
    except ValueError:
        return False


def send_ping(neighbor_ip):
    """Send an ICMP echo request to neighbor_ip.

    Does not wait for a response. Expected to have the side
    effect of populating the arp table entry for neighbor_ip.
    """
    logger.info("Sending ping to %s", neighbor_ip)
    send(IP(dst=neighbor_ip) / ICMP())


"""
# TODO, use it instead of ping
def send_arp_request(ip):
    # Create an ARP request packet to get the MAC address of the specified IP
    arp_packet = ARP(pdst=ip)
    # Broadcast the packet on the network
    ether_frame = Ether(dst="ff:ff:ff:ff:ff:ff")
    packet = ether_frame/arp_packet
    result = srp(packet, timeout=3, verbose=0)[0]

    # Get the MAC address from the response
    return result[0][1].hwsrc
"""


def fetch_mac(ipr: IPRoute, target_ip: str) -> Optional[str]:
    """Fetches the MAC address of the target IP from the ARP table.

    Args:
        ipr (IPRoute): The IPRoute object.
        target_ip (str): The target IP address.

    Returns:
        Optional[str]: The MAC address of the target IP.
    """
    neighbors = ipr.get_neighbours(dst=target_ip)
    for i in range(len(neighbors)):
        attrs = dict(neighbors[i]["attrs"])
        if attrs.get(DESTINATION_IP, "") == target_ip:
            return attrs.get(LINK_LAYER_ADDRESS, "")
    return None


def mac_to_hex_int(mac):
    try:
        return int(mac.replace(":", ""), 16)
    except ValueError:
        logger.error("Invalid MAC address: %s", mac)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(
        description="Basic IPv4 Routing Controller"
    )
    parser.add_argument(
        "-i", type=str, nargs="+", help="interface(s) to control"
    )
    parser.add_argument(
        "--ip", type=str, default="localhost", help="BESSD address"
    )
    parser.add_argument(
        "--port", type=str, default="10514", help="BESSD port"
    )

    args = parser.parse_args()

    if args.i:
        ipr = IPRoute()
        ipdb = IPDB()
        bess_controller = BessController(args.ip, args.port)
        route_parser = RouteEntryParser(ipdb=ipdb)
        controller = RouteController(
            bess_controller=bess_controller,
            route_parser=route_parser,
            ipdb=ipdb,
            ipr=ipr,
        )
        """Runs the route controller."""
        controller.bootstrap_routes()
        logger.info("Registering netlink event listener callback...")
        controller.register_callbacks()
        controller.start_pinging_missing_entries()
        logger.info("Ping missing entries thread started")
        logger.info("Registering signals...")
        signal.signal(
            signal.SIGHUP, lambda number, _: controller.reconfigure(number)
        )
        signal.signal(
            signal.SIGINT, lambda number, _: controller.cleanup(number)
        )
        signal.signal(
            signal.SIGTERM, lambda number, _: controller.cleanup(number)
        )
        logger.info("Sleep until a signal is received")
        signal.pause()
    else:
        parser.print_help()
