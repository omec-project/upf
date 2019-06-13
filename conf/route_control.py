#!/usr/bin/env python

import argparse
#from os import system
import time
import signal
import sys

# for retrieving neighbor info
from pyroute2 import IPDB, IPRoute
# for sending ARP/ICMP pkts
from scapy.all import *

try:
    from pybess.bess import *
except ImportError:
    print('Cannot import the API module (pybess)')
    raise


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
    #system('ping -c 1 ' + neighbor_ip + ' > /dev/null')
    send(IP(dst=neighbor_ip)/ICMP())


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


def link_route_module(server, gateway_mac, item):
    iprange = item.iprange
    prefix_len = item.prefix_len
    route_module = item.iface + '_routes'
    last_module = item.iface + '_dpdk_po'
    gateway_mac_str = '{:x}'.format(gateway_mac)
    print('Adding route entry ' + iprange + '/' + str(prefix_len) + ' for %s' % route_module)

    print('Trying to retrieve neighbor entry ' + item.neighbor_ip + ' from neighbor cache')
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
    
    # Pass routing entry to bessd's route module
    response = server.run_module_command(route_module,
                                         'add',
                                         'IPLookupCommandAddArg',
                                         {'prefix': iprange,
                                          'prefix_len': int(prefix_len),
                                          'gate': gate_idx})
    if response.error.code != 0:
        print('Addition failed! %s module may not exist or route entry already added' % route_module)
        return

    if not neighbor_exists:
        print('Neighbor does not exist')
        # Create Update module
        update_module = route_module + '_EthMac_' + gateway_mac_str
        response = server.create_module('Update',
                                        update_module,
                                        {'fields': [{'offset': 0, 'size': 6, 'value': gateway_mac}]})
        if response.error.code != 0:
            print('Error creating Update module %s' % update_module)
            return

        print('Update module created')

        # Connect Update module to route module
        link_modules(server, route_module, update_module)

        # Connect Update module to dpdk_out module
        link_modules(server, update_module, last_module)

        # Add a new neighbor in neighbor cache
        neighborcache[item.neighbor_ip] = item

        # Add a record of the affliated gate id
        item.gate_idx = gate_idx

        # Set the mac str
        item.macstr = gateway_mac_str

        # Increment global gate count number
        modgatecnt[route_module] += 1

    else:
        print('Neighbor already exists')

    # Finally increment route count
    item.route_count += 1


def del_route_entry(server, item):
    iprange = item.iprange
    prefix_len = item.prefix_len
    route_module = item.iface + '_routes'
    last_module = item.iface + '_dpdk_po'

    neighbor_exists = neighborcache.get(item.neighbor_ip)
    if neighbor_exists:
        # Delete routing entry from bessd's route module
        response = server.run_module_command(route_module,
                                             'delete',
                                             'IPLookupCommandDeleteArg',
                                             {'prefix': iprange,
                                              'prefix_len': int(prefix_len)})
        if response.error.code != 0:
            print('Deletion failed! %s module may not exist or route entry does not exist' % route_module)
            return

        print('Route entry ' + iprange + '/' + str(prefix_len) + ' deleted from ' + route_module)
        neighbor_exists.route_count -= 1
        if neighbor_exists.route_count == 0:
            update_module = route_module + '_EthMac_' + neighbor_exists.macstr
            # if route count is 0, then delete the whole module
            response = server.destroy_module(update_module)
            if response.error.code != 0:
                print('Error deleting the Update module %s' % update_module)
                sys.exit()
            print('Module ' + update_module + ' destroyed')
            del neighborcache[item.neighbor_ip]
            print('Deleting item from neighborcache')
            del neighbor_exists
        else:
            print('Route count for ' + item.neighbor_ip +
                  ' decremented to ' + neighbor_exists.route_count)
            neighborcache[item.neighbor_ip] = neighbor_exists
    else:
        print('Neighbor ' + item.neighbor_ip + 'does not exist')


def probe_addr(item, src_mac):
    # Store entry if entry does not exist in ARP cache
    arpcache[item.neighbor_ip] = item
    print('Adding entry ' + str(item) + ' in arp probe table')

    # Probe ARP request by sending ping
    send_ping(item.neighbor_ip)

    # Probe ARP request
    ##send_arp(neighbor_ip, src_mac, item.iface)


def parse_new_route(msg):
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

    # Fetch prefix_len
    item.prefix_len = msg['dst_len']

    # if mac is 0, send ARP request
    if gateway_mac == 0:
        print('Adding entry ' + str(item.iface) + ' in arp probe table')
        probe_addr(item, ipdb.interfaces[item.iface].address)

    else:  # if gateway_mac is set
        print('Linking module ' + item.iface + '_routes' + ' with ' + item.iface +
              '_dpdk_po (Dest MAC: ' + str(_mac) + ').')
        # Pause bessd to avoid race condition (and potential crashes)
        bess.pause_all()

        link_route_module(bess, gateway_mac, item)

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

        # Add route entry, and add item in the registered neighbor cache
        link_route_module(bess, mac2hex(gateway_mac), item)

        # Now resume bessd operations
        bess.resume_all()

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

    # Pause bessd to avoid race condition (and potential crashes)
    bess.pause_all()

    del_route_entry(bess, item)

    # Now resume bessd operations
    bess.resume_all()

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


def boostrap_routes():
    routes = ipr.get_routes()
    for i in routes:
        if i['event'] == 'RTM_NEWROUTE':
            parse_new_route(i)

def connect_bessd():
    print('Connecting to BESS daemon...'),
    retries = 5
    # Connect to BESS (assuming host=localhost, port=10514 (default))
    while retries > 0:
        try:
            if not bess.is_connected():
                bess.connect(grpc_url=args.ip + ':' + args.port)
            break
        except BESS.RPCError:
            print('Error connecting to BESS daemon. Retrying in 1 sec...')
            retries -= 1
            time.sleep(1)
    
    if retries == 0:
        raise Exception('BESS connection failure.')
    else:
        print('Done.')    


def reconfigure(number, frame):
    print('Received: {} Reloading routes'.format(number))
    boostrap_routes()
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
