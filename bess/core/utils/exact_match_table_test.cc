// Copyright (c) 2014-2016, The Regents of the University of California.
// Copyright (c) 2016-2017, Nefeli Networks, Inc.
// Copyright 2025 Canonical Ltd.
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:
//
// * Redistributions of source code must retain the above copyright notice, this
// list of conditions and the following disclaimer.
//
// * Redistributions in binary form must reproduce the above copyright notice,
// this list of conditions and the following disclaimer in the documentation
// and/or other materials provided with the distribution.
//
// * Neither the names of the copyright holders nor the names of their
// contributors may be used to endorse or promote products derived from this
// software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
// AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
// IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
// ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE
// LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
// CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
// SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
// INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
// CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
// ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
// POSSIBILITY OF SUCH DAMAGE.

#include "exact_match_table.h"

#include <gtest/gtest.h>

#include "../dpdk.h"
#include "../packet_pool.h"
#include "endian.h"

using bess::utils::Error;
using bess::utils::ExactMatchField;
using bess::utils::ExactMatchKey;
using bess::utils::ExactMatchRuleFields;
using bess::utils::ExactMatchTable;
using google::protobuf::RepeatedPtrField;

// An ExactMatchTable stores its rules in a DPDK rte_hash, so no table can be
// created before EAL is up: rte_hash_create() fails, CuckooMap stays in its
// non-DPDK mode without reporting that, and every lookup then misses. Bringing
// DPDK up here is what makes these tests independent of the order they run in.
// Without it, four of them passed only because FindMakeKeysPktBatch runs
// before them and initialises EAL as a side effect of constructing a
// PacketPool; run on their own they failed.
class EmTableTest : public ::testing::Test {
 protected:
  // InitDpdk() is idempotent and aborts the process if EAL cannot be brought
  // up, so there is nothing here worth asserting afterwards.
  void SetUp() override { bess::InitDpdk(); }
};

TEST_F(EmTableTest, AddField) {
  ExactMatchTable<uint8_t> em;
  Error err = em.AddField(0, 4, 0, 0);
  ASSERT_EQ(0, err.first);
  ASSERT_EQ(1, em.num_fields());
  ExactMatchField ret = em.get_field(0);
  EXPECT_EQ(0, ret.offset);
  EXPECT_EQ(4, ret.size);
  EXPECT_EQ(0xFFFFFFFF, ret.mask);
  err = em.AddField(0, 4, 0, MAX_FIELDS);
  ASSERT_EQ(EINVAL, err.first);
}

TEST_F(EmTableTest, AddRule) {
  ExactMatchTable<uint16_t> em;
  ASSERT_EQ(0, em.AddField(0, 4, 0, 0).first);
  em.Init(1 << 4);
  ExactMatchRuleFields rule = {
      {0x01, 0x02, 0x03, 0x04},
  };
  Error err = em.AddRule(0xBEEF, rule);
  ASSERT_EQ(0, err.first);
  em.ClearRules();
  em.DeInit();
}

// A table with no room left must say so, and say which failure it was. Nothing
// above this layer knows the table's capacity, so a rule dropped here would
// otherwise be reported to the caller as installed.
TEST_F(EmTableTest, AddRuleReportsAFullTable) {
  const uint32_t entries = 1 << 6;

  ExactMatchTable<uint16_t> em;
  ASSERT_EQ(0, em.AddField(0, 4, 0, 0).first);
  em.Init(entries);

  Error err = std::make_pair(0, std::string());
  uint32_t accepted = 0;

  for (uint32_t i = 1; i <= entries * 8; i++) {
    const ExactMatchRuleFields rule = {
        {static_cast<uint8_t>(i), static_cast<uint8_t>(i >> 8),
         static_cast<uint8_t>(i >> 16), static_cast<uint8_t>(i >> 24)},
    };

    err = em.AddRule(static_cast<uint16_t>(i), rule);
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
         "refusal stays distinguishable from a table that was never created; "
         "reported: "
      << err.second;

  em.ClearRules();
  em.DeInit();
}

TEST_F(EmTableTest, FindMakeKeysPktBatch) {
  const size_t n = 2;
  ExactMatchTable<uint16_t> em;
  ExactMatchRuleFields rule = {{0x04, 0x03, 0x02, 0x01}};
  ExactMatchKey keys[n];
  bess::PacketBatch batch;
  bess::PlainPacketPool pool;
  bess::Packet *pkts[n];
  pool.AllocBulk(pkts, n, 0);
  char databuf[32] = {0};

  // Init() sizes the table's key from the fields added so far, so it has to
  // come after AddField(); called before, it built a table with a key length
  // of zero, which rte_hash_create() refuses. The assertion below then held
  // for the wrong reason -- a table that does not exist misses every key.
  ASSERT_EQ(0, em.AddField(0, 4, 0, 0).first);
  em.Init(1 << 6);
  ASSERT_EQ(0, em.AddRule(0xF00, rule).first);

  batch.clear();
  for (size_t i = 0; i < n; i++) {
    bess::Packet *pkt = pkts[i];
    bess::utils::Copy(pkt->append(sizeof(databuf)), databuf, sizeof(databuf));
    batch.add(pkt);
  }

  const auto buffer_fn = [](const bess::Packet *pkt, const ExactMatchField &) {
    return pkt->head_data<void *>();
  };
  em.MakeKeys(&batch, buffer_fn, keys);
  for (size_t i = 0; i < n; i++) {
    // Packets are bogus, shouldn't match anything.
    ASSERT_EQ(0xDEAD, em.Find(keys[i], 0xDEAD));
  }
  em.ClearRules();
  em.DeInit();
}

TEST_F(EmTableTest, LookupOneFieldOneRule) {
  ExactMatchTable<uint16_t> em;
  em.AddField(0, 4, 0, 0);
  ExactMatchRuleFields rule = {
      {0x04, 0x03, 0x02, 0x01},
  };
  uint64_t buf = 0x01020304;
  uint64_t bad_buf = 0xBAD;
  ExactMatchKey key = em.MakeKey(&buf);
  ExactMatchKey bad_key = em.MakeKey(&bad_buf);
  em.Init(1 << 10);
  em.AddRule(0xBEEF, rule);
  EXPECT_EQ(0xBEEF, em.Find(key, 0xDEAD));
  EXPECT_EQ(0xDEAD, em.Find(bad_key, 0xDEAD));
  em.ClearRules();
  em.DeInit();
}

TEST_F(EmTableTest, LookupTwoFieldsOneRule) {
  ExactMatchTable<uint16_t> em;
  ASSERT_EQ(0, em.AddField(0, 4, 0, 0).first);
  ASSERT_EQ(0, em.AddField(6, 2, 0, 1).first);
  ASSERT_EQ(2, em.num_fields());
  ExactMatchRuleFields rule = {{0x04, 0x03, 0x02, 0x01}, {0x06, 0x05}};
  uint64_t buf = 0x0506000001020304;
  ExactMatchKey key = em.MakeKey(&buf);
  em.Init(1 << 10);
  ASSERT_EQ(0, em.AddRule(0xBEEF, rule).first);
  uint16_t ret = em.Find(key, 0xDEAD);
  ASSERT_EQ(0xBEEF, ret);
  em.ClearRules();
  em.DeInit();
}

TEST_F(EmTableTest, LookupTwoFieldsTwoRules) {
  ExactMatchTable<uint16_t> em;
  ASSERT_EQ(0, em.AddField(0, 4, 0, 0).first);
  ASSERT_EQ(0, em.AddField(6, 2, 0, 1).first);
  ASSERT_EQ(2, em.num_fields());
  em.Init(1 << 10);
  ExactMatchRuleFields rule1 = {{0x04, 0x03, 0x02, 0x01}, {0x06, 0x05}};
  ExactMatchRuleFields rule2 = {{0x0F, 0x0E, 0x0D, 0x0C}, {0x06, 0x05}};
  uint64_t buf1 = 0x0506000001020304;
  uint64_t buf2 = 0x050600000C0D0E0F;
  uint64_t bad_buf = 0xBAD;
  const void *bufs[3] = {&buf1, &buf2, &bad_buf};
  ExactMatchKey keys[3];
  em.MakeKeys(bufs, keys, 3);
  ASSERT_EQ(0, em.AddRule(0xF00, rule1).first);
  ASSERT_EQ(0, em.AddRule(0xBA2, rule2).first);
  EXPECT_EQ(0xF00, em.Find(keys[0], 0xDEAD));
  EXPECT_EQ(0xBA2, em.Find(keys[1], 0xDEAD));
  EXPECT_EQ(0xDEAD, em.Find(keys[2], 0xDEAD));
  em.ClearRules();
  em.DeInit();
}

// This test is for a specific bug introduced at one point
// where the MakeKeys function didn't clear out any random
// crud that might be on the stack.
TEST_F(EmTableTest, IgnoreBytesPastEnd) {
  ExactMatchTable<uint16_t> em;
  ASSERT_EQ(0, em.AddField(6, 1, 0, 0).first);
  ASSERT_EQ(0, em.AddField(7, 8, 0, 1).first);
  em.Init(1 << 10);
  uint64_t buf[2] = {0x0102030405060708, 0x1112131415161718};
  const void *bufs[1] = {&buf};
  ExactMatchRuleFields rule = {
      {0x02}, {0x01, 0x18, 0x17, 0x16, 0x15, 0x14, 0x13, 0x12}};
  ExactMatchKey keys[1];
  memset(keys, 0x55, sizeof(keys));
  em.MakeKeys(bufs, keys, 1);
  ASSERT_EQ(0, em.AddRule(0x600d, rule).first);
  uint16_t ret = em.Find(keys[0], 0xDEAD);
  ASSERT_EQ(0x600d, ret);
  em.ClearRules();
  em.DeInit();
}
