# SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
# SPDX-License-Identifier: Apache-2.0

# FlowMeasure: the leader decides which buffer packets are counted in, and
# followers learn it from the packets that pass through them. A controller
# flips the leader, then reads the retired buffer from every module.

import struct

from test_utils import *

FSEID = 7
PDR = 20
FLAG_A = 1


def packet():
    return bytes(
        scapy.Ether(src="02:00:00:00:00:01", dst="02:00:00:00:00:02")
        / scapy.IP(src="10.250.0.1", dst="10.0.0.1")
        / scapy.UDP(sport=1000, dport=2000)
        / ("x" * 100)
    )


class BessFlowMeasureTest(BessModuleTestCase):
    # Timestamp and SetMetadata supply the attributes FlowMeasure reads, then
    # the packets pass the leader and one follower, as in conf/up4.bess.
    # FlowMeasure reads fseid and pdr_id in host order, and SetMetadata stores
    # value_int in network order, so the values are given as host-order bytes.
    def pipeline(self):
        ts = Timestamp()
        meta = SetMetadata(
            attrs=[
                {"name": "fseid", "size": 8, "value_bin": struct.pack("=Q", FSEID)},
                {"name": "pdr_id", "size": 4, "value_bin": struct.pack("=I", PDR)},
            ]
        )
        leader = FlowMeasure(leader=True, flag_attr_name="buffer_flag", entries=1024)
        follower = FlowMeasure(leader=False, flag_attr_name="buffer_flag", entries=1024)
        ts -> meta -> leader -> follower
        return ts, leader, follower

    # A follower that has seen no packet since the flip still takes the retired
    # buffer for its active one. It is not: the flip waits for packets stamped
    # with the old flag to drain, and any later packet writes the other buffer.
    # Refusing the read leaves the counts in the table, and a later read of the
    # same buffer reports them in the wrong interval.
    def test_a_follower_idle_since_the_flip_reads_the_retired_buffer(self):
        ts, leader, follower = self.pipeline()
        out = self.run_pipeline(ts, follower, 0, [packet()], [0])
        self.assertEqual(len(out[0]), 1)

        self.assertEqual(leader.flip().old_flag, FLAG_A)
        stats = follower.read(flag_to_read=FLAG_A, clear=True).statistics
        self.assertEqual(
            [(s.fseid, s.pdr, s.total_packets) for s in stats], [(FSEID, PDR, 1)]
        )
        # Cleared: the next read of the same buffer has nothing left to report.
        self.assertEqual(len(follower.read(flag_to_read=FLAG_A, clear=True).statistics), 0)

    # The leader's own flag is authoritative, so asking it for the buffer
    # packets are still being counted in is a controller error.
    def test_the_leader_refuses_its_active_buffer(self):
        ts, leader, follower = self.pipeline()
        self.run_pipeline(ts, follower, 0, [packet()], [0])

        with self.assertRaises(bess.Error) as ctx:
            leader.read(flag_to_read=FLAG_A, clear=False)
        self.assertEqual(ctx.exception.errmsg, "refusing to read active buffer")


suite = unittest.TestLoader().loadTestsFromTestCase(BessFlowMeasureTest)
results = unittest.TextTestRunner(verbosity=2).run(suite)

if results.failures or results.errors:
    sys.exit(1)
