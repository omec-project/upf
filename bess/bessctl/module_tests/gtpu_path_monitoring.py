# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Forsway Scandinavia AB

# GtpuPathMonitoring: an echo request copied to every gNB, an echo response
# timed against it, and the request returned to the pool once it is copied.

import ipaddress
import struct

from test_utils import *

N3 = "10.200.0.3"
GNB_A = "10.200.0.201"
GNB_B = "10.200.0.202"
ECHO_REQUEST = 1
ECHO_RESPONSE = 2


def ip(addr):
    return int(ipaddress.IPv4Address(addr))


def echo(mtype, src, dst, seq=0):
    # Version 1, PT, S: the sequence number, N-PDU number and next extension
    # header type follow the fixed header, which the module reads.
    gtp = bytes([0x32, mtype]) + struct.pack("!HI", 4, 0) + struct.pack("!HBB", seq, 0, 0)
    return (
        scapy.Ether(src="02:00:00:00:00:01", dst="02:00:00:00:00:02")
        / scapy.IP(src=src, dst=dst)
        / scapy.UDP(sport=2152, dport=2152)
        / scapy.Raw(gtp)
    )


def seq_of(pkt):
    return struct.unpack("!H", bytes(pkt[scapy.Raw])[8:10])[0]


class BessGtpuPathMonitoringTest(BessModuleTestCase):
    def test_echo_request_goes_to_every_gnb(self):
        gpm = GtpuPathMonitoring()
        gpm.add(gnb_ip=ip(GNB_A))
        gpm.add(gnb_ip=ip(GNB_B))
        out = self.run_module(gpm, 0, [echo(ECHO_REQUEST, N3, "0.0.0.0")], [0])
        self.assertEqual(sorted(p[scapy.IP].dst for p in out[0]), [GNB_A, GNB_B])
        self.assertEqual([seq_of(p) for p in out[0]], [0, 0])

    def test_echo_request_without_a_gnb_is_dropped(self):
        gpm = GtpuPathMonitoring()
        out = self.run_module(gpm, 0, [echo(ECHO_REQUEST, N3, "0.0.0.0")], [0])
        self.assertEqual(len(out[0]), 0)

    def test_a_gnb_added_twice_stays_until_deleted_twice(self):
        gpm = GtpuPathMonitoring()
        gpm.add(gnb_ip=ip(GNB_A))
        gpm.add(gnb_ip=ip(GNB_A))
        gpm.delete(gnb_ip=ip(GNB_A))
        out = self.run_module(gpm, 0, [echo(ECHO_REQUEST, N3, "0.0.0.0")], [0])
        self.assertEqual([p[scapy.IP].dst for p in out[0]], [GNB_A])
        gpm.delete(gnb_ip=ip(GNB_A))
        out = self.run_module(gpm, 0, [echo(ECHO_REQUEST, N3, "0.0.0.0")], [0])
        self.assertEqual(len(out[0]), 0)

    def test_echo_response_is_timed_and_read_clears_it(self):
        gpm = GtpuPathMonitoring()
        gpm.add(gnb_ip=ip(GNB_A))
        self.run_module(
            gpm,
            0,
            [echo(ECHO_REQUEST, N3, "0.0.0.0"), echo(ECHO_RESPONSE, GNB_A, N3, 0)],
            [0],
        )
        stats = gpm.read(clear=True).statistics
        self.assertEqual([(s.gnb_ip, s.count) for s in stats], [(ip(GNB_A), 1)])
        stats = gpm.read(clear=False).statistics
        self.assertEqual([(s.gnb_ip, s.count) for s in stats], [(ip(GNB_A), 0)])

    # Each echo request is copied once per gNB and only the copies leave, so
    # the request itself is dropped, and counted as a dead end.
    def test_an_echo_request_is_dropped_once_copied(self):
        gpm = GtpuPathMonitoring()
        gpm.add(gnb_ip=ip(GNB_A))
        out = self.run_module(gpm, 0, [echo(ECHO_REQUEST, N3, "0.0.0.0")], [0])
        self.assertEqual(len(out[0]), 1)
        self.assertEqual(self.bess.get_module_info(gpm.name).deadends, 1)

    # Without the drop, every request keeps a buffer for good, and a burst
    # empties the pool every port allocates from; bessd then dies when the
    # fan-out cannot copy.
    def test_echo_requests_go_back_to_the_pool(self):
        gpm = GtpuPathMonitoring()
        gpm.add(gnb_ip=ip(GNB_A))
        flood = Source()
        flood -> Rewrite(templates=[bytes(echo(ECHO_REQUEST, N3, "0.0.0.0"))]) -> gpm -> Sink()
        bess.resume_all()
        time.sleep(2)
        bess.pause_all()
        sent = bess.get_module_info(gpm.name).ogates[0].pkts
        print("echo request copies sent in the burst: %d" % sent, file=sys.stderr)
        bess.disconnect_modules(flood.name, 0)

        out = self.run_module(gpm, 0, [echo(ECHO_REQUEST, N3, "0.0.0.0")], [0])
        self.assertEqual([p[scapy.IP].dst for p in out[0]], [GNB_A])
        self.assertBessAlive()

suite = unittest.TestLoader().loadTestsFromTestCase(BessGtpuPathMonitoringTest)
results = unittest.TextTestRunner(verbosity=2).run(suite)

if results.failures or results.errors:
    sys.exit(1)
