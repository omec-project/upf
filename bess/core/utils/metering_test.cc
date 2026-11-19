/*
 * SPDX-License-Identifier: BSD-3-Clause
 * Copyright 2026 Forsway Scandinavia AB
 */
#include "metering.h"

#include <gtest/gtest.h>

#include "../dpdk.h"

using bess::utils::Error;
using bess::utils::Metering;
using bess::utils::MeteringKey;

namespace {

// Metering's table is a DPDK rte_hash, so it cannot be created before EAL is
// up: rte_hash_create() fails, CuckooMap stays in its non-DPDK mode without
// saying so, and every insert then returns -1 regardless of the table's state.
// Bringing DPDK up here, rather than relying on some earlier test in the binary
// to have done it, is what keeps these tests independent of run order.
class MeteringTest : public ::testing::Test {
 protected:
  // InitDpdk() is idempotent and aborts the process if EAL cannot be brought
  // up, so there is nothing here worth asserting afterwards.
  void SetUp() override { bess::InitDpdk(); }
};

MeteringKey KeyOf(uint64_t v) {
  MeteringKey key = {{0}};
  key.u64_arr[0] = v;

  return key;
}

// A rule the table took must be reported as taken. insert_dpdk() forwards
// rte_hash_add_key_data(), which returns 0 on success, so a condition that
// treats 0 as the failure case reports every successful insert as an error --
// and the caller cannot tell that apart from a real refusal.
TEST_F(MeteringTest, AddReportsSuccessForARuleTheTableTook) {
  Metering<uint16_t> table;
  table.Init(sizeof(uint64_t), 1 << 6);

  const Error err = table.Add(0xBEEF, KeyOf(0x1234));

  EXPECT_EQ(0, err.first) << "a rule the table took was reported as a failure: "
                          << err.second;

  table.DeInit();
}

// A table with no room left must say so, and say which failure it was. Nothing
// above this layer knows the capacity, so an add dropped here is invisible to
// the control plane.
TEST_F(MeteringTest, AddReportsFailureWhenTheTableIsFull) {
  const int entries = 1 << 6;

  Metering<uint16_t> table;
  table.Init(sizeof(uint64_t), entries);

  Error err = std::make_pair(0, std::string());
  int accepted = 0;

  for (int i = 0; i < entries * 8; i++) {
    err = table.Add(static_cast<uint16_t>(i), KeyOf(i + 1));
    if (err.first != 0) {
      break;
    }
    accepted++;
  }

  // EXPECT rather than ASSERT: an ASSERT returns from the test immediately, so
  // the table below would never be released. Init() names its rte_hash after
  // the address of its own member, which is a stack address, so a leaked table
  // collides by name with the next test's and breaks it -- one failure here
  // would be reported as several.
  EXPECT_NE(0, err.first) << "the table accepted " << accepted
                          << " rules into a " << entries
                          << "-entry table without reporting a failure";
  EXPECT_EQ(ENOSPC, err.first)
      << "a full table must report ENOSPC and not some other errno, so that a "
         "refusal is distinguishable from a table that was never created; "
         "reported: "
      << err.second;

  table.DeInit();
}

// Counted counts its live instances, so that a test can tell whether the table
// freed the values it took copies of.
struct Counted {
  static int live;

  uint16_t v;

  explicit Counted(uint16_t value = 0) : v(value) { live++; }
  Counted(const Counted &o) : v(o.v) { live++; }
  Counted &operator=(const Counted &) = default;
  ~Counted() { live--; }
};

int Counted::live = 0;

// A rule added again for the same key replaces its value. The old one may still
// be in a worker's hands, so it is retired, and freed only by FreeRetired.
TEST_F(MeteringTest, AddRetiresTheValueItReplaces) {
  Metering<Counted> table;
  table.Init(sizeof(uint64_t), 1 << 6);
  const int base = Counted::live;

  EXPECT_EQ(0, table.Add(Counted(1), KeyOf(7)).first);
  EXPECT_EQ(0u, table.Retired()) << "a new rule replaced nothing";

  EXPECT_EQ(0, table.Add(Counted(2), KeyOf(7)).first);
  EXPECT_EQ(1u, table.Retired());
  EXPECT_EQ(2, table.Find(KeyOf(7), Counted(0)).v);
  EXPECT_EQ(base + 2, Counted::live) << "the replaced value was freed at once";

  table.FreeRetired();
  EXPECT_EQ(0u, table.Retired());
  EXPECT_EQ(base + 1, Counted::live) << "FreeRetired did not free the value";

  table.DeInit();
  EXPECT_EQ(base, Counted::live) << "DeInit leaked the value the table held";
}

TEST_F(MeteringTest, DeleteRetiresTheValueItRemoves) {
  Metering<Counted> table;
  table.Init(sizeof(uint64_t), 1 << 6);
  const int base = Counted::live;

  EXPECT_EQ(0, table.Add(Counted(1), KeyOf(7)).first);
  EXPECT_EQ(0, table.Delete(KeyOf(7)).first);
  EXPECT_EQ(1u, table.Retired());
  EXPECT_EQ(base + 1, Counted::live);

  EXPECT_EQ(ENOENT, table.Delete(KeyOf(7)).first);
  EXPECT_EQ(1u, table.Retired()) << "deleting a missing rule retired a value";

  table.FreeRetired();
  EXPECT_EQ(base, Counted::live);

  table.DeInit();
}

TEST_F(MeteringTest, ClearRetiresEveryValue) {
  Metering<Counted> table;
  table.Init(sizeof(uint64_t), 1 << 6);
  const int base = Counted::live;

  for (uint16_t i = 1; i <= 3; i++) {
    EXPECT_EQ(0, table.Add(Counted(i), KeyOf(i)).first);
  }

  table.Clear();
  EXPECT_EQ(0u, table.Count());
  EXPECT_EQ(3u, table.Retired());

  table.FreeRetired();
  EXPECT_EQ(base, Counted::live);

  table.DeInit();
}

// DeInit frees the values still waiting to be freed as well as those the
// table holds, each once.
TEST_F(MeteringTest, DeInitFreesTheRetiredValues) {
  Metering<Counted> table;
  table.Init(sizeof(uint64_t), 1 << 6);
  const int base = Counted::live;

  EXPECT_EQ(0, table.Add(Counted(1), KeyOf(7)).first);
  EXPECT_EQ(0, table.Add(Counted(2), KeyOf(7)).first);
  EXPECT_EQ(0, table.Add(Counted(3), KeyOf(8)).first);
  EXPECT_EQ(0, table.Delete(KeyOf(8)).first);
  EXPECT_EQ(2u, table.Retired());

  table.DeInit();
  EXPECT_EQ(base, Counted::live);
}

// A key removed and added again holds only its new value, and the old one is
// retired once.
TEST_F(MeteringTest, AKeyAddedAgainAfterItsRemovalHoldsOnlyItsNewValue) {
  Metering<Counted> table;
  table.Init(sizeof(uint64_t), 1 << 6);
  const int base = Counted::live;

  EXPECT_EQ(0, table.Add(Counted(1), KeyOf(7)).first);
  EXPECT_EQ(0, table.Delete(KeyOf(7)).first);
  EXPECT_EQ(0, table.Add(Counted(2), KeyOf(7)).first);
  table.Clear();
  EXPECT_EQ(0, table.Add(Counted(3), KeyOf(7)).first);

  EXPECT_EQ(2u, table.Retired());
  EXPECT_EQ(3, table.Find(KeyOf(7), Counted(0)).v);
  table.FreeRetired();
  EXPECT_EQ(base + 1, Counted::live);

  table.DeInit();
  EXPECT_EQ(base, Counted::live);
}

// A rule the table refused leaves nothing behind.
TEST_F(MeteringTest, ARefusedAddRetiresNothing) {
  const int entries = 1 << 6;

  Metering<Counted> table;
  table.Init(sizeof(uint64_t), entries);
  const int base = Counted::live;

  int i = 0;
  while (i < entries * 8 && table.Add(Counted(1), KeyOf(i + 1)).first == 0) {
    i++;
  }

  EXPECT_EQ(0u, table.Retired());
  EXPECT_EQ(base + static_cast<int>(table.Count()), Counted::live)
      << "a refused value was not freed";

  table.DeInit();
  EXPECT_EQ(base, Counted::live);
}

}  // namespace
