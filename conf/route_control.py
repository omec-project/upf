#!/usr/bin/env python
# SPDX-License-Identifier: Apache-2.0
# Copyright 2019 Intel Corporation

import argparse
import signal
import sys
import time
import ipaddress

# for retrieving neighbor info
from pyroute2 import IPDB, IPRoute

from scapy.all import *

try:
    from pybess.bess import *
except ImportError:
    print('Cannot import the API module (pybess)')
    raise

MAX_RETRIES = 5
SLEEP_S = 2


class NeighborEntry:
    def __init__(self):
        self.neighbor_ip = None
        self.iface = None
        self.iprange = None
        self.prefix_len = None
        self.route_count = 0
        self.gate_idx = 0
        self.macstr = None

    def __str__(self):
        return ('{neigh: %s, iface: %s, ip-range: %s/%s}' %
                (self.neighbor_ip, self.iface, self.iprange, self.prefix_len))


def mac2hex(mac):
    return int(mac.replace(':', ''), 16)


def send_ping(neighbor_ip):
    send(IP(dst=neighbor_ip) / ICMP())


def send_arp(neighbor_ip, src_mac, iface):
    pkt = Ether(dst="ff:ff:ff:ff:ff:ff") / ARP(pdst=neighbor_ip, hwsrc=src_mac)
    pkt.show()
    hexdump(pkt)
    sendp(pkt, iface=iface)


def fetch_mac(dip):
    ip = ''
    _mac = ''
    neighbors = ipr.get_neighbours(dst=dip)
    for i in range(len(neighbors)):
        for att in neighbors[i]['attrs']:
            if 'NDA_DST' in att and dip == att[1]:
                # ('NDA_DST', dip)
                ip = att[1]
            if 'NDA_LLADDR' in att:
                # ('NDA_LLADDR', _mac)
                _mac = att[1]
                return _mac


def link_modules(server, module, next_module, ogate=0, igate=0):
    print('Linking {} module'.format(next_module))

    # Pause bess first
    bess.pause_all()
    # Connect module to next_module
    for _ in range(MAX_RETRIES):
        try:
            server.connect_modules(module, next_module, ogate, igate)
        except BESS.Error as e:
            bess.resume_all()
            if e.code == errno.EBUSY:
                break
            else:
                return  #raise
        except Exception as e:
            print(
                'Error connecting module {}:{}->{}:{}: {}. Retrying in {} secs...'
                .format(module, ogate, igate, next_module, e, SLEEP_S))
            time.sleep(SLEEP_S)
        else:
            bess.resume_all()
            break
    else:
        bess.resume_all()
        print('BESS module connection ({}:{}->{}:{}) failure.'.format(
            module, ogate, igate, next_module))
        return
        #raise Exception('BESS module connection ({}:{}->{}:{}) failure.'.
        #                format(module, ogate, igate, next_module))


def link_route_module(server, gateway_mac, item):
    iprange = item.iprange
    prefix_len = item.prefix_len
    route_module = item.iface + 'Routes'
    last_module = item.iface + 'Merge'
    gateway_mac_str = '{:X}'.format(gateway_mac)
    print('Adding route entry {}/{} for {}'.format(iprange, prefix_len,
                                                   route_module))

    print('Trying to retrieve neighbor entry {} from neighbor cache'.format(
        item.neighbor_ip))
    neighbor_exists = neighborcache.get(item.neighbor_ip)

    # How many gates does this module have?
    # If entry does not exist, then initialize it
    if not modgatecnt.get(route_module):
        modgatecnt[route_module] = 0

    # Compute likely index
    if neighbor_exists:
        # No need to create a new Update module
        gate_idx = neighbor_exists.gate_idx
    else:
        # Need to create a new Update module,
        # so get gate_idx from gate count
        gate_idx = modgatecnt[route_module]

    # Pause bess first
    bess.pause_all()
    # Pass routing entry to bessd's route module
    for _ in range(MAX_RETRIES):
        try:
            server.run_module_command(route_module, 'add',
                                      'IPLookupCommandAddArg', {
                                          'prefix': iprange,
                                          'prefix_len': int(prefix_len),
                                          'gate': gate_idx
                                      })
        except:
            print('Error adding route entry {}/{} in {}. Retrying in {}sec...'.
                  format(iprange, prefix_len, route_module, SLEEP_S))
            time.sleep(SLEEP_S)
        else:
            bess.resume_all()
            break
    else:
        bess.resume_all()
        print('BESS route entry ({}/{}) insertion failure in module {}'.format(
            iprange, prefix_len, route_module))
        return
        #raise Exception('BESS route entry ({}/{}) insertion failure in module {}'.
        #                format(iprange, prefix_len, route_module))

    if not neighbor_exists:
        print('Neighbor does not exist')
        # Create Update module
        update_module = route_module + 'DstMAC' + gateway_mac_str

        # Pause bess first
        bess.pause_all()
        for _ in range(MAX_RETRIES):
            try:
                server.create_module('Update', update_module, {
                    'fields': [{
                        'offset': 0,
                        'size': 6,
                        'value': gateway_mac
                    }]
                })
            except BESS.Error as e:
                bess.resume_all()
                if e.code == errno.EEXIST:
                    break
                else:
                    return  #raise
            except Exception as e:
                print(
                    'Error creating update module {}: {}. Retrying in {} secs...'
                    .format(update_module, e, SLEEP_S))
                time.sleep(SLEEP_S)
            else:
                bess.resume_all()
                break
        else:
            bess.resume_all()
            print('BESS module {} creation failure.'.format(update_module))
            return  #raise Exception('BESS module {} creation failure.'.
            #        format(update_module))

        print('Update module created')

        # Connect Update module to route module
        link_modules(server, route_module, update_module, gate_idx, 0)

        # Connect Update module to dpdk_out module
        link_modules(server, update_module, last_module, 0, 0)

        # Add a new neighbor in neighbor cache
        neighborcache[item.neighbor_ip] = item

        # Add a record of the affliated gate id
        item.gate_idx = gate_idx

        # Set the mac str
        item.macstr = gateway_mac_str

        # Increment global gate count number
        modgatecnt[route_module] += 1

        neighbor_exists = item

    else:
        print('Neighbor already exists')

    # Finally increment route count
    neighborcache[item.neighbor_ip].route_count += 1


def del_route_entry(server, item):
    iprange = item.iprange
    prefix_len = item.prefix_len
    route_module = item.iface + 'Routes'

    neighbor_exists = neighborcache.get(item.neighbor_ip)
    if neighbor_exists:
        # Pause bess first
        bess.pause_all()
        # Delete routing entry from bessd's route module
        for i in range(MAX_RETRIES):
            try:
                server.run_module_command(route_module, 'delete',
                                          'IPLookupCommandDeleteArg', {
                                              'prefix': iprange,
                                              'prefix_len': int(prefix_len)
                                          })
            except:
                print(
                    'Error while deleting route entry for {}. Retrying in {} sec...'
                    .format(route_module, SLEEP_S))
                time.sleep(SLEEP_S)
            else:
                bess.resume_all()
                break
        else:
            bess.resume_all()
            print('Route entry deletion failure.')
            return
            #raise Exception('Route entry deletion failure.')

        print('Route entry {}/{} deleted from {}'.format(
            iprange, prefix_len, route_module))

        # Decrementing route count for the registered neighbor
        neighbor_exists.route_count -= 1

        # If route count is 0, then delete the whole module
        if neighbor_exists.route_count == 0:
            update_module = route_module + 'DstMAC' + neighbor_exists.macstr
            # Pause bess first
            bess.pause_all()
            for i in range(MAX_RETRIES):
                try:
                    server.destroy_module(update_module)
                except:
                    print('Error destroying module {}. Retrying in {}sec...'.
                          format(update_module, SLEEP_S))
                    time.sleep(SLEEP_S)
                else:
                    bess.resume_all()
                    break
            else:
                bess.resume_all()
                print('Module {} deletion failure.'.format(update_module))
                return
                #raise Exception('Module {} deletion failure.'.
                #                format(update_module))

            print('Module {} destroyed'.format(update_module))

            # Delete entry from the neighbor cache
            del neighborcache[item.neighbor_ip]
            print('Deleting item from neighborcache')
            del neighbor_exists
        else:
            print('Route count for {}  decremented to {}'.format(
                item.neighbor_ip, neighbor_exists.route_count))
            neighborcache[item.neighbor_ip] = neighbor_exists
    else:
        print('Neighbor {} does not exist'.format(item.neighbor_ip))


def probe_addr(item, src_mac):
    # Store entry if entry does not exist in ARP cache
    arpcache[item.neighbor_ip] = item
    print('Adding entry {} in arp probe table'.format(item))

    try:
        ipb = ipaddress.ip_address(item.neighbor_ip)
        if isinstance(ipb, ipaddress.IPv4Address):
          print("The IP address {} is valid ipv4 address".format(ipb))
        else:
          print("The IP address {} is valid ipv6 address. Ignore ".format(ipb))
          return
    except:
        print("The IP address {} is invalid".format(item.neighbor_ip))
        return
    # Probe ARP request by sending ping
    send_ping(item.neighbor_ip)

    # Probe ARP request
    ##send_arp(neighbor_ip, src_mac, item.iface)


def parse_new_route(msg):
    item = NeighborEntry()
    # Fetch prefix_len
    item.prefix_len = msg['dst_len']
    # Default route
    if item.prefix_len == 0:
        item.iprange = '0.0.0.0'

    for att in msg['attrs']:
        if 'RTA_DST' in att:
            # Fetch IP range
            # ('RTA_DST', iprange)
            item.iprange = att[1]
        if 'RTA_GATEWAY' in att:
            # Fetch gateway MAC address
            # ('RTA_GATEWAY', neighbor_ip)
            item.neighbor_ip = att[1]
            _mac = fetch_mac(att[1])
            if not _mac:
                gateway_mac = 0
            else:
                gateway_mac = mac2hex(_mac)
        if 'RTA_OIF' in att:
            # Fetch interface name
            # ('RTA_OIF', iface)
            item.iface = ipdb.interfaces[int(att[1])].ifname

    if not item.iface in args.i or not item.iprange or not item.neighbor_ip:
        # Neighbor info is invalid
        del item
        return

    # if mac is 0, send ARP request
    if gateway_mac == 0:
        print('Adding entry {} in arp probe table. Neighbor: {}'.format(item.iface,item.neighbor_ip))
        probe_addr(item, ipdb.interfaces[item.iface].address)

    else:  # if gateway_mac is set
        print('Linking module {}Routes with {}Merge (Dest MAC: {})'.format(
            item.iface, item.iface, _mac))

        link_route_module(bess, gateway_mac, item)


def parse_new_neighbor(msg):
    for att in msg['attrs']:
        if 'NDA_DST' in att:
            # ('NDA_DST', neighbor_ip)
            neighbor_ip = att[1]
        if 'NDA_LLADDR' in att:
            # ('NDA_LLADDR', neighbor_mac)
            gateway_mac = att[1]

    item = arpcache.get(neighbor_ip)
    if item:
        print('Linking module {}Routes with {}Merge (Dest MAC: {})'.format(
            item.iface, item.iface, gateway_mac))

        # Add route entry, and add item in the registered neighbor cache
        link_route_module(bess, mac2hex(gateway_mac), item)

        # Remove entry from unresolved arp cache
        del arpcache[neighbor_ip]


def parse_del_route(msg):
    item = NeighborEntry()
    for att in msg['attrs']:
        if 'RTA_DST' in att:
            # Fetch IP range
            # ('RTA_DST', iprange)
            item.iprange = att[1]
        if 'RTA_GATEWAY' in att:
            # Fetch gateway MAC address
            # ('RTA_GATEWAY', neighbor_ip)
            item.neighbor_ip = att[1]
        if 'RTA_OIF' in att:
            # Fetch interface name
            # ('RTA_OIF', iface)
            item.iface = ipdb.interfaces[int(att[1])].ifname

    if not item.iface in args.i or not item.iprange or not item.neighbor_ip:
        # Neighbor info is invalid
        del item
        return

    # Fetch prefix_len
    item.prefix_len = msg['dst_len']

    del_route_entry(bess, item)

    # Delete item
    del item


def netlink_event_listener(ipdb, netlink_message, action):

    # If you get a netlink message, parse it
    msg = netlink_message

    if action == 'RTM_NEWROUTE':
        parse_new_route(msg)

    if action == 'RTM_NEWNEIGH':
        parse_new_neighbor(msg)

    if action == 'RTM_DELROUTE':
        parse_del_route(msg)


def bootstrap_routes():
    routes = ipr.get_routes()
    for i in routes:
        if i['event'] == 'RTM_NEWROUTE':
            parse_new_route(i)


def connect_bessd():
    print('Connecting to BESS daemon...'),
    # Connect to BESS (assuming host=localhost, port=10514 (default))
    for i in range(MAX_RETRIES):
        try:
            if not bess.is_connected():
                bess.connect(grpc_url=args.ip + ':' + args.port)
        except BESS.RPCError:
            print(
                'Error connecting to BESS daemon. Retrying in {}sec...'.format(
                    SLEEP_S))
            time.sleep(SLEEP_S)
        else:
            break
    else:
        raise Exception('BESS connection failure.')

    print('Done.')


def reconfigure(number, frame):
    print('Received: {} Reloading routes'.format(number))
    # clear arpcache
    for ip in list(arpcache):
        item = arpcache.get(ip)
        del item
    arpcache.clear()
    for ip in list(neighborcache):
        item = neighborcache.get(ip)
        del item
    neighborcache.clear()
    for modname in list(modgatecnt):
        item = modgatecnt.get(modname)
        del item
    modgatecnt.clear()
    bootstrap_routes()
    signal.pause()


def cleanup(number, frame):
    ipdb.unregister_callback(event_callback)
    print('Received: {} Exiting'.format(number))
    sys.exit()


def main():
    global arpcache, neighborcache, modgatecnt, ipdb, event_callback, bess, ipr
    # for holding unresolved ARP queries
    arpcache = {}
    # for holding list of registered neighbors
    neighborcache = {}
    # for holding gate count per route module
    modgatecnt = {}
    # for interacting with kernel
    ipdb = IPDB()
    ipr = IPRoute()
    # for bess client
    bess = BESS()

    # connect to bessd
    connect_bessd()

    # program current routes
    bootstrap_routes()

    # listen for netlink events
    print('Registering netlink event listener callback...'),
    event_callback = ipdb.register_callback(netlink_event_listener)
    print('Done.')

    signal.signal(signal.SIGHUP, reconfigure)
    signal.signal(signal.SIGINT, cleanup)
    signal.signal(signal.SIGTERM, cleanup)
    signal.pause()


if __name__ == '__main__':
    parser = argparse.ArgumentParser(
        description='Basic IPv4 Routing Controller')
    parser.add_argument('-i',
                        type=str,
                        nargs='+',
                        help='interface(s) to control')
    parser.add_argument('--ip',
                        type=str,
                        default='localhost',
                        help='BESSD address')
    parser.add_argument('--port', type=str, default='10514', help='BESSD port')

    # for holding command-line arguments
    global args
    args = parser.parse_args()

    if args.i:
        main()
    # if interface list is empty, print help menu and quit
    else:
        print(parser.print_help())
