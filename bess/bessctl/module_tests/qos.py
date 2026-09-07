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


suite = unittest.TestLoader().loadTestsFromTestCase(BessQosTest)
results = unittest.TextTestRunner(verbosity=2).run(suite)

if results.failures or results.errors:
    sys.exit(1)
