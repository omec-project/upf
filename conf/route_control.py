#!/usr/bin/env python
#----------------------------------------------------------------------------------#
BESSD_HOST = 'localhost'
BESSD_PORT = '10514'
S1UDEV = 's1u'
SGIDEV = 'sgi'
# for retrieving arp records
import arpreq
# for retrieving route entries
import iptools
from pyroute2 import IPDB
# for listening netlink events
from pyroute2 import IPRSocket
# for pkt generation
#import scapy.all as scapy
#----------------------------------------------------------------------------------#
try:
    from pybess.bess import *
except ImportError:
    print('Cannot import the API module (pybess)')
    raise
#----------------------------------------------------------------------------------#
def mac2hex(mac):
    return int(mac.replace(':', ''), 16)
#----------------------------------------------------------------------------------#
def link_modules(server, module, next_module):
    print('Linking %s module' % next_module)
    print(' ')
    # Connect module to next_module
    response = server.connect_modules(module, next_module)
    if response.error.code != 0:
        print('Error connecting module %s to %s' % (module, next_module))
#----------------------------------------------------------------------------------#
def link_route_module(server, module, last_module, gateway_mac, prefix, prefix_len):
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
        return
                    
    # Create Update module
    response = server.create_module('Update',
                                    module + '_EthMac_' + str(gateway_mac),
                                    {'fields': [{'offset': 0, 'size': 6, 'value': gateway_mac}]})
    if response.error.code != 0:
        print('Error creating module %s' % next_module)
        return
            
    # Connect Update module to route module
    link_modules(server, module, module + '_EthMac_' + str(gateway_mac))

    # Connect Update module to dpdk_out module
    link_modules(server, module + '_EthMac_' + str(gateway_mac), last_module)
#----------------------------------------------------------------------------------#
def netlink_event_listener(ipdb, bess):
    while 1:
        # Open up iproute socket
        ip = IPRSocket()
        ip.bind()
        message = ip.get()
        # If you get a netlink message, parse it
        for msg in message:
            if msg['event'] == 'RTM_NEWROUTE':
                for att in msg['attrs']:
                    if 'RTA_DST' in att:
                        # Fetch IP range
                        prefix = att[1]
                    if 'RTA_GATEWAY' in att:
                        # Fetch gateway MAC address
                        gateway_mac = mac2hex(arpreq.arpreq(att[1]))
                    if 'RTA_OIF' in att:
                        # Fetch interface name
                        iface = ipdb.interfaces[int(att[1])].ifname

                # Fetch prefix_len
                prefix_len = msg['dst_len']

                # Pause bessd to avoid race condition (and potential crashes)
                bess.pause_all()

                link_route_module(bess, iface + "_routes", iface + "_dpdk_po", gateway_mac, prefix, prefix_len)

                # Now resume bessd operations
                bess.resume_all()
        ip.close()
#----------------------------------------------------------------------------------#
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
                link_route_module(bess, "s1u_routes", "s1u_dpdk_po", gateway_mac, prefix, prefix_len)
            if iface == SGIDEV:
                link_route_module(bess, "sgi_routes", "sgi_dpdk_po", gateway_mac, prefix, prefix_len)

    # Now resume bessd operations
    bess.resume_all()

    # if no routing entries were added, start listening passively for netlink events
    netlink_event_listener(ipdb, bess)
#----------------------------------------------------------------------------------#
if __name__ == '__main__':
    main()
#----------------------------------------------------------------------------------#
