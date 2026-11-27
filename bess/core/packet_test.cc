/*
 * SPDX-License-Identifier: BSD-3-Clause
 * Copyright 2026 Forsway Scandinavia AB
 */
#include "packet.h"

#include <gtest/gtest.h>

#include <sys/mman.h>
#include <unistd.h>

#include <cstdint>
#include <cstring>
#include <initializer_list>
#include <memory>
#include <utility>

#include "dpdk.h"
#include "packet_pool.h"

namespace {

class PacketCopyTest : public ::testing::Test {
 protected:
  // InitDpdk() is idempotent and aborts the process if EAL cannot be brought
  // up, so there is nothing here worth asserting afterwards.
  void SetUp() override {
    bess::InitDpdk();
    pool_ = std::make_unique<bess::PlainPacketPool>(64);
  }

  std::unique_ptr<bess::PlainPacketPool> pool_;
};

// A packet filling a buffer's data area and then grown into its headroom is
// longer than a fresh buffer's tailroom. That is the shape of a downlink packet
// at the Lawful Interception tee: GTP-U encapsulation and the 9-byte LI tag are
// prepended after it was received. Its copy was appended past the tailroom,
// which fails, and the null was copied to.
TEST_F(PacketCopyTest, CopiesAPacketThatGrewIntoItsHeadroom) {
  for (int growth : {0, 9, 53, SNBUF_HEADROOM}) {
    SCOPED_TRACE(growth);

    bess::Packet *src = pool_->Alloc();
    ASSERT_NE(src, nullptr);
    ASSERT_NE(src->append(src->tailroom()), nullptr);
    if (growth > 0) {
      ASSERT_NE(src->prepend(growth), nullptr);
    }

    const int len = src->total_len();
    ASSERT_EQ(len, SNBUF_DATA + growth);
    uint8_t *data = src->head_data<uint8_t *>();
    for (int i = 0; i < len; i++) {
      data[i] = static_cast<uint8_t>(i * 7 + 3);
    }

    bess::Packet *dst = bess::Packet::copy(src);
    ASSERT_NE(dst, nullptr);
    EXPECT_EQ(dst->total_len(), len);
    EXPECT_EQ(dst->data_len(), len);
    EXPECT_EQ(0, memcmp(dst->head_data(), src->head_data(), len));

    bess::Packet::Free(dst);
    bess::Packet::Free(src);
  }
}

// A packet whose data ends at the end of its buffer has nothing readable past
// its last byte. That is a full-size frame, however much was trimmed from its
// head (GTP-U decapsulation) or prepended to it (the 9-byte LI tag), so its
// copy fits a fresh buffer's tailroom. Here the source's buffer is followed by
// an unmapped page, so a copy that reads past the buffer faults. A sloppy copy
// reads up to a block past the data, 15 bytes with 16-byte blocks and 31 with
// AVX2's 32: 2033 bytes with 10 free after them is copied exactly, and 2017
// with 31 free may be copied in whole blocks.
TEST_F(PacketCopyTest, ReadsNothingPastTheEndOfTheSourceBuffer) {
  const size_t page = sysconf(_SC_PAGESIZE);
  const uint16_t buf_len = SNBUF_HEADROOM + SNBUF_DATA;
  const size_t region = ((buf_len + page - 1) / page + 1) * page;
  void *mem = mmap(nullptr, region, PROT_READ | PROT_WRITE,
                   MAP_PRIVATE | MAP_ANONYMOUS, -1, 0);
  ASSERT_NE(mem, MAP_FAILED);
  char *guard = static_cast<char *>(mem) + region - page;
  ASSERT_EQ(0, mprotect(guard, page, PROT_NONE));

  struct Shape {
    int room;       // left free after the data
    int trimmed;    // from the head
    int prepended;  // to the head
  };
  // 2021 bytes ending at the buffer's end is Copilot's case.
  for (const Shape &c : std::initializer_list<Shape>{{0, 36, 9},
                                                     {0, 36, 0},
                                                     {0, 1, 0},
                                                     {0, 0, 0},
                                                     {10, 5, 0},
                                                     {31, 0, 0}}) {
    SCOPED_TRACE(testing::Message()
                 << c.room << "/" << c.trimmed << "/" << c.prepended);

    bess::Packet *src = pool_->Alloc();
    ASSERT_NE(src, nullptr);
    rte_mbuf *m = reinterpret_cast<rte_mbuf *>(src);
    void *own_addr = m->buf_addr;
    const uint16_t own_len = m->buf_len;
    m->buf_addr = guard - buf_len;
    m->buf_len = buf_len;
    src->set_data_off(SNBUF_HEADROOM);

    ASSERT_NE(src->append(src->tailroom() - c.room), nullptr);
    if (c.trimmed > 0) {
      ASSERT_NE(src->adj(c.trimmed), nullptr);
    }
    if (c.prepended > 0) {
      ASSERT_NE(src->prepend(c.prepended), nullptr);
    }
    const int len = src->total_len();
    ASSERT_EQ(len, SNBUF_DATA - c.room - c.trimmed + c.prepended);
    ASSERT_EQ(src->head_data<char *>() + len + c.room, guard);
    uint8_t *data = src->head_data<uint8_t *>();
    for (int i = 0; i < len; i++) {
      data[i] = static_cast<uint8_t>(i * 7 + 3);
    }

    bess::Packet *dst = bess::Packet::copy(src);
    ASSERT_NE(dst, nullptr);
    EXPECT_EQ(dst->total_len(), len);
    EXPECT_EQ(0, memcmp(dst->head_data(), src->head_data(), len));
    // What lets a copy that did not grow write in whole blocks: it starts at a
    // block-aligned data area, which a length of at most 2048 rounded up to a
    // block does not pass.
    EXPECT_EQ(0u, reinterpret_cast<uintptr_t>(dst->head_data()) % 32);
    bess::Packet::Free(dst);

    m->buf_addr = own_addr;
    m->buf_len = own_len;
    src->set_data_off(SNBUF_HEADROOM);
    bess::Packet::Free(src);
  }

  munmap(mem, region);
}

// Only the first segment's data is copied, so a packet in several segments is
// refused, as an empty pool is, before any buffer is taken from the pool.
TEST_F(PacketCopyTest, RefusesAPacketInSeveralSegments) {
  bess::Packet *head = pool_->Alloc(100);
  bess::Packet *tail = pool_->Alloc(100);
  ASSERT_NE(head, nullptr);
  ASSERT_NE(tail, nullptr);
  ASSERT_EQ(0, rte_pktmbuf_chain(reinterpret_cast<rte_mbuf *>(head),
                                 reinterpret_cast<rte_mbuf *>(tail)));
  ASSERT_FALSE(head->is_linear());

  const size_t free_before = pool_->Size();
  EXPECT_EQ(bess::Packet::copy(head), nullptr);
  EXPECT_EQ(pool_->Size(), free_before);

  bess::Packet::Free(head);  // and its tail
}

}  // namespace
