# SPDX-License-Identifier: BSD-3-Clause
# Copyright 2026 Forsway Scandinavia AB

from test_utils import *


class BessQosTest(BessModuleTestCase):
    def test_delete_refuses_a_key_it_could_not_extract(self):
        # Offset-based fields keep this standalone: the module needs no
        # metadata provider upstream of it to be built.
        qos = Qos(
            fields=[
                {"offset": 23, "num_bytes": 1},
                {"offset": 2, "num_bytes": 2},
            ]
        )

        # A delete naming the right number of fields is accepted even though no
        # such rule was ever added. Deleting an absent rule is idempotency on
        # this path, not a refusal, and this pins that.
        qos.delete(fields=[{"value_bin": b"\xff"}, {"value_bin": b"\x23\xba"}])

        # A delete naming the wrong number of fields cannot describe a key, so
        # it must be refused. Left unchecked, ExtractKey() returns EINVAL
        # before it reaches its memset(), and the module then deletes against
        # an uninitialised key.
        with self.assertRaises(bess.Error):
            qos.delete(fields=[{"value_bin": b"\xff"}])

        self.assertBessAlive()

    # pfcpiface rewrites a QER with add, and removes it with delete and clear,
    # while a worker runs the module. Each replaced or removed value is freed
    # with the workers paused, once 1024 wait, and clear always pauses them.
    # The rule is rewritten often enough to free twice under traffic, and the
    # workers must forward again after each pause.
    def test_rules_change_while_a_worker_runs(self):
        qos = Qos(fields=[{"offset": 23, "num_bytes": 1}])
        # Source leaves packet data as it finds it, so every packet is
        # rewritten to one whose byte 23, the IPv4 protocol, is TCP.
        pkt = get_tcp_packet(sip="10.1.2.3", dip="20.0.0.1")
        Source() -> Rewrite(templates=[bytes(pkt)]) -> qos
        qos:7 -> Sink()
        qos:8 -> Sink()
        rule = [{"value_bin": b"\x06"}]

        def pkts():
            return {g.ogate: g.pkts for g in bess.get_module_info(qos.name).ogates}

        # Traffic first, so that the rewrites run under it.
        qos.add(gate=7, fields=rule)
        bess.resume_all()
        deadline = time.time() + 5
        while pkts()[7] == 0 and time.time() < deadline:
            time.sleep(0.05)
        self.assertGreater(pkts()[7], 0, "no traffic before the rewrites")

        # 2999 replacements: the retired values are freed at 1024 and 2048.
        for i in range(1, 3000):
            qos.add(gate=7 + i % 2, fields=rule)
        time.sleep(0.2)
        before = pkts()
        time.sleep(0.2)
        after = pkts()
        # The last add sent the rule to gate 8.
        self.assertGreater(after[8], before[8], "the workers did not resume")

        qos.delete(fields=rule)
        qos.add(gate=7, fields=rule)
        time.sleep(0.2)
        self.assertGreater(pkts()[7], after[7])

        qos.clear()
        cleared = pkts()
        time.sleep(0.2)
        self.assertEqual(pkts(), cleared, "a cleared rule still matched")
        bess.pause_all()

        self.assertBessAlive()


suite = unittest.TestLoader().loadTestsFromTestCase(BessQosTest)
results = unittest.TextTestRunner(verbosity=2).run(suite)

if results.failures or results.errors:
    sys.exit(1)
