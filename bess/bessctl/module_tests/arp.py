# Copyright (c) 2017, Cloudigo.
# All rights reserved.
#
# Redistribution and use in source and binary forms, with or without
# modification, are permitted provided that the following conditions are met:
#
# * Redistributions of source code must retain the above copyright notice, this
# list of conditions and the following disclaimer.
#
# * Redistributions in binary form must reproduce the above copyright notice,
# this list of conditions and the following disclaimer in the documentation
# and/or other materials provided with the distribution.
#
# * Neither the names of the copyright holders nor the names of their
# contributors may be used to endorse or promote products derived from this
# software without specific prior written permission.
#
# THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
# AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
# IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
# ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE
# LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
# CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
# SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
# INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
# CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
# ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
# POSSIBILITY OF SUCH DAMAGE.

from test_utils import *


class BessArpTest(BessModuleTestCase):

    def test_arp(self):
        arp = ArpResponder()

        eth_header = scapy.Ether(
            src='02:1e:67:9f:4d:ae', dst='ff:ff:ff:ff:ff:ff')
        arp_header = scapy.ARP(op=1, pdst='1.2.3.4')
        arp_req = eth_header / arp_header

        arp.add(ip='1.2.3.4', mac_addr='A0:22:33:44:55:66')

        arp_reply = arp_req.copy()
        arp_reply[scapy.Ether].src = 'A0:22:33:44:55:66'
        arp_reply[scapy.Ether].dst = '02:1e:67:9f:4d:ae'
        arp_reply[scapy.ARP].op = 2

        arp_reply[scapy.ARP].hwdst = arp_req[scapy.ARP].hwsrc
        arp_reply[scapy.ARP].hwsrc = 'A0:22:33:44:55:66'

        arp_reply[scapy.ARP].pdst = arp_req[scapy.ARP].psrc
        arp_reply[scapy.ARP].psrc = '1.2.3.4'

        pkt_outs = self.run_module(arp, 0, [arp_req], [0])
        self.assertEquals(len(pkt_outs[0]), 1)
        self.assertSamePackets(pkt_outs[0][0], arp_reply)

suite = unittest.TestLoader().loadTestsFromTestCase(BessArpTest)
results = unittest.TextTestRunner(verbosity=2).run(suite)

if results.failures or results.errors:
    sys.exit(1)
