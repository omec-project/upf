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

}  // namespace
