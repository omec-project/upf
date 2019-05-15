#!/usr/bin/env python

from __future__ import print_function
from __future__ import absolute_import
import sys
import os
import os.path
import io
import tempfile
import time
import cli
import commands
# for retrieving route entries
import netifaces
import iptools

try:
    this_dir = os.path.dirname(os.path.realpath(__file__))
    sys.path.insert(1, os.path.join(this_dir, '..'))
    from pybess.bess import *
except ImportError:
    print('Cannot import the API module (pybess)', file=sys.stderr)
    raise

def main():
    # Retrieve ip address and netmask prefix for each interface
    s1u_addr_block = netifaces.ifaddresses('s1u')
    s1u_addr = s1u_addr_block[netifaces.AF_INET][0]['addr']
    print(s1u_addr)
    s1u_netmask = s1u_addr_block[netifaces.AF_INET][0]['netmask']
    print(s1u_netmask)
    print(iptools.ipv4.netmask2prefix(s1u_netmask))

    sgi_addr_block = netifaces.ifaddresses('sgi')
    sgi_addr = sgi_addr_block[netifaces.AF_INET][0]['addr']
    print(sgi_addr)
    sgi_netmask = s1u_addr_block[netifaces.AF_INET][0]['netmask']
    print(s1u_netmask)
    print(iptools.ipv4.netmask2prefix(sgi_netmask))

    s = BESS()
    s.connect(grpc_url='localhost:10514')

    response = client.run_module_command('s1u_routes',
                                         'add',
                                         'IPLookupCommandAddArg',
                                         {'prefix': s1u_addr,
                                          'prefix_len': iptools.ipv4.netmask2prefix(s1u_netmask),
                                          'gate': 0})
    asserEqual(0, response.error.code)
    
    response = client.run_module_command('sgi_routes',
                                         'add',
                                         'IPLookupCommandAddArg',
                                         {'prefix': sgi_addr,
                                          'prefix_len': iptools.ipv4.netmask2prefix(sgi_netmask),
                                          'gate': 0})
    asserEqual(0, response.error.code)

if __name__ == '__main__':
    main()
