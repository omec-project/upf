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

def add_s1u_route_entry(server, gateway_mac, prefix, prefix_len):
    print('Adding entry for S1U')
    print(' ')
    # Pass s1u routing entry to bessd's s1u_routes module
    response = server.run_module_command('s1u_routes',
                                         'add',
                                         'IPLookupCommandAddArg',
                                         {'prefix': prefix,
                                          'prefix_len': int(prefix_len),
                                          'gate': 0})
    if response.error.code != 0:
        print('Error inserting s1u_routes')
                    
    # Connect s1u_routes module to l3c
    response = server.connect_modules('l3c', 's1u_routes')
    if response.error.code != 0:
        print('Error connecting module s1u_routes with l3c')

                    
    # Create Update module
    response = server.create_module('Update',
                                    's1u_dst_mac',
                                    {'fields': [{'offset': 0, 'size': 6, 'value': gateway_mac}]})
    if response.error.code != 0:
        print('Error creating module s1u_dst_mac')
            
    # Connect Update module to s1u_routes
    response = server.connect_modules('s1u_routes', 's1u_dst_mac', 0, 0)
    if response.error.code != 0:
        print('Error connecting module s1u_routes with s1u_dst_mac')

    # Connect s1u_dpdk_po to update module
    response = server.connect_modules('s1u_dst_mac', 's1u_dpdk_po')
    if response.error.code != 0:
        print('Error connecting module s1u_dst_mac to s1u_dpdk_po')

def add_sgi_route_entry(server, gateway_mac, prefix, prefix_len):
    print('Adding entry for SGI')
    print(' ')
    # Pass sgi routing entry to bessd's sgi_routes module
    response = server.run_module_command('sgi_routes',
                                         'add',
                                         'IPLookupCommandAddArg',
                                         {'prefix': prefix,
                                          'prefix_len': int(prefix_len),
                                          'gate': 0})
    if response.error.code != 0:
        print('Error inserting sgi_route')

    # Connect sgi_routes module to sgi_ether_encap
    response = server.connect_modules('sgi_ether_encap', 'sgi_routes')
    if response.error.code != 0:
        print('Error connecting module sgi_routes with sgi_ether_encap')

                    
    # Create Update module
    response = server.create_module('Update',
                                    'sgi_dst_mac',
                                    {'fields': [{'offset': 0, 'size': 6, 'value': gateway_mac}]})
    if response.error.code != 0:
        print('Error creating module sgi_dst_mac')
            
    # Connect Update module to s1u_routes
    response = server.connect_modules('sgi_routes', 'sgi_dst_mac', 0, 0)
    if response.error.code != 0:
        print('Error connecting module sgi_routes with sgi_dst_mac')
                
    # Connect sgi_dpdk_po to update module
    response = server.connect_modules('sgi_dst_mac', 'sgi_dpdk_po')
    if response.error.code != 0:
        print('Error connecting module sgi_dst_mac to sgi_dpdk_po')

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
                add_s1u_route_entry(bess, gateway_mac, prefix, prefix_len)
            if iface == SGIDEV:
                add_sgi_route_entry(bess, gateway_mac, prefix, prefix_len)

    # Now resume bessd operations
    bess.resume_all()
    
if __name__ == '__main__':
    main()
