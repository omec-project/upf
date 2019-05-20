#!/usr/bin/env python

BESSD_HOST = 'localhost'
BESSD_PORT = '10514'
# for retrieving route entries
import netifaces
import iptools

try:
    from pybess.bess import *
except ImportError:
    print('Cannot import the API module (pybess)')
    raise

def route_control():

    for i in netifaces.interfaces():
        addr_block = netifaces.ifaddresses(i)
        if netifaces.AF_INET in addr_block:
            for j in range(len(addr_block[netifaces.AF_INET])):
                addr = addr_block[netifaces.AF_INET][j]['addr']
                print(addr)
                netmask = addr_block[netifaces.AF_INET][j]['netmask']
                print(iptools.ipv4.netmask2prefix(netmask))
                subnet = iptools.ipv4.subnet2block(addr + '/' + netmask)

                # Connect to BESS (assuming host=localhost, port=10514 (default))
                bess = BESS()
                bess.connect(grpc_url=BESSD_HOST+':'+BESSD_PORT)

                # Pause bessd to avoid race condition (and potential crashes)
                bess.pause_all()

                # Pass s1u routing entry to bessd's s1u_routes module
                response = bess.run_module_command('s1u_routes',
                                                   'add',
                                                   'IPLookupCommandAddArg',
                                                   {'prefix': subnet[0],
                                                    'prefix_len': iptools.ipv4.netmask2prefix(netmask),
                                                    'gate': 0})
                if response.error.code != 0:
                    print('Error inserting s1u_route')

                # Pass sgi routing entry to bessd's sgi_routes module
                response = bess.run_module_command('sgi_routes',
                                                   'add',
                                                   'IPLookupCommandAddArg',
                                                   {'prefix': subnet[0],
                                                    'prefix_len': iptools.ipv4.netmask2prefix(netmask),
                                                    'gate': 0})
                if response.error.code != 0:
                    print('Error inserting sgi_route')

                # Now resume bessd operations
                bess.resume_all()

if __name__ == '__main__':
    route_control()
