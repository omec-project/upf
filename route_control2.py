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
# for if_indextoname()
import ctypes
import ctypes.util

try:
    from pybess.bess import *
except ImportError:
    print('Cannot import the API module (pybess)')
    raise

libc = ctypes.CDLL(ctypes.util.find_library('c'))

def if_indextoname(index):
    if not isinstance (index, int):
        raise TypeError('index must be an int.')
    libc.if_indextoname.argtypes = [ctypes.c_uint32, ctypes.c_char_p]
    libc.if_indextoname.restype = ctypes.c_char_p

    ifname = ctypes.create_string_buffer (32)
    ifname = libc.if_indextoname (index, ifname)
    if not ifname:
        raise RuntimeError("Invalid Index")

    return ifname

def mac2hex(mac):
    return int(mac.replace(':', ''), 16)

def route_control():
    ipdb = IPDB()

    # Connect to BESS (assuming host=localhost, port=10514 (default))
    bess = BESS()
    bess.connect(grpc_url=BESSD_HOST+':'+BESSD_PORT)

    # Pause bessd to avoid race condition (and potential crashes)
    bess.pause_all()

    for i in ipdb.routes:        
        if iptools.ipv4.validate_cidr(i['dst']) and i['gateway']:
            if str(if_indextoname(i['oif'])).find(S1UDEV) >= 0:
                prefix = i['dst'].split('/')[0]
                prefix_len = i['dst'].split('/')[1]
                print('S1U')
                print(prefix)
                print(prefix_len)
                if arpreq.arpreq(i['gateway']):
                    print(arpreq.arpreq(i['gateway']))
                    print(mac2hex(arpreq.arpreq(i['gateway'])))
                    print(' ')
                    # Pass s1u routing entry to bessd's s1u_routes module
                    response = bess.run_module_command('s1u_routes',
                                                       'add',
                                                       'IPLookupCommandAddArg',
                                                       {'prefix': prefix,
                                                        'prefix_len': int(prefix_len),
                                                        'gate': 0})
                    if response.error.code != 0:
                        print('Error inserting s1u_routes')
                    
                    # Connect s1u_routes module to l3c
                    response = bess.connect_modules('l3c', 's1u_routes')
                    if response.error.code != 0:
                        print('Error connecting module s1u_routes with l3c')

                    
                    # Create Update module
                    response = bess.create_module('Update',
                                                  's1u_dst_mac',
                                                  {'fields': [{'offset': 0, 'size': 6, 'value': mac2hex(arpreq.arpreq(i['gateway']))}]}
                    				)
                    if response.error.code != 0:
                        print('Error creating module s1u_dst_mac')

                    # Connect Update module to s1u_routes
                    response = bess.connect_modules('s1u_routes', 's1u_dst_mac', 0, 0)
                    if response.error.code != 0:
                        print('Error connecting module s1u_routes with s1u_dst_mac')

                    # Connect s1u_dpdk_po to update module
                    response = bess.connect_modules('s1u_dst_mac', 's1u_dpdk_po')
                    if response.error.code != 0:
                        print('Error connecting module s1u_dst_mac to s1u_dpdk_po')
                    
            if str(if_indextoname(i['oif'])).find(SGIDEV) >= 0:
                prefix = i['dst'].split('/')[0]
                prefix_len = i['dst'].split('/')[1]
                print('SGI')
                print(prefix)
                print(prefix_len)
                if arpreq.arpreq(i['gateway']):
                    print(arpreq.arpreq(i['gateway']))
                    print(' ')
                    # Pass sgi routing entry to bessd's sgi_routes module
                    response = bess.run_module_command('sgi_routes',
                                                       'add',
                                                       'IPLookupCommandAddArg',
                                                       {'prefix': prefix,
                                                        'prefix_len': int(prefix_len),
                                                        'gate': 0})
                    if response.error.code != 0:
                        print('Error inserting sgi_route')

                    # Connect sgi_routes module to sgi_ether_encap
                    response = bess.connect_modules('sgi_ether_encap', 'sgi_routes')
                    if response.error.code != 0:
                        print('Error connecting module sgi_routes with sgi_ether_encap')

                    
                    # Create Update module
                    response = bess.create_module('Update',
                                                  'sgi_dst_mac',
                                                  {'fields': [{'offset': 0, 'size': 6, 'value': mac2hex(arpreq.arpreq(i['gateway']))}]}
                    				)
                    if response.error.code != 0:
                        print('Error creating module sgi_dst_mac')

                    # Connect Update module to s1u_routes
                    response = bess.connect_modules('sgi_routes', 'sgi_dst_mac', 0, 0)
                    if response.error.code != 0:
                        print('Error connecting module sgi_routes with sgi_dst_mac')

                    # Connect sgi_dpdk_po to update module
                    response = bess.connect_modules('sgi_dst_mac', 'sgi_dpdk_po')
                    if response.error.code != 0:
                        print('Error connecting module sgi_dst_mac to sgi_dpdk_po')

    # Now resume bessd operations
    bess.resume_all()

if __name__ == '__main__':
    route_control()
