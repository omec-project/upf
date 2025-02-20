#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright 2019 Intel Corporation
# Copyright 2023 Canonical Ltd.

import argparse
import ipaddress
import logging
import signal
import sys
import time
from collections import defaultdict
from dataclasses import dataclass, field
from threading import Lock, Thread
from typing import Dict, List, Optional, Tuple
from pyroute2.netlink.rtnl.rtmsg import rtmsg
from pyroute2.netlink.rtnl.ndmsg import ndmsg
from pybess.bess import *
from pyroute2 import NDB, IPRoute
from scapy.all import ICMP, IP, send
from socket import AF_INET

LOG_FORMAT = "%(asctime)s %(levelname)s %(message)s"
logging.basicConfig(format=LOG_FORMAT, level=logging.INFO)
logger = logging.getLogger(__name__)

KEY_NETWORK_LAYER_DEST_ADDR = "NDA_DST"
KEY_LINK_LAYER_ADDRESS = "NDA_LLADDR"
KEY_NEW_NEIGHBOR_ACTION = "RTM_NEWNEIGH"
KEY_DELETE_ROUTE_ACTION = "RTM_DELROUTE"
KEY_NEW_ROUTE_ACTION = "RTM_NEWROUTE"
KEY_INTERFACE = "RTA_OIF"
KEY_DESTINATION_IP = "RTA_DST"
KEY_DESTINATION_GATEWAY_IP = "RTA_GATEWAY"
KEY_DESTINATION_PREFIX_LENGTH = "dst_len"


@dataclass
class RouteEntry:
    """A representation of a neighbor in route entry."""

    next_hop_ip: str = field(default=None)
    interface: str = field(default=None)
    dest_prefix: str = field(default=None)
    prefix_len: int = field(default=0)


@dataclass
class NeighborEntry:
    """A representation of a neighbor in neighbor cache."""

    gate_idx: int = field(default=0)
    mac_address: str = field(default=None)
    route_count: int = field(default=0)


class BessController:
    """Wraps commands from bess client."""

    MAX_RETRIES = 5
    SLEEP_S = 2

    def __init__(self, bess_ip: str, bess_port: str) -> None:
        """Initializes the BESS controller.

        Args:
            bess_ip (str): The IP address of the BESS daemon.
            bess_port (str): The port of the BESS daemon.
        """
        self._bess = self._get_bess(ip=bess_ip, port=bess_port)

    def _get_bess(self, ip: str, port: str) -> "BESS":
        """Connects to the BESS daemon."""
        bess = BESS()
        for _ in range(self.MAX_RETRIES):
            try:
                if not bess.is_connected():
                    bess.connect(grpc_url=ip + ":" + port)
            except BESS.RPCError:
                logger.error(
                    "Error connecting to BESS daemon. Retrying in %s sec...",
                    self.SLEEP_S,
                )
                time.sleep(self.SLEEP_S)
            except Exception as e:
                logger.exception("Error connecting to BESS daemon")
                raise Exception("BESS connection failure.", e)
            else:
                logger.info("Connected to BESS daemon")
                return bess
        else:
            raise Exception(
                "BESS connection failure after {} attempts.".format(self.MAX_RETRIES)
            )

    def add_route_to_module(
        self, route_entry: RouteEntry, gate_idx: int, module_name: str
    ) -> None:
        """Adds a route entry to BESS.

        Args:
            route_entry (RouteEntry): Entry to be added to BESS module.
            gate_idx (int): Gate of the module used in the route.
            module_name (str): The name of the module.
        """
        for _ in range(self.MAX_RETRIES):
            try:
                self._bess.pause_all()
                self._bess.run_module_command(
                    module_name,
                    "add",
                    "IPLookupCommandAddArg",
                    {
                        "prefix": route_entry.dest_prefix,
                        "prefix_len": route_entry.prefix_len,
                        "gate": gate_idx,
                    },
                )
            except Exception:
                logger.exception(
                    "Error adding route entry %s/%i in %s. Retrying in %i sec...",
                    route_entry.dest_prefix,
                    route_entry.prefix_len,
                    module_name,
                    self.SLEEP_S,
                )
                time.sleep(self.SLEEP_S)
            else:
                logger.info(
                    "Route entry %s/%i added to %s",
                    route_entry.dest_prefix,
                    route_entry.prefix_len,
                    module_name,
                )
                break
            finally:
                self._bess.resume_all()
        else:
            raise Exception(
                "BESS route entry ({}/{}) insertion failure in module {}".format(
                    route_entry.dest_prefix,
                    route_entry.prefix_len,
                    module_name,
                )
            )

    def delete_module_route_entry(self, route_entry: RouteEntry) -> None:
        """Deletes a route entry from BESS module.

        Args:
            route_entry (RouteEntry): The neighbor entry.
        """
        route_module = route_entry.interface + "Routes"
        for _ in range(self.MAX_RETRIES):
            try:
                self._bess.pause_all()
                self._bess.run_module_command(
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
                    self.SLEEP_S,
                )
                time.sleep(self.SLEEP_S)
            else:
                logger.info("Route entry deleted for %s", route_module)
                break
            finally:
                self._bess.resume_all()
        else:
            raise Exception(
                "BESS route entry ({}/{}) deletion failure in module {}".format(
                    route_entry.dest_prefix,
                    route_entry.prefix_len,
                    route_module,
                )
            )

    def create_module(
        self, module_name: str, module_class: str, gateway_mac: int
    ) -> None:
        """Creates a BESS module.

        Args:
            module_name (str): The name of the module.
            module_class (str): The class of the module.
            gateway_mac (int): MAC address of the gateway as an int.
        """
        for _ in range(self.MAX_RETRIES):
            try:
                self._bess.pause_all()
                self._bess.create_module(
                    module_class,
                    module_name,
                    {"fields": [{"offset": 0, "size": 6, "value": gateway_mac}]},
                )
            except BESS.Error as e:
                if e.code == errno.EEXIST:
                    logger.error("Module %s already exists", module_name)
                    break
                else:
                    raise Exception(
                        "Unknown error when inserting {}: {}".format(module_name, e)
                    )
            except Exception:
                logger.exception(
                    "Error creating update module %s, retrying in %i secs",
                    module_name,
                    self.SLEEP_S,
                )
                time.sleep(self.SLEEP_S)
            else:
                logger.info("Add Update module %s successfully", module_name)
                break
            finally:
                self._bess.resume_all()
        else:
            raise Exception("BESS module {} creation failure.".format(module_name))

    def link_modules(self, module, next_module, ogate, igate) -> None:
        """Links two BESS modules together.

        Args:
            module (str): The name of the first module.
            next_module (str): The name of the second module.
            ogate (int, optional): The output gate of the first module.
            igate (int, optional): The input gate of the second module.
        """
        for _ in range(self.MAX_RETRIES):
            try:
                self._bess.pause_all()
                self._bess.connect_modules(module, next_module, ogate, igate)
            except BESS.Error as e:
                logger.exception("Got BESS error")
                if e.code == errno.EBUSY:
                    logger.error("Got code EBUSY. Retrying in %i secs...", self.SLEEP_S)
                    time.sleep(self.SLEEP_S)
                else:
                    raise Exception("Unknown error when linking modules: {}".format(e))
            except Exception:
                logger.exception(
                    "Error linking module: %s:%i->%i/%s. Retrying in %s secs...",
                    module,
                    ogate,
                    igate,
                    next_module,
                    self.SLEEP_S,
                )
                time.sleep(self.SLEEP_S)
            else:
                logger.info(
                    "Module %s:%i->%i/%s linked",
                    module,
                    ogate,
                    igate,
                    next_module,
                )
                break
            finally:
                self._bess.resume_all()
        else:
            raise Exception(
                "BESS module connection ({}:{}->{}:{}) failure.".format(
                    module, ogate, igate, next_module
                )
            )

    def delete_module(self, module_name: str) -> None:
        """Deletes a BESS module.

        Args:
            module_name (str): The name of the module to delete.
        """
        for _ in range(self.MAX_RETRIES):
            try:
                self._bess.pause_all()
                self._bess.destroy_module(module_name)
            except Exception:
                logger.exception(
                    "Error destroying module %s. Retrying in %i sec...",
                    module_name,
                    self.SLEEP_S,
                )
                time.sleep(self.SLEEP_S)
            else:
                logger.info("Module %s destroyed", module_name)
                break
            finally:
                self._bess.resume_all()
        else:
            raise Exception("Module {} deletion failure.".format(module_name))


class RouteController:
    """Provides an interface to manage routes from netlink messages.

    Listens for netlink events and handling them.
    Creates BESS modules for route entries."""

    MAX_GATES = 8192

    def __init__(
        self,
        bess_controller: BessController,
        ndb: NDB,
        ipr: IPRoute,
        interfaces: List[str],
    ):
        """
        Initializes the route controller.

        Args:
            bess_controller (BessController):
                Controller for BESS (Berkeley Extensible Software Switch).
            ndb (NDB): database to manage Network configurations.
            ipr (IPRoute): IP routing control object.
            interfaces: list of interfaces

        Attributes:
            _unresolved_arp_queries_cache (dict[str, RouteEntry]):
                A cache to store unresolved ARP queries.
            _neighbor_cache (dict[str, RouteEntry]):
                A cache to keep track of entries add in Bess.
            _module_gate_count_cache (Dict[str, int]):
                A cache for counting module gate occurrences.
        """
        self._unresolved_arp_queries_cache: Dict[str, RouteEntry] = {}
        self._neighbor_cache: Dict[str, NeighborEntry] = {}
        self._module_gate_count_cache: Dict[str, int] = defaultdict(lambda: 0)

        self._lock = Lock()

        self._ndb = ndb
        self._ipr = ipr
        self._bess_controller = bess_controller
        self._ping_missing_thread = Thread(
            target=self._ping_missing_entries, daemon=True
        )
        self._interfaces = interfaces

    def register_handlers(self) -> None:
        """Register handler functions."""
        self._ndb.task_manager.register_handler(
            rtmsg,
            self._netlink_route_handler,
        )
        self._ndb.task_manager.register_handler(
            ndmsg,
            self._netlink_neighbor_handler,
        )
        logger.info("Registered netlink event handlers...")

    def start_pinging_missing_entries(self) -> None:
        """Starts a new thread for ping missing entries."""
        if not self._ping_missing_thread or not self._ping_missing_thread.is_alive():
            self._ping_missing_thread.start()
            logger.info("Ping missing entries thread started")

    def bootstrap_routes(self) -> None:
        """Goes through all routes and handles new ones."""
        routes = self._ipr.get_routes(family=AF_INET)
        for route in routes:
            if route["event"] == KEY_NEW_ROUTE_ACTION:
                if route_entry := self._parse_route_entry_msg(route):
                    with self._lock:
                        self.add_new_route_entry(route_entry)

    def add_new_route_entry(self, route_entry: RouteEntry) -> None:
        """Handles a new route entry.

        Args:
            route_entry (RouteEntry): The route entry.
        """
        if not validate_ipv4(route_entry.next_hop_ip):
            return
        if not (next_hop_mac := fetch_mac(self._ndb, route_entry.next_hop_ip)):
            logger.info(
                "mac address of the next hop %s is not stored in ARP table. Probing...",
                route_entry.next_hop_ip,
            )
            self._probe_addr(route_entry)
            return

        self._add_neighbor(route_entry, next_hop_mac)

    def _add_neighbor(self, route_entry: RouteEntry, next_hop_mac: str) -> None:
        """Adds the route in BESS module.

        Creates required BESS modules.

        Args:
            route_entry (RouteEntry)
            next_hop_mac (str): The MAC address of the next hop.
        """
        route_module_name = get_route_module_name(route_entry.interface)
        gate_idx = self._get_gate_idx(route_entry, route_module_name)
        try:
            self._bess_controller.add_route_to_module(
                route_entry,
                gate_idx=gate_idx,
                module_name=route_module_name,
            )

        except Exception:
            logger.exception("Error adding route entry to BESS: %s", route_entry)
            return

        if not self._neighbor_cache.get(route_entry.next_hop_ip):
            logger.info("Neighbor entry does not exist, creating modules.")
            update_module_name = get_update_module_name(
                route_entry.interface,
                next_hop_mac,
            )
            merge_module_name = get_merge_module_name(route_entry.interface)
            self._create_update_module(
                destination_mac=next_hop_mac,
                update_module_name=update_module_name,
            )
            self._create_module_links(
                gate_idx=gate_idx,
                update_module_name=update_module_name,
                route_module_name=route_module_name,
                merge_module_name=merge_module_name,
            )
            self._neighbor_cache[route_entry.next_hop_ip] = NeighborEntry(
                gate_idx=gate_idx,
                mac_address=next_hop_mac,
            )
            self._module_gate_count_cache[route_module_name] += 1
        else:
            logger.info("Neighbor already exists")

        self._neighbor_cache[route_entry.next_hop_ip].route_count += 1

    def _create_update_module(
        self,
        update_module_name: str,
        destination_mac: str,
    ) -> None:
        """Creates an update module in BESS.

        Args:
            update_module_name (str): The name of the module.
            destination_mac (str): The MAC address of the gateway.
        """
        try:
            mac_in_hexadecimal = mac_to_int(destination_mac)
            self._bess_controller.create_module(
                module_name=update_module_name,
                module_class="Update",
                gateway_mac=mac_in_hexadecimal,
            )
        except Exception:
            logger.exception("Error creating update module %s", update_module_name)
            return

    def add_unresolved_new_neighbor(self, netlink_message: dict) -> None:
        """Handle new neighbor event.

        It will add the neighbor if it was in the unresolved ARP queries cache.

        Args:
            netlink_message (dict): The netlink message.
        """
        attr_dict = dict(netlink_message["attrs"])
        route_entry = self._unresolved_arp_queries_cache.get(
            attr_dict[KEY_NETWORK_LAYER_DEST_ADDR]
        )
        gateway_mac = attr_dict[KEY_LINK_LAYER_ADDRESS]
        if route_entry:
            self._add_neighbor(route_entry, gateway_mac)
            del self._unresolved_arp_queries_cache[route_entry.next_hop_ip]

    def _create_module_links(
        self,
        gate_idx: int,
        update_module_name: str,
        route_module_name: str,
        merge_module_name: str,
    ) -> None:
        """Create update module and link modules.

        Args:
            gate_idx (int): Output gate index.
            update_module_name (str): The name of the update module.
            route_module_name (str): The name of the route module.
            merge_module_name (str): The name of the merge module.
        """
        try:
            self._bess_controller.link_modules(
                route_module_name, update_module_name, gate_idx, 0
            )
        except Exception:
            logger.exception(
                "Error linking module % s to module % s",
                update_module_name,
                route_module_name,
            )
            return

        try:
            self._bess_controller.link_modules(
                update_module_name, merge_module_name, 0, 0
            )
        except Exception:
            logger.exception(
                "Error linking module %s to module %s",
                update_module_name,
                merge_module_name,
            )
            return

    def delete_route_entry(self, route_entry: RouteEntry) -> None:
        """Deletes a route entry from BESS and the neighbor cache."""
        next_hop = self._neighbor_cache.get(route_entry.next_hop_ip)

        if next_hop:
            try:
                self._bess_controller.delete_module_route_entry(route_entry)
            except Exception:
                logger.exception("Error deleting route entry %s", route_entry)
                return

            next_hop.route_count -= 1

            if next_hop.route_count == 0:
                route_module = get_route_module_name(route_entry.interface)
                update_module_name = get_update_module_name(
                    route_module_name=route_module,
                    mac_address=next_hop.mac_address,
                )

                try:
                    self._bess_controller.delete_module(update_module_name)
                except Exception:
                    logger.exception(
                        "Error deleting update module %s",
                        update_module_name,
                    )
                    return

                logger.info("Module deleted %s", update_module_name)

                del self._neighbor_cache[route_entry.next_hop_ip]
                logger.info("Deleted item from neighbor cache")
            else:
                logger.info(
                    "Route count for %s decremented to %i",
                    route_entry.next_hop_ip,
                    next_hop.route_count,
                )
                self._neighbor_cache[route_entry.next_hop_ip] = next_hop
        else:
            logger.info("Neighbor %s does not exist", route_entry.next_hop_ip)

    def _ping_missing_entries(self):
        """Pings missing entries every 10 seconds.

        The goal is to populate the ARP cache.
        If the target host does not respond it will be pinged again.
        """
        while True:
            with self._lock:
                missing_arp_entries = list(self._unresolved_arp_queries_cache.keys())
                logger.info("Missing ARP entries: %s", missing_arp_entries)
            for ip in missing_arp_entries:
                try:
                    send_ping(ip)
                except Exception as e:
                    logger.exception("Error when pinging %s: %s", ip, e)
            logger.info("Finished pinging missing ARP entries. Sleeping...")
            time.sleep(10)

    def _probe_addr(self, route_entry: RouteEntry) -> None:
        """Probes the MAC address of a neighbor.
        Pings the neighbor to trigger the update of the ARP table.

        Args:
            route_entry (NeighborEntry): The neighbor entry.
        """
        self._unresolved_arp_queries_cache[route_entry.next_hop_ip] = route_entry
        logger.info("Adding entry %s in arp table by pinging", route_entry)
        send_ping(route_entry.next_hop_ip)

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
            cached_entry := self._neighbor_cache.get(route_entry.next_hop_ip)
        ) is not None:
            return cached_entry.gate_idx
        return self._module_gate_count_cache[module_name]

    def _netlink_neighbor_handler(self, _, netlink_message: dict) -> None:
        """Listens for netlink neighbor events and handles them.

        Args:
            _: target
            netlink_message (dict): The netlink message.
        """
        try:
            event = netlink_message.get("event")
        except Exception:
            logger.exception("Error parsing netlink message")
            return

        logger.info("%s netlink event received.", event)

        if event == KEY_NEW_NEIGHBOR_ACTION:
            with self._lock:
                self.add_unresolved_new_neighbor(netlink_message)

    def _netlink_route_handler(self, _, netlink_message: dict) -> None:
        """Listens for netlink route events and handles them.

        Args:
            _: target
            netlink_message (dict): The netlink message.
        """
        event = netlink_message.get("event")
        if event is None:
            logger.error("Netlink message does not include an event.")
            return

        logger.info("%s netlink event received.", event)
        route_entry = self._parse_route_entry_msg(netlink_message)
        if event == KEY_NEW_ROUTE_ACTION and route_entry:
            with self._lock:
                self.add_new_route_entry(route_entry)

        elif event == KEY_DELETE_ROUTE_ACTION and route_entry:
            with self._lock:
                self.delete_route_entry(route_entry)

    def cleanup(self, number: int) -> None:
        """Unregisters the netlink event handlers and exits."""
        logger.info("Received: %i Exiting", number)
        self._ndb.task_manager.unregister_handler(
            rtmsg,
            self._netlink_route_handler,
        )
        self._ndb.task_manager.unregister_handler(
            ndmsg,
            self._netlink_neighbor_handler,
        )
        logger.info("Unregistered netlink event listener callback")
        sys.exit()

    def reconfigure(self, number: int) -> None:
        """Reconfigures the route controller.
        Clears caches and bootstraps routes.
        """
        with self._lock:
            self._unresolved_arp_queries_cache.clear()
            self._neighbor_cache.clear()
            self._module_gate_count_cache.clear()
        self.bootstrap_routes()
        signal.pause()
        logger.info("Received: %i reconfigured", number)

    def _parse_route_entry_msg(self, route_entry: dict) -> Optional[RouteEntry]:
        """Parses a route entry message.
        If the entry passes the checks, it is returned as a RouteEntry object.

        Args:
            route_entry (dict): A netlink route entry message.

        Returns:
            RouteEntry: A route entry object.
        """
        try:
            attr_dict = dict(route_entry["attrs"])
        except (ValueError, KeyError):
            logger.exception("Error parsing route entry message")
            return None

        if not (next_hop_ip := attr_dict.get(KEY_DESTINATION_GATEWAY_IP)):
            return None

        if not attr_dict.get(KEY_INTERFACE):
            return None
        interface_index = int(attr_dict.get(KEY_INTERFACE))
        interface = self._ndb.interfaces[interface_index].get("ifname")
        if interface not in self._interfaces:
            return None

        dest_prefix = None
        if route_entry.get(KEY_DESTINATION_PREFIX_LENGTH) == 0:
            dest_prefix = "0.0.0.0"

        if attr_dict.get(KEY_DESTINATION_IP):
            dest_prefix = attr_dict.get(KEY_DESTINATION_IP)

        if not dest_prefix:
            return None

        return RouteEntry(
            dest_prefix=dest_prefix,
            next_hop_ip=next_hop_ip,
            interface=interface,
            prefix_len=route_entry[KEY_DESTINATION_PREFIX_LENGTH],
        )


def get_route_module_name(interface_name: str) -> str:
    """Returns the name of the route module.

    Args:
        interface_name (str): The name of the interface.
    """
    return interface_name + "Routes"


def get_update_module_name(route_module_name: str, mac_address: str) -> str:
    """Returns the name of the update module.

    Args:
        route_module_name (str): The name of the route module.
        mac_address (str): The MAC address of the gateway.
    """
    return route_module_name + "DstMAC" + mac_to_hex(mac_address)


def get_merge_module_name(interface_name: str) -> str:
    """Returns the name of the merge module.

    Args:
        interface_name (str): The name of the interface.
    """
    return interface_name + "Merge"


def validate_ipv4(ip: str) -> bool:
    """Validate the given IP address.

    Args:
        ip (str): The IP address to validate."""
    try:
        return isinstance(ipaddress.ip_address(ip), ipaddress.IPv4Address)
    except ValueError:
        logger.error("The IP address %s is invalid", ip)
        return False


def send_ping(neighbor_ip):
    """Send an ICMP echo request to neighbor_ip.

    Does not wait for a response. Expected to have the side
    effect of populating the arp table entry for neighbor_ip.
    """
    send(IP(dst=neighbor_ip) / ICMP())
    logger.info("Sent ping to %s", neighbor_ip)


def fetch_mac(ndb: NDB, target_ip: str) -> Optional[str]:
    """Fetches the MAC address of the target IP from the ARP table using NDB.

    Args:
        ndb (NDB): The NDB object.
        target_ip (str): The target IP address.

    Returns:
        Optional[str]: The MAC address of the target IP.
    """
    neighbors = ndb.neighbours.dump()
    for neighbor in neighbors:
        if neighbor["dst"] == target_ip:
            logger.info(
                "Mac address found for %s, Mac: %s",
                target_ip,
                neighbor["lladdr"],
            )
            return neighbor["lladdr"]
    logger.info("Mac address not found for %s", target_ip)
    return None


def mac_to_int(mac: str) -> int:
    """Converts a MAC address to an integer."""
    try:
        return int(mac.replace(":", ""), 16)
    except ValueError:
        raise ValueError("Invalid MAC address: %s", mac)


def mac_to_hex(mac: str) -> str:
    """Converts a MAC address to a hexadecimal string."""
    return "{:012X}".format(mac_to_int(mac))


def parse_args() -> Tuple[List[str], str, str]:
    parser = argparse.ArgumentParser(description="Basic IPv4 Routing Controller")
    parser.add_argument("-i", type=str, nargs="+", help="interface(s) to control")
    parser.add_argument("--ip", type=str, default="localhost", help="BESSD address")
    parser.add_argument("--port", type=str, default="10514", help="BESSD port")
    args = parser.parse_args()
    if not args.i:
        parser.print_help()
        raise ValueError("interface must be specified")
    return args.i, args.ip, args.port


def register_signal_handlers(controller: RouteController) -> None:
    """Register signal handlers for SIGHUP, SIGINT, SIGTERM.

    Args:
        controller (RouteController): The route controller.
    """
    signal.signal(signal.SIGHUP, lambda number, _: controller.reconfigure(number))
    signal.signal(signal.SIGINT, lambda number, _: controller.cleanup(number))
    signal.signal(signal.SIGTERM, lambda number, _: controller.cleanup(number))
    logger.info("Registered signals handlers.")


if __name__ == "__main__":
    interface_arg, ip_arg, port_arg = parse_args()
    ipr = IPRoute()
    ndb = NDB()
    bess_controller = BessController(ip_arg, port_arg)
    route_controller = RouteController(
        bess_controller=bess_controller,
        ndb=ndb,
        ipr=ipr,
        interfaces=interface_arg,
    )
    route_controller.bootstrap_routes()
    route_controller.register_handlers()
    route_controller.start_pinging_missing_entries()
    register_signal_handlers(controller=route_controller)
    logger.info("Sleep until a signal is received")
    signal.pause()
