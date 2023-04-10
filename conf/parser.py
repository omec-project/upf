#!/usr/bin/env python
# SPDX-License-Identifier: Apache-2.0
# Copyright 2019 Intel Corporation

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
        self.hwcksum = False
        self.gtppsc = False
        self.ddp = False
        self.measure_upf = False
        self.mode = None
        self.sim_core = None
        self.sim_max_sessions = None
        self.sim_start_ue_ip = None
        self.sim_start_enb_ip = None
        self.sim_start_aupf_ip = None
        self.sim_n6_app_ip = None
        self.sim_n9_app_ip = None
        self.sim_start_n3_teid = None
        self.sim_start_n9_teid = None
        self.sim_pkt_size = None
        self.sim_total_flows = None
        self.workers = 1
        self.access_ifname = None
        self.core_ifname = None
        self.interfaces = dict()
        self.enable_ntf = False
        self.notify_sockaddr = "/tmp/notifycp"
        self.endmarker_sockaddr = "/tmp/pfcpport"
        self.enable_slice_metering = False
        self.measure_flow = False
        self.table_size_pdr_lookup = 0
        self.table_size_flow_measure = 0
        self.table_size_app_qer_lookup = 0
        self.table_size_session_qer_lookup = 0
        self.table_size_far_lookup = 0

    def parse(self, ifaces):
        # Maximum number of flows to manage ip4 frags for re-assembly
        try:
            self.max_ip_defrag_flows = int(self.conf["max_ip_defrag_flows"])
        except ValueError:
            print(
                'Invalid value for max_ip_defrag_flows. Not installing IP4Defrag module.')
        except KeyError:
            print('max_ip_defrag_flows value not set. Not installing IP4Defrag module.')

        # Enable ip4 fragmentation
        try:
            self.ip_frag_with_eth_mtu = int(self.conf["ip_frag_with_eth_mtu"])
        except ValueError:
            print(
                'Invalid value for ip_frag_with_eth_mtu. Not installing IP4Frag module.')
        except KeyError:
            print('ip_frag_with_eth_mtu value not set. Not installing IP4Frag module.')

        # Enable PDU Session container
        try:
            self.gtppsc = bool(self.conf["gtppsc"])
        except KeyError:
            print('gtppsc not set. Default: Not adding PDU Session Container extension header')

        # Enable hardware checksum
        try:
            self.hwcksum = bool(self.conf["hwcksum"])
        except KeyError:
            print('hwcksum not set, using default software fallback')

        # Enable DDP
        try:
            self.ddp = bool(self.conf["ddp"])
        except KeyError:
            print('ddp not set, using default software fallback')

        # Telemetrics
        # See this link for details:
        # https://github.com/omec-project/bess/blob/master/bessctl/module_tests/timestamp.py
        try:
            self.measure_upf = self.conf["measure_upf"]
        except KeyError:
            print('measure_upf value not set. Not installing Measure module.')

        # Fetch interfaces
        for iface in ifaces:
            try:
                self.interfaces[iface] = self.conf[iface]
            except KeyError:
                self.interfaces[iface] = {'ifname': iface}
                print('Can\'t read {} interface. Setting it to default ({}).'.format(
                    iface, iface))

        # Detect mode. Default is dpdk
        try:
            self.mode = self.conf["mode"]
        except KeyError:
            print('Autodetecting network driver')

        # params for simulation
        try:
            self.sim_core = self.conf["sim"]["core"]
            self.sim_max_sessions = self.conf["sim"]["max_sessions"]
            self.sim_start_ue_ip = self.conf["sim"]["start_ue_ip"]
            self.sim_start_enb_ip = self.conf["sim"]["start_enb_ip"]
            self.sim_start_aupf_ip = self.conf["sim"]["start_aupf_ip"]
            self.sim_n6_app_ip = self.conf["sim"]["n6_app_ip"]
            self.sim_n9_app_ip = self.conf["sim"]["n9_app_ip"]
            self.sim_start_n3_teid = int(self.conf["sim"]["start_n3_teid"], 16)
            self.sim_start_n9_teid = int(self.conf["sim"]["start_n9_teid"], 16)
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

        # Interface names
        try:
            self.access_ifname = self.conf["access"]["ifname"]
            self.core_ifname = self.conf["core"]["ifname"]
        except KeyError:
            self.access_ifname = "access"
            self.core_ifname = "core"
            print('Can\'t parse interface name(s)! Setting it to default values ({}, {})'.format(
                "access", "core"))

        # Slice rate limits
        try:
            self.conf["slice_rate_limit_config"]
            self.enable_slice_metering = True
        except KeyError:
            print("No slice rate limit! Disabling meter.")

        # UnixPort Paths
        try:
            self.notify_sockaddr = self.conf["notify_sockaddr"]
        except KeyError:
            print('Can\'t parse unix socket paths for notify! Setting it to default values ({})'.format(
                "/tmp/notifycp"))

        # UnixPort Paths
        try:
            self.endmarker_sockaddr = self.conf["endmarker_sockaddr"]
        except KeyError:
            print('Can\'t parse unix socket paths for end marker! Setting it to default values ({})'.format(
                "/tmp/pfcpport"))
        # Network Token Function
        try:
            self.enable_ntf = bool(self.conf['enable_ntf'])
        except KeyError:
            print('Network Token Function disabled')

        # Flow measurements
        try:
            self.measure_flow = bool(self.conf['measure_flow'])
        except KeyError:
            print('Flow measurement function disabled')

        # Table sizes
        try:
            self.table_size_pdr_lookup = self.conf["table_sizes"]["pdrLookup"]
            self.table_size_flow_measure = self.conf["table_sizes"]["flowMeasure"]
            self.table_size_app_qer_lookup = self.conf["table_sizes"]["appQERLookup"]
            self.table_size_session_qer_lookup = self.conf["table_sizes"]["sessionQERLookup"]
            self.table_size_far_lookup = self.conf["table_sizes"]["farLookup"]
        except KeyError:
            print("No explicit table sizes")
