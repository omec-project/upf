#!/usr/bin/env python

BESSD_HOST = 'localhost'
BESSD_PORT = '10514'
S1UDEV = 's1u'
SGIDEV = 'sgi'
# for retrieving arp records
import arpreq
# for retrieving route entries
import iptools
from pyroute2 import IPDB
# for pkt generation
#import scapy.all as scapy

try:
    from pybess.bess import *
except ImportError:
    print('Cannot import the API module (pybess)')
    raise

def mac2hex(mac):
    return int(mac.replace(':', ''), 16)

def link_modules(server, module, next_module):
    print('Linking %s module' % next_module)
    print(' ')
    # Connect module to next_module
    response = server.connect_modules(module, next_module)
    if response.error.code != 0:
        print('Error connecting module %s to %s' % (module, next_module))

def link_route_module(server, module, next_module, gateway_mac, prefix, prefix_len):
    print('Adding route entry for %s' % module)
    print(' ')
    # Pass routing entry to bessd's route module
    response = server.run_module_command(module,
                                         'add',
                                         'IPLookupCommandAddArg',
                                         {'prefix': prefix,
                                          'prefix_len': int(prefix_len),
                                          'gate': 0})
    if response.error.code != 0:
        print('Error inserting route entry for %s' % module)
                    
    # Create Update module
    response = server.create_module('Update',
                                    next_module,
                                    {'fields': [{'offset': 0, 'size': 6, 'value': gateway_mac}]})
    if response.error.code != 0:
        print('Error creating module %s' % next_module)
            
    # Connect Update module to route module
    link_modules(server, module, next_module)

def main():
    ipdb = IPDB()

    # Connect to BESS (assuming host=localhost, port=10514 (default))
    bess = BESS()
    bess.connect(grpc_url=BESSD_HOST + ':' + BESSD_PORT)

    # Pause bessd to avoid race condition (and potential crashes)
    bess.pause_all()

    for i in ipdb.routes:
        # For every gateway entry
        if iptools.ipv4.validate_cidr(i['dst']) and i['gateway'] and arpreq.arpreq(i['gateway']):
            # Get interface name
            iface = ipdb.interfaces[int(i['oif'])].ifname
            # Get prefix
            prefix = i['dst'].split('/')[0]
            # Get prefix length
            prefix_len = i['dst'].split('/')[1]
            # Get MAC address of the the gateway
            gateway_mac = mac2hex(arpreq.arpreq(i['gateway']))
            if iface == S1UDEV:
                link_route_module(bess, "s1u_routes", "s1u_dst_mac", gateway_mac, prefix, prefix_len)
                link_modules(bess, "s1u_dst_mac", "s1u_dpdk_po")
            if iface == SGIDEV:
                link_route_module(bess, "sgi_routes", "sgi_dst_mac", gateway_mac, prefix, prefix_len)
                link_modules(bess, "sgi_dst_mac", "sgi_dpdk_po")

    # Now resume bessd operations
    bess.resume_all()
    
if __name__ == '__main__':
    main()
