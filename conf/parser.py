#!/usr/bin/env python
# SPDX-License-Identifier: Apache-2.0
# Copyright(c) 2019 Intel Corporation

# for errnos
import errno
# for get_env
from conf.utils import *

# how many times should controller try to connect before giving up
MAX_RETRIES = 5
# Sleep these many seconds before trying to reconnect
SLEEP_S = 2
# Maximum number of gates per module instance in BESS. Don't change it.
MAX_GATES = 8192


# ====================================================
#       Parameters
# ====================================================


class Parser:
    def __init__(self, fname):
        self.name = get_env('CONF_FILE', fname)
        self.conf = get_json_conf(self.name, False)
        self.max_ip_defrag_flows = None
        self.ip_frag_with_eth_mtu = None
        self.measure = False
        self.mode = None
        self.sim_start_ue_ip = None
        self.sim_start_enb_ip = None
        self.sim_start_teid = None
        self.sim_pkt_size = None
        self.sim_total_flows = None
        self.workers = 1
        self.max_sessions = None
        self.s1u_ifname = None
        self.sgi_ifname = None
        self.interfaces = dict()
        self.enable_ntf = False

    def parse(self, ifaces):
        # Maximum number of flows to manage ip4 frags for re-assembly
        try:
            self.max_ip_defrag_flows = int(self.conf["max_ip_defrag_flows"])
        except ValueError:
            print('Invalid value for max_ip_defrag_flows. Not installing IP4Defrag module.')
        except KeyError:
            print('max_ip_defrag_flows value not set. Not installing IP4Defrag module.')

        # Enable ip4 fragmentation
        try:
            self.ip_frag_with_eth_mtu = int(self.conf["ip_frag_with_eth_mtu"])
        except ValueError:
            print('Invalid value for ip_frag_with_eth_mtu. Not installing IP4Frag module.')
        except KeyError:
            print('ip_frag_with_eth_mtu value not set. Not installing IP4Frag module.')

        # Telemtrics
        # See this link for details:
        ## https://github.com/NetSys/bess/blob/master/bessctl/module_tests/timestamp.py
        try:
            self.measure = bool(self.conf["measure"])
        except ValueError:
            print('Invalid value for measure. Not installing Measure module.')
        except KeyError:
            print('measure value not set. Not installing Measure module.')

        # Fetch interfaces
        for iface in ifaces:
            try:
                self.interfaces[iface] = self.conf[iface]
            except KeyError:
                self.interfaces[iface] = {'ifname': iface}
                print('Can\'t read {} interface. Setting it to default ({}).'.format(iface, iface))

        # Detect mode. Default is dpdk
        try:
            self.mode = self.conf["mode"]
        except KeyError:
            print('Autodetecting network driver')

        # params for simulation
        try:
            self.sim_start_ue_ip = self.conf["sim"]["start_ue_ip"]
            self.sim_start_enb_ip = self.conf["sim"]["start_enb_ip"]
            self.sim_start_teid = int(self.conf["sim"]["start_teid"], 16)
            self.sim_pkt_size = self.conf["sim"]["pkt_size"]
            self.sim_total_flows = self.conf["sim"]["total_flows"]
        except ValueError:
            print('Invalid sim mode fields added.')
        except KeyError:
            print('Sim mode not selected.')

        # Parse workers
        try:
            self.workers = int(self.conf["workers"])
        except ValueError:
            print('Invalid workers value! Re-setting # of workers to 1.')

        # Maximum number of sessions to manage
        try:
            self.max_sessions = int(self.conf["max_sessions"])
        except ValueError:
            print('Invalid max_sessions value!')

        # Interface names
        try:
            self.s1u_ifname = self.conf["s1u"]["ifname"]
            self.sgi_ifname = self.conf["sgi"]["ifname"]
        except KeyError:
            self.s1u_ifname = "s1u"
            self.sgi_ifname = "sgi"
            print('Can\'t parse interface name(s)! Setting it to default values ({}, {})'.format("s1u", "sgi"))

        # Network Token Function
        try:
            self.enable_ntf = bool(self.conf['enable_ntf'])
        except KeyError:
            print('Network Token Function disabled')
