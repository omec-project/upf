#!/usr/bin/env python

# for retrieving route entries
import netifaces
import iptools

try:
    from pybess.bess import *
except ImportError:
    print('Cannot import the API module (pybess)')
    raise

def route_control():
        # Retrieve ip address and netmask prefix for s1u
        s1u_addr_block = netifaces.ifaddresses('s1u')
        s1u_addr = s1u_addr_block[netifaces.AF_INET][0]['addr']
        print(s1u_addr)
        s1u_netmask = s1u_addr_block[netifaces.AF_INET][0]['netmask']
        print(s1u_netmask)
        print(iptools.ipv4.netmask2prefix(s1u_netmask))
        s1u_subnet = iptools.ipv4.subnet2block(s1u_addr + '/' + s1u_netmask)
        print(s1u_subnet[0])

        # Retrieve ip address and netmask prefix for sgi
        sgi_addr_block = netifaces.ifaddresses('sgi')
        sgi_addr = sgi_addr_block[netifaces.AF_INET][0]['addr']
        print(sgi_addr)
        sgi_netmask = sgi_addr_block[netifaces.AF_INET][0]['netmask']
        print(sgi_netmask)
        print(iptools.ipv4.netmask2prefix(sgi_netmask))
        sgi_subnet = iptools.ipv4.subnet2block(sgi_addr + '/' + sgi_netmask)
        print(sgi_subnet[0])

        # Connect to BESS (assuming host=localhost, port=10514 (default))
        bess = BESS()
        bess.connect(grpc_url='localhost:10514')

        # Pause bessd to avoid race condition (and potential crashes)
        bess.pause_all()

        # Pass s1u routing entry to bessd's s1u_routes module
        response = bess.run_module_command('s1u_routes',
                                           'add',
                                           'IPLookupCommandAddArg',
                                           {'prefix': s1u_subnet[0],
                                            'prefix_len': iptools.ipv4.netmask2prefix(s1u_netmask),
                                            'gate': 0})
        if response.error.code != 0:
            print('Error inserting s1u_route')

        # Pass sgi routing entry to bessd's sgi_routes module   
        response = bess.run_module_command('sgi_routes',
                                           'add',
                                           'IPLookupCommandAddArg',
                                           {'prefix': sgi_subnet[0],
                                            'prefix_len': iptools.ipv4.netmask2prefix(sgi_netmask),
                                            'gate': 0})
        if response.error.code != 0:
            print('Error inserting sgi_route')

        # Now resume bessd operations
        bess.resume_all()

if __name__ == '__main__':
    route_control()
