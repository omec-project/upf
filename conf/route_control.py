#!/usr/bin/env python
# SPDX-License-Identifier: Apache-2.0
# Copyright 2019 Intel Corporation

import argparse
from collections import defaultdict
import ipaddress
import logging
import signal
import sys
import time

from dataclasses import dataclass, field
from threading import Lock, Thread
from typing import Optional, Dict

from pyroute2 import IPDB, IPRoute

from scapy.all import IP, ICMP, send

from pybess.bess import *

LOG_FORMAT = "%(asctime)s %(levelname)s %(message)s"
logging.basicConfig(format=LOG_FORMAT, level=logging.INFO)
logger = logging.getLogger(__name__)

MAX_RETRIES = 5
SLEEP_S = 2
NEW_ROUTE_ATTR = "RTM_NEWROUTE"
INTERFACE_ATTR = "RTA_OIF"
MATCH_DESTINATION_ATTR = "RTA_DST"
GATEWAY_ATTR = "RTA_GATEWAY"
MATCH_DESTINATION_PREFIX_LEN_ATTR = "dst_len"
UODATE_MODULE_CLASS = "Update"

@dataclass(frozen=True)
class RouteEntry:
    """A representation of a neighbor in route entry."""
    next_hop_ip: str = field(default=None)
    interface: str = field(default=None)
    dest_prefix: str = field(default=None)
    prefix_len: int = field(default=0)
    route_count: int = field(default=0, repr=False)
    gate_idx: int = field(default=0, repr=False)
    macstr: str = field(default=None, repr=False)

class RouteEntryParser:
    """A parser that converts route entry messages into NeighborEntry instances."""

    def __init__(self, ipdb):
        self.ipdb = ipdb

    def parse(self, route_entry: dict) -> RouteEntry:
        """Parses a route entry message.
        If the entry passes the checks, it is returned as a NeighborEntry.

        Args:
            route_entry (dict): A netlink route entry message.

        Returns:
            NeighborEntry: A neighbor entry.
        """
        attr_dict = dict(route_entry["attrs"])

        if not attr_dict.get(INTERFACE_ATTR):
            return
        interface = self.ipdb.interfaces[int(attr_dict.get(INTERFACE_ATTR))]

        if route_entry[MATCH_DESTINATION_PREFIX_LEN_ATTR] == 0:
            dest_prefix = "0.0.0.0"

        if attr_dict.get(MATCH_DESTINATION_ATTR):
            dest_prefix = attr_dict.get(MATCH_DESTINATION_ATTR)
        
        if not dest_prefix:
            return

        if not (next_hop_ip := attr_dict.get(GATEWAY_ATTR)):
            return

        return RouteEntry(
            dest_prefix=dest_prefix,
            next_hop_ip=next_hop_ip,
            interface=interface,
            prefix_len=route_entry[MATCH_DESTINATION_PREFIX_LEN_ATTR],
        )

class BessController:
    def __init__(self, bess_ip, bess_port) -> None:
        self.bess = self.get_bess(ip=bess_ip, port=bess_port)

    def get_bess(self, ip: str, port: str) -> BESS:
        """Connects to the BESS daemon."""
        bess = BESS()
        logger.info("Connecting to BESS daemon...")
        for _ in range(MAX_RETRIES):
            try:
                if not bess.is_connected():
                    bess.connect(grpc_url=ip + ":" + port)
            except BESS.RPCError:
                logger.error("Error connecting to BESS daemon. Retrying in {}sec...".format(SLEEP_S))
                time.sleep(SLEEP_S)
            except Exception as e:
                logger.error("Error connecting to BESS daemon: {}.".format(e))
                raise Exception("BESS connection failure.", e)
            else:
                logger.info("Connected to BESS daemon")
                return bess
        else:
            raise Exception("BESS connection failure after {} attempts.".format(MAX_RETRIES))

    def add_route_entry(self, route_entry: RouteEntry) -> None:
        """Adds a route entry to BESS.
        
        Args:
            neighbor (NeighborEntry): The neighbor entry to add in Bess.
        """
        route_module_name = route_entry.interface + 'Routes'
        logger.info(
            "Adding route entry {}/{} for {}".format(
                route_entry.dest_prefix, route_entry.prefix_len, route_module_name
            )
        )
        for _ in range(MAX_RETRIES):
            try:
                self.bess.pause_all()
                self.bess.run_module_command(
                    route_module_name,
                    "add",
                    "IPLookupCommandAddArg",
                    {
                        "prefix": route_entry.dest_prefix,
                        "prefix_len": int(route_entry.prefix_len),
                        "gate": self.get_gate_idx(route_module_name)
                    },
                )
            except Exception as e:
                logger.error(
                    "Error adding route entry {}/{} in {}. Retrying in {}sec...".format(
                        route_entry.dest_prefix, route_entry.prefix_len, route_module_name, SLEEP_S
                    )
                )
                logger.error(f"Exception was: {e}")
                time.sleep(SLEEP_S)
            else:
                logger.info(
                    "Route entry {}/{} added to {}".format(
                        route_entry.dest_prefix, route_entry.prefix_len, route_module_name
                    )
                )
                break
            finally:
                self.bess.resume_all()
        else:
            raise Exception(
                "BESS route entry ({}/{}) insertion failure in module {}".format(
                    route_entry.dest_prefix, route_entry.prefix_len, route_module_name
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
                    {"prefix": route_entry.dest_prefix, "prefix_len": int(route_entry.prefix_len)},
                )
            except:
                logger.error(
                    "Error deleting route entry for {}. Retrying in {} sec...".format(
                        route_module, SLEEP_S
                    )
                )
                time.sleep(SLEEP_S)
            else:
                break
            finally:
                self.bess.resume_all()
        else:
            raise Exception(
                "BESS route entry ({}/{}) deletion failure in module {}".format(
                    route_entry.dest_prefix, route_entry.prefix_len, route_module
                )
            )

    def create_module(self, module_name: str, module_class: str, gateway_mac: str) -> None:
        """Creates a BESS module.
        
        Args:
            gateway_mac (str): The MAC address of the gateway.
            update_module_name (str): The name of the module.
            module_class (str): The class of the module.
        """
        for _ in range(self.MAX_RETRIES):
            try:
                logger.info(f"Inserting {module_name}")
                self.bess.pause_all()
                self.bess.create_module(
                    module_class,
                    module_name,
                    {"fields": [{"offset": 0, "size": 6, "value": gateway_mac}]},
                )
            except BESS.Error as e:
                if e.code == errno.EEXIST:
                    logger.error(f"Module {module_name} already exists")
                    break
                else:
                    raise Exception("Unknown error when inserting {}: {}".format(module_name, e))
            except Exception as e:
                logger.error(
                    f"Error creating update module {module_name}: {e}. Retrying in {self.SLEEP_S} secs..."
                )
                time.sleep(self.SLEEP_S)
            else:
                logger.info(f"Add Update module {module_name} successfully")
                break
            finally:
                self.bess.resume_all()
        else:
            raise Exception('BESS module {} creation failure.'.format(module_name))

    def link_modules(self, module, next_module, ogate, igate) -> None:
        """Links two BESS modules together.

        Args:
            module (str): The name of the first module.
            next_module (str): The name of the second module.
            ogate (int, optional): The output gate of the first module.
            igate (int, optional): The input gate of the second module.
        """
        logger.info("Linking {} module to {} module".format(module, next_module))
        for _ in range(MAX_RETRIES):
            try:
                self.bess.pause_all()
                self.bess.connect_modules(module, next_module, ogate, igate)
            except BESS.Error as e:
                logger.error(f"Got BESS.Error with code {e.code}")
                if e.code == errno.EBUSY:
                    logger.error(
                        "Got code EBUSY. Retrying in {} secs...".format(
                            SLEEP_S
                        )
                    )
                    time.sleep(SLEEP_S)
                else:
                    logger.error(f"Got unknown code, returning: {e}")
                    raise Exception("Unknown error when linking modules: {}".format(e))
            except Exception as e:
                logger.error(
                    "Error connecting module {}:{}->{}:{}: {}. Retrying in {} secs...".format(
                        module, ogate, igate, next_module, e, SLEEP_S
                    )
                )
                time.sleep(SLEEP_S)
            else:
                logger.info(
                    "Module {}:{}->{}:{} connected".format(
                        module, ogate, igate, next_module
                    )
                )
                break
            finally:
                self.bess.resume_all()
        else:
            raise Exception(
                'BESS module connection ({}:{}->{}:{}) failure.'.format(
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
            except:
                logger.error(
                    "Error destroying module {}. Retrying in {}sec...".format(
                        module_name, SLEEP_S
                    )
                )
                time.sleep(self.SLEEP_S)
            else:
                logger.info("Module {} destroyed".format(module_name))
                logger.info("BESS resume all")
                break
            finally:
                self.bess.resume_all()
        else:
            raise Exception('Module {} deletion failure.'.format(module_name))

   
class RouteControl:
    def __init__(self, bess_ip, bess_port):
        """Initializes the route controller."""
        self.unresolved_arp_queries_cahce : dict[str, RouteEntry] = {}
        self.neighbor_cache : dict[str, RouteEntry] = {}
        self.module_gate_count_cache: Dict[str, int] = defaultdict(lambda: 0)

        self.lock = Lock()

        self.ipr = IPRoute()
        self.ipdb = IPDB()
        self.bess_controller = BessController()
        self.route_parser = RouteEntryParser(self.ipdb)

    def run(self):
        """Runs the route controller."""
        self.bootstrap_routes()

        logger.info("Registering netlink event listener callback...")
        self.event_callback = self.ipdb.register_callback(self.netlink_event_listener)

        self.ping_missing = Thread(target=self.ping_missing_entries, daemon=True)
        self.ping_missing.start()
        logger.info("Ping missing entries thread started")
    
    def bootstrap_routes(self) -> None:
        """Goes through all routes and handles new ones."""
        routes = self.ipr.get_routes()
        for route in routes:
            if route["event"] == NEW_ROUTE_ATTR:
                route_entry = self.route_parser.parse(route)
                self.handle_new_route_entry(route_entry)

    def handle_new_route_entry(self, route_entry: RouteEntry) -> None:
        """Handles a new route entry.

        Args:
            route_entry (RouteEntry): The route entry.
        """
        if not (next_hop_mac:= fetch_mac(route_entry.next_hop_ip)):
            logger.info(
                "mac address of the next hop {} is not stored in ARP table. Probing...".format(
                    route_entry.next_hop_ip
                )
            )
            self.probe_addr(
                route_entry, self.ipdb.interfaces[route_entry.interface].address
            )
            return

        next_hop_mac_hex = mac2hex(next_hop_mac)
        self.add_neighbor(route_entry, next_hop_mac_hex)

    def add_neighbor(self, route_entry: RouteEntry, next_hop_mac_hex) -> None:
        """Adds the route to next hop in BESS.
        Creates required BESS modules.

        Args:
            route_entry (RouteEntry)
            next_hop_mac_hex (str): The MAC address of the next hop.
        """
        try:
            self.bess_controller.add_route_entry(route_entry)
        except:
            logger.error("Error adding route entry to BESS: {}".format(route_entry))
            return

        with self.lock:
            logger.info(
                "Trying to retrieve next hop entry {} from neighbor cache".format(
                    route_entry.next_hop_ip
                )
            )
            if not self.neighbor_cache.get(route_entry.next_hop_ip):
                logger.info("Neighbor entry does not exist, creating modules.")
                self.create_module_links(next_hop_mac_hex, route_entry)
            else:
                logger.info("Neighbor already exists")

            self.neighbor_cache[route_entry.next_hop_ip].route_count += 1
    
    def netlink_event_listener(self, netlink_message: dict, action: str) -> None:
        """Listens for netlink events and handles them.
        
        Args:
            netlink_message (dict): The netlink message.
            action (str): The action.
        """
        route_entry = self.route_parser.parse(netlink_message)
        if action == NEW_ROUTE_ATTR:
            logger.info("{} event received.".format(NEW_ROUTE_ATTR))
            with self.lock:
                self.handle_new_route_entry(route_entry)

        elif action == "RTM_DELROUTE":
            logger.info("RTM_DELROUTE event received.")
            with self.lock:
                self.del_route(route_entry)

        elif action == "RTM_NEWNEIGH":
            logger.info("RTM_NEWNEIGH event received.")
            with self.lock:
                self.parse_new_neighbor(netlink_message)

    def ping_missing_entries(self):
        """Pings missing entries every 10 seconds.
        The goal is to populate the ARP cache.
        If the target host does not respond it will be pinged in the next cycle.
        """
        while True:
            with self.lock:
                missing_arp_entries = list(self.unresolved_arp_queries_cahce.keys())
                logger.info("Missing ARP entries: {}".format(missing_arp_entries))
            for ip in missing_arp_entries:
                try:
                    logger.info("Pinging {}".format(ip))
                    send_ping(ip)
                except Exception as e:
                    logger.error(f"Error when pinging {ip}: {e}")
            logger.info("Finished pinging missing ARP entries. Sleeping...")
            time.sleep(10)
    
    def probe_addr(self, route_entry: RouteEntry) -> None:
        """Probes the MAC address of a neighbor.
        Pings the neighbor to trigger the update of the ARP table.
        
        Args:
            neighbor (NeighborEntry): The neighbor entry.
        """
        with self.lock:
            self.unresolved_arp_queries_cahce[route_entry.neighbor_ip] = route_entry
        logger.info("Adding entry {} in arp probe table".format(route_entry))
        if not validate_ipv4(route_entry.next_hop_ip):
            logger.error(f"The IP address {route_entry.next_hop_ip} is invalid")
            return
        logger.info("{} is a valid IPv4 address".format(route_entry.next_hop_ip))
        send_ping(route_entry.next_hop_ip)

    def parse_new_neighbor(self, netlink_message: dict) -> None:
        """Handle new neighbor event.

        Args:
            netlink_message (dict): The netlink message.
        """
        attr_dict = dict(netlink_message["attrs"])
        unresolved_next_hop = self.unresolved_arp_queries_cahce.get(attr_dict["NDA_DST"])
        gateway_mac = attr_dict["NDA_LLADDR"]
        if unresolved_next_hop:
            logger.info(
                "Linking module {}Routes with {}Merge (Dest MAC: {})".format(
                    unresolved_next_hop.iface, unresolved_next_hop.iface, gateway_mac
                )
            )

            self.add_neighbor(unresolved_next_hop, mac2hex(gateway_mac))

            del self.unresolved_arp_queries_cahce[unresolved_next_hop.next_hop_ip]

    def get_gate_idx(self, route_entry: RouteEntry, module_name: str) -> int:
        """Get gate index for a route module.

        If the item is cached, return the cached gate index.
        If the item is new, increment the gate count and return the new gate index.

        Args:
            route_entry (RouteEntry)
            module_name (str): The name of the module.
        Returns:
            int: The gate index.
        """
        if (cached_entry := self.neighbor_cache.get(route_entry.next_hop_ip)) is not None:
            return cached_entry.gate_idx
        return self.module_gate_count_cache[module_name]

    def create_module_links(self, gateway_mac: str, route_entry: RouteEntry) -> None:
        """Create update module and link modules.

        Args:
            gateway_mac (str): The MAC address of the gateway.
            neighbor (NeighborEntry): The neighbor entry.    
        """
        route_module_name = route_entry.interface + "Routes"
        merge_module_name = route_entry.interface + "Merge"
        gateway_mac_hex = "{:X}".format(gateway_mac)
        update_module_name = route_module_name + "DstMAC" + gateway_mac_hex

        try:
            self.bess_controller.create_module(
                module_name=update_module_name,
                module_class=UODATE_MODULE_CLASS,
                gateway_mac=gateway_mac,
            )
        except:
            logger.error("Error creating update module {}".format(update_module_name))
            return

        gate_idx = self.get_gate_idx(route_entry, route_module_name)

        logger.info(
        "Linking module {} to module {}".format(update_module_name, route_module_name)
        )
        try:
            self.bess_controller.link_modules(route_module_name, update_module_name, gate_idx, 0)
        except:
            logger.error("Error linking module {} to module {}".format(update_module_name, route_module_name))
            return

        logger.info(
            "Linking module {} to module {}".format(update_module_name, merge_module_name)
        )
        try:
            self.bess_controller.link_modules(update_module_name, merge_module_name, 0, 0)
        except:
            logger.error("Error linking module {} to module {}".format(update_module_name, merge_module_name))
            return
        
        route_entry.gate_idx = gate_idx
        route_entry.macstr = gateway_mac_hex
        self.neighbor_cache[route_entry.next_hop_ip] = route_entry
        self.module_gate_count_cache[route_module_name] += 1

    def del_route(self, route_entry: RouteEntry) -> None:
        """Deletes a route entry from BESS and the neighbor cache."""
        logger.info("Deleting route entry for {}".format(route_entry))
        next_hop = self.neighbor_cache.get(route_entry.next_hop_ip)

        if next_hop:
            try:
                self.bess_controller.del_module_route_entry(route_entry)
            except:
                logger.error("Error deleting route entry {}".format(route_entry))
                return

            next_hop.route_count -= 1

            if next_hop.route_count == 0:
                route_module = route_entry.iface + "Routes"
                update_module_name = route_module + "DstMAC" + next_hop.macstr

                try:
                    self.bess_controller.del_module(update_module_name)
                except:
                    logger.error("Error deleting update module {}".format(update_module_name))
                    return

                logger.info("Module deleted {}".format(update_module_name))

                del self.neighbor_cache[route_entry.next_hop_ip]
                logger.info("Deleting item from neighbor cache")
            else:
                logger.info(
                    "Route count for {} decremented to {}".format(
                        route_entry.next_hop_ip, next_hop.route_count
                    )
                )
                self.neighbor_cache[route_entry.next_hop_ip] = next_hop
        else:
            logger.info("Neighbor {} does not exist".format(route_entry.next_hop_ip))

    def cleanup(self, number) -> None:
        """Unregisters the netlink event listener callback and exits."""
        self.ipdb.unregister_callback(self.event_callback)
        logger.info("Unregistered netlink event listener callback")
        logger.info("Received: {} Exiting".format(number))
        sys.exit()

    def reconfigure(self, number):
        """Reconfigures the route controller.
        Clears caches and bootstraps routes.
        """
        logger.info("Received: {} Reconfiguring".format(number))
        with self.lock:
            self.unresolved_arp_queries_cahce.clear()
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
    logger.info("Sending ping to {}".format(neighbor_ip))
    send(IP(dst=neighbor_ip) / ICMP())

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
        if attrs.get('NDA_DST', '') == target_ip:
            return attrs.get('NDA_LLADDR', '')
    return None

def mac2hex(mac):
    try:
        return int(mac.replace(':', ''), 16)
    except ValueError:
        logger.error('Invalid MAC address: %s', mac)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Basic IPv4 Routing Controller")
    parser.add_argument("-i", type=str, nargs="+", help="interface(s) to control")
    parser.add_argument("--ip", type=str, default="localhost", help="BESSD address")
    parser.add_argument("--port", type=str, default="10514", help="BESSD port")

    args = parser.parse_args()

    if args.i:
        controller = RouteControl(args.ip, args.port)
        controller.run()
        logger.info("Registering signals...")
        signal.signal(signal.SIGHUP, lambda number, _: controller.reconfigure(number))
        signal.signal(signal.SIGINT, lambda number, _: controller.cleanup(number))
        signal.signal(signal.SIGTERM, lambda number, _: controller.cleanup(number))
        logger.info("Sleep until a signal is received")
        signal.pause()
    else:
        parser.print_help()

# TODO Understand module creation and deletion
# TODO linting