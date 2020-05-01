#!/usr/bin/env python
# SPDX-License-Identifier: Apache-2.0
# Copyright(c) 2019 Intel Corporation

import os
import signal
import socket
import sys

import iptools
import json
import psutil
from pyroute2 import IPDB


def exit(code, msg):
    print(msg)
    sys.exit(code)


def getpid(process_name):
    for proc in psutil.process_iter(attrs=['pid', 'name']):
        if process_name == proc.info['name']:
            return proc.info['pid']


def getpythonpid(process_name):
    for proc in psutil.process_iter(attrs=['pid', 'cmdline']):
        if len(proc.info['cmdline']) < 2:
            continue
        if process_name in proc.info['cmdline'][1] and 'python' in proc.info['cmdline'][0]:
            return proc.info['pid']
    return


def get_json_conf(path, dump):
    conf = json.loads(open(path).read())
    if dump:
        print(json.dumps(conf, indent=4, sort_keys=True))
    return conf


def get_env(varname, default=None):
    try:
        var = os.environ[varname]
        return var
    except KeyError:
        if default is not None:
            return '{}'.format(default)
        else:
            exit(1, 'Empty env var {}'.format(varname))


def ips_by_interface(name):
    ipdb = IPDB()
    return [ipobj[0] for ipobj in ipdb.interfaces[name]['ipaddr'].ipv4]


def alias_by_interface(name):
    ipdb = IPDB()
    return ipdb.interfaces[name]['ifalias']


def mac_by_interface(name):
    ipdb = IPDB()
    return ipdb.interfaces[name]['address']


def mac2hex(mac):
    return long(mac.replace(':', ''), 16)


def peer_by_interface(name):
    ipdb = IPDB()
    try:
        peer_idx = ipdb.interfaces[name]['link']
        peer_name = ipdb.interfaces[peer_idx]['ifname']
    except:
        raise Exception('veth interface {} does not exist'.format(name))
    else:
        return peer_name


def aton(ip):
    return socket.inet_aton(ip)


def validate_cidr(cidr):
    return iptools.ipv4.validate_cidr(cidr)


def cidr2mask(cidr):
    _, prefix = cidr.split('/')
    return format(0xffffffff << (32 - int(prefix)), '08x')


def cidr2block(cidr):
    return iptools.ipv4.cidr2block(cidr)


def ip2hex(ip):
    return iptools.ipv4.ip2hex(ip)


def ip2long(ip):
    return iptools.ipv4.ip2long(ip)


def get_process_affinity():
    return psutil.Process().cpu_affinity()
