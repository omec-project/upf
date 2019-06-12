#!/usr/bin/env python

import argparse
from os import system
import time
import signal
import sys

# for retrieving neighbor info
from pyroute2 import IPDB, IPRoute
from scapy.all import *

try:
    from pybess.bess import *
except ImportError:
    print('Cannot import the API module (pybess)')
    raise


class RouteEntry:
    def __init__(self):
        self.neighbor_ip = ' '
        self.local_ip = ' '
        self.iface = ' '
        self.iprange = ' '
        self.prefix_len = ' '

    def __str__(self):
        return ('{neigh: %s, local_ip: %s, iface: %s, ip-range: %s/%s}' %
                (self.neighbor_ip, self.local_ip, self.iface, self.iprange, self.prefix_len))


def mac2hex(mac):
    return int(mac.replace(':', ''), 16)


def send_ping(neighbor_ip):
    os.system('ping -c 1 ' + neighbor_ip + ' > /dev/null')


def send_arp(neighbor_ip, src_mac, iface):
    pkt = Ether(dst="ff:ff:ff:ff:ff:ff")/ARP(pdst=neighbor_ip, hwsrc=src_mac)
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
                print('Setting ip as ' + ip)
            if 'NDA_LLADDR' in att:
                # ('NDA_LLADDR', _mac)
                _mac = att[1]
                return _mac


def link_modules(server, module, next_module):
    print('Linking %s module' % next_module)
    # Connect module to next_module
    response = server.connect_modules(module, next_module)
    if response.error.code != 0:
        print('Error connecting module %s to %s' % (module, next_module))


def link_route_module(server, route_module, last_module, gateway_mac, iprange, prefix_len):
    print('Adding route entry ' + iprange + '/' + str(prefix_len) + ' for %s' % route_module)

    gateway_mac_str = '{:x}'.format(gateway_mac)
    _try = 0
    trial_limit = 5
    insert_success = 0
    while _try < trial_limit and insert_success == 0:
        # Pass routing entry to bessd's route module
        response = server.run_module_command(route_module,
                                             'add',
                                             'IPLookupCommandAddArg',
                                             {'prefix': iprange,
                                              'prefix_len': int(prefix_len),
                                              'gate': 0})
        if response.error.code != 0:
            print('Error inserting route entry for %s. Retrying...' % route_module)
            ++_try
            time.sleep(1)
        else:
            insert_success = 1

    if insert_success == 0:
        print('Addition failed! %s module may not exist' % route_module)
        return

    # Create Update module
    update_module = route_module + '_EthMac_' + gateway_mac_str
    response = server.create_module('Update',
                                    update_module,
                                    {'fields': [{'offset': 0, 'size': 6, 'value': gateway_mac}]})
    if response.error.code != 0:
        print('Error creating Update module %s' % update_module)
        return

    # Connect Update module to route module
    link_modules(server, route_module, update_module)

    # Connect Update module to dpdk_out module
    link_modules(server, update_module, last_module)


def probe_addr(local_ip, neighbor_ip, iface,
               iprange, prefix_len, src_mac):
    # Store entry if entry does not exist in ARP cache
    item = RouteEntry()
    item.neighbor_ip = neighbor_ip
    item.local_ip = local_ip
    item.iface = iface
    item.iprange = iprange
    item.prefix_len = prefix_len
    arpcache[item.neighbor_ip] = item
    print('Adding entry ' + str(item) + ' in arp probe table')

    # Probe ARP request by sending ping
    send_ping(item.neighbor_ip)

    # Probe ARP request
    ##send_arp(neighbor_ip, src_mac, item.iface)


def parse_new_route(msg):
    iface = {}
    iprange = {}
    gateway_mac = {}
    neighbor_ip = {}
    for att in msg['attrs']:
        if 'RTA_DST' in att:
            # Fetch IP range
            # ('RTA_DST', iprange)
            iprange = att[1]
        if 'RTA_GATEWAY' in att:
            # Fetch gateway MAC address
            # ('RTA_GATEWAY', neighbor_ip)
            neighbor_ip = att[1]
            _mac = fetch_mac(att[1])
            if not _mac:
                gateway_mac = 0
            else:
                gateway_mac = mac2hex(_mac)
        if 'RTA_OIF' in att:
            # Fetch interface name
            # ('RTA_OIF', iface)
            iface = ipdb.interfaces[int(att[1])].ifname

    if not iface in args.i or not iprange or not neighbor_ip:
        return

    # Fetch prefix_len
    prefix_len = msg['dst_len']

    # if mac is 0, send ARP request
    if gateway_mac == 0:
        for ipv4 in ipdb.interfaces[iface].ipaddr.ipv4:
            local_ip = ipv4[0]
            probe_addr(local_ip, neighbor_ip, iface,
                       iprange, prefix_len, ipdb.interfaces[iface].address)

    else:  # if gateway_mac is set
        print('Linking module ' + iface + '_routes' + ' with ' + iface +
              '_dpdk_po (Dest MAC: ' + str(_mac) + ').')
        # Pause bessd to avoid race condition (and potential crashes)
        bess.pause_all()

        link_route_module(bess, iface + "_routes", iface +
                          "_dpdk_po", gateway_mac, iprange, prefix_len)

        # Now resume bessd operations
        bess.resume_all()


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
        print('Linking module ' + item.iface + '_routes' + ' with ' + item.iface + '_dpdk_po (Dest MAC: ' +
              str(gateway_mac) + ').')

        # Pause bessd to avoid race condition (and potential crashes)
        bess.pause_all()

        link_route_module(bess, item.iface + "_routes", item.iface + "_dpdk_po",
                          mac2hex(gateway_mac), item.iprange, str(item.prefix_len))

        # Now resume bessd operations
        bess.resume_all()

        del arpcache[neighbor_ip]


# TODO - XXX: What if route is deleted. Need to add logic to de-link chained modules
def netlink_event_listener(ipdb, netlink_message, action):

    # If you get a netlink message, parse it
    msg = netlink_message

    if action == 'RTM_NEWROUTE':
        parse_new_route(msg)

    if action == 'RTM_NEWNEIGH':
        parse_new_neighbor(msg)


def boostrap_routes():
    _try = 0
    trial_limit = 5
    # Connect to BESS (assuming host=localhost, port=10514 (default))
    while not bess.is_connected() and _try < trial_limit:
        bess.disconnect()
        time.sleep(1)
        print('Connecting to BESS daemon...'),
        bess.connect(grpc_url=args.ip + ':' + args.port)
        ++_try

    if not bess.is_connected():
        print('BESS connection failure.')
        sys.exit()
    else:
        print('Done.')

    routes = ipr.get_routes()
    for i in routes:
        if i['event'] == 'RTM_NEWROUTE':
            parse_new_route(i)


def reconfigure(number, frame):
    print('Received: {} Reloading routes'.format(number))
    boostrap_routes()
    signal.pause()


def cleanup(number, frame):
    ipdb.unregister_callback(event_callback)
    print('Received: {} Exiting'.format(number))
    sys.exit()


def main():
    global arpcache, ipdb, event_callback, bess
    # for holding unresolved ARP queries
    arpcache = {}
    # for interacting with kernel
    ipdb = IPDB()
    # for bess client
    bess = BESS()

    # program current routes
    boostrap_routes()

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
    parser.add_argument('-i', type=str, nargs='+',
                        help='interface(s) to control')
    parser.add_argument(
        '--ip', type=str, default='localhost', help='BESSD address')
    parser.add_argument('--port', type=str, default='10514', help='BESSD port')

    # for holding command-line arguments
    global args
    args = parser.parse_args()

    if args.i:
        main()
    # if interface list is empty, print help menu and quit
    else:
        print(parser.print_help())
