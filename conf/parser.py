# vim: syntax=py
# -*- mode: python -*-
# SPDX-License-Identifier: Apache-2.0
# Copyright(c) 2019 Intel Corporation

# for errnos
import errno
# for get_env
from conf.utils import *

# ====================================================
#       Parameters
# ====================================================


conf_file = get_env('CONF_FILE', 'conf/spgwu.json')
conf = get_json_conf(conf_file, False)

class Parser:
    def __init__(self, fname):
        self.name = get_env('CONF_FILE', fname)
        self.conf = get_json_conf(self.name, False)
        self.max_ip_defrag_flows = None
        self.ip_frag_with_eth_mtu = None
        self.measure = False
        self.mode = None
        self.enb_cidr = None
        self.ue_cide = None
        self.workers = 1
        self.max_sessions = None
        self.s1u_ifname = None
        self.sgi_ifname = None
        self.interfaces = dict()

    def parse(self, ifaces):
        # Maximum number of flows to manage ip4 frags for re-assembly
        try:
            self.max_ip_defrag_flows = int(conf["max_ip_defrag_flows"])
        except ValueError:
            print('Invalid value for max_ip_defrag_flows. Not installing IP4Defrag module.')
        except KeyError:
            print('max_ip_defrag_flows value not set. Not installing IP4Defrag module.')

        # Enable ip4 fragmentation
        try:
            self.ip_frag_with_eth_mtu = int(conf["ip_frag_with_eth_mtu"])
        except ValueError:
            print('Invalid value for ip_frag_with_eth_mtu. Not installing IP4Frag module.')
        except KeyError:
            print('ip_frag_with_eth_mtu value not set. Not installing IP4Frag module.')

        # Telemtrics
        # See this link for details:
        ## https://github.com/NetSys/bess/blob/master/bessctl/module_tests/timestamp.py
        try:
            self.measure = bool(conf["measure"])
        except ValueError:
            print('Invalid value for measure. Not installing Measure module.')
        except KeyError:
            print('measure value not set. Not installing Measure module.')

        # Fetch interfaces
        for iface in ifaces:
            self.interfaces[iface] = conf[iface]

        # Detect mode. Default is dpdk
        try:
            self.mode = conf["mode"]
        except KeyError:
            print('Autodetecting network driver')

        # CIDRs for UEs & enbs
        self.enb_cidr = conf["enb_cidr"]
        self.ue_cidr = conf["ue_cidr"]

        # Parse workers
        try:
            self.workers = int(conf["workers"])
        except ValueError:
            print('Invalid workers value! Re-setting # of workers to 1.')

        # Maximum number of sessions to manage
        try:
            self.max_sessions = int(conf["max_sessions"])
        except ValueError:
            print('Invalid max_sessions value!')

        # Interface names
        try:
            self.s1u_ifname = conf["s1u"]["ifname"]
            self.sgi_ifname = conf["sgi"]["ifname"]
        except KeyError:
            print('Can\'t parse interface name(s)!')
