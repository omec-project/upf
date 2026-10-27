# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Forsway Scandinavia AB

# GtpuPathMonitoring: an echo request copied to every gNB, an echo response
# timed against it, the request returned to the pool once it is copied, and
# the commands that change the module's tables while a worker runs it.

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
        # Destroyed, not just disconnected: a Source left behind keeps its task,
        # and its packets would satisfy run_module's wait before the probe has
        # gone through.
        bess.destroy_module(flood.name)

        out = self.run_module(gpm, 0, [echo(ECHO_REQUEST, N3, "0.0.0.0")], [0])
        self.assertEqual([p[scapy.IP].dst for p in out[0]], [GNB_A])
        self.assertBessAlive()

    # pfcpiface adds and deletes gNBs as sessions come and go and reads the
    # statistics on every scrape, all while a worker runs the module. Without
    # the module's lock this takes bessd down within seconds, in a normal build
    # as well as under SANITIZE=1.
    def test_commands_while_a_worker_runs(self):
        gpm = GtpuPathMonitoring()
        gnbs = [ip("10.201.0.%d" % (i + 1)) for i in range(64)]
        Source() -> Rewrite(templates=[bytes(echo(ECHO_REQUEST, N3, "0.0.0.0"))]) -> gpm -> Sink()
        # Responses from the gNB addresses, with any sequence number, so some
        # match a stored request and update the latency table that read walks.
        # They are logged one by one, so they are kept to a trickle.
        responses = Source()
        response = Rewrite(templates=[bytes(echo(ECHO_RESPONSE, "10.201.0.1", N3, 0))])
        source_and_seq = RandomUpdate(
            fields=[
                {"offset": 26, "size": 4, "min": gnbs[0], "max": gnbs[-1]},
                {"offset": 50, "size": 2, "min": 0, "max": 65535},
            ]
        )
        responses -> response -> source_and_seq -> gpm
        bess.add_tc("responses", policy="rate_limit", resource="packet", limit={"packet": 200})
        responses.attach_task(parent="responses")

        start = time.time()
        samples = 0
        bess.resume_all()
        for i in range(500):
            gpm.add(gnb_ip=gnbs[i % 64])
            if i >= 16:
                gpm.delete(gnb_ip=gnbs[(i - 16) % 64])
            samples += sum(s.count for s in gpm.read(clear=i % 2 == 0).statistics)
            if i % 250 == 249:
                gpm.clear()
        bess.pause_all()
        print(
            "500 command rounds under traffic took %.1f s, %d latency samples read"
            % (time.time() - start, samples),
            file=sys.stderr,
        )
        # The responses did reach the latency table that read walks.
        self.assertGreater(samples, 0)
        self.assertBessAlive()

    # Packet::copy returns null when the pool is empty. Seventeen full queues of
    # echo requests hold more than the whole pool (262 144 buffers by default,
    # bessd --buffers), and they drain slowly into the module, so after the
    # first few each request it copies meets an empty pool.
    def test_echo_request_survives_an_empty_pool(self):
        gpm = GtpuPathMonitoring()
        gpm.add(gnb_ip=ip(GNB_A))
        gpm -> Sink()
        bess.add_tc("drain", policy="rate_limit", resource="packet", limit={"packet": 1000})
        bess.add_tc("queues", policy="round_robin", parent="drain")
        fill = RoundRobin(gates=list(range(17)))
        Source() -> Rewrite(templates=[bytes(echo(ECHO_REQUEST, N3, "0.0.0.0"))]) -> fill
        for i in range(17):
            q = Queue(size=16384)
            bess.connect_modules(fill.name, q.name, i, 0)
            bess.connect_modules(q.name, gpm.name, 0, 0)
            q.attach_task(parent="queues")

        bess.resume_all()
        time.sleep(3)
        bess.pause_all()
        self.assertBessAlive()
        # Each request is dropped once its copies are made, so the dead ends
        # count the requests; with one gNB, fewer copies than that means some
        # copies failed.
        info = bess.get_module_info(gpm.name)
        requests, copies = info.deadends, info.ogates[0].pkts
        print("requests %d, copies %d" % (requests, copies), file=sys.stderr)
        self.assertGreater(requests, copies)


suite = unittest.TestLoader().loadTestsFromTestCase(BessGtpuPathMonitoringTest)
results = unittest.TextTestRunner(verbosity=2).run(suite)

if results.failures or results.errors:
    sys.exit(1)
