# SPDX-License-Identifier: Apache-2.0
# Copyright 2021 Open Networking Foundation

from trex.stl.api import STLClient
import ptf.testutils as testutils
from ptf.base_tests import BaseTest


class TrexTest(BaseTest):
    """
    Base test for setting up and tearing down TRex client instance for
    linerate tests.
    """
    def setUp(self):
        super(TrexTest, self).setUp()
        trex_server_addr = testutils.test_param_get("trex_server_addr")
        self.trex_client = STLClient(server=trex_server_addr)
        self.trex_client.connect()
        self.trex_client.acquire()
        self.reset()

    def reset(self):
        self.trex_client.reset()  # Resets configs from all ports
        self.trex_client.clear_stats()  # Clear status from all ports
        # Put all ports to promiscuous mode, otherwise they will drop all
        # incoming packets if the destination mac is not the port mac address.
        self.trex_client.set_port_attr(
            self.trex_client.get_all_ports(), promiscuous=True
        )

    def tearDown(self):
        print("Tearing down STLClient...")
        self.trex_client.stop()
        self.trex_client.release()
        self.trex_client.disconnect()
        super(TrexTest, self).tearDown()
