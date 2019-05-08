import os
import socket
import sys

import iptools
import psutil
from pyroute2 import IPDB


def exit(code, msg):
    print(msg)
    sys.exit(code)


def get_env(varname):
    try:
        var = os.environ[varname]
    except KeyError:
        exit(1, 'Empty env var {}'.format(varname))
    else:
        return var


def ips_by_interface(name):
    ipdb = IPDB()
    return [ipobj[0] for ipobj in ipdb.interfaces[name]['ipaddr'].ipv4]


def mac_by_interface(name):
    ipdb = IPDB()
    return ipdb.interfaces[name]['address']


def mac2hex(mac):
    return int(mac.replace(':', ''), 16)


def peer_by_interface(name):
    ipdb = IPDB()
    try:
        peer_idx = ipdb.interfaces[name]['link']
        peer_name = ipdb.interfaces[peer_idx]['ifname']
    except:
        exit(2, 'veth interface {} does not exist'.format(name))
    else:
        return peer_name


def aton(ip):
    return socket.inet_aton(ip)


def validate_subnet(subnet):
    return iptools.ipv4.validate_subnet(subnet)


def ip2hex(subnet):
    return iptools.ipv4.ip2hex(subnet)


def get_process_affinity():
    return psutil.Process().cpu_affinity()
