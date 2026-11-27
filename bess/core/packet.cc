// Copyright (c) 2014-2016, The Regents of the University of California.
// Copyright (c) 2016-2017, Nefeli Networks, Inc.
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

#include "packet.h"

#include <cassert>
#include <cstdio>
#include <iomanip>
#include <sstream>
#include <string>

#include <glog/logging.h>

#include <rte_errno.h>

#include "dpdk.h"
#include "opts.h"
#include "utils/common.h"

namespace bess {

static struct rte_mempool *pframe_pool[RTE_MAX_NUMA_NODES];

Packet *Packet::copy(const Packet *src) {
  // Only the first segment's data is copied, so a packet in several segments
  // cannot be: a caller sees the same failure as an empty pool.
  if (!src->is_linear()) {
    return nullptr;  // FAIL.
  }

  Packet *dst = reinterpret_cast<Packet *>(rte_pktmbuf_alloc(src->pool_));
  if (!dst) {
    return nullptr;  // FAIL.
  }

  // A packet can be longer than a fresh buffer's tailroom: whatever was
  // prepended since it was received (an encapsulation, say) came out of its
  // headroom. Such a copy starts that much earlier, so it ends where the buffer
  // does. Both buffers come from the same pool, so it fits.
  const int len = src->total_len();
  const bool grown = len > dst->tailroom();
  if (grown) {
    dst->set_data_off(dst->data_off() - (len - dst->tailroom()));
  }

  // A sloppy copy reads and writes whole blocks, up to 31 bytes past len
  // (utils/copy.h). A copy that did not grow starts where a fresh buffer's data
  // area does, which its rounded length cannot overrun; a grown one ends at the
  // end of the buffer. The source's data ends wherever it ends: a full-size
  // frame's ends at the end of its buffer, however much was trimmed from or
  // prepended to its head. So copy sloppily only when both have the room.
  const bool sloppy = !grown && src->tailroom() >= 31;
  bess::utils::CopyInlined(dst->append(len), src->head_data(), len, sloppy);

  return dst;
}

// basically rte_hexdump() from eal_common_hexdump.c
static std::string HexDump(const void *buffer, size_t len) {
  std::ostringstream dump;
  size_t i, ofs;
  const char *data = reinterpret_cast<const char *>(buffer);

  dump << "Dump data at [" << buffer << "], len=" << len << std::endl;
  ofs = 0;
  while (ofs < len) {
    dump << std::setfill('0') << std::setw(8) << std::hex << ofs << ":";
    for (i = 0; ((ofs + i) < len) && (i < 16); i++) {
      dump << " " << std::setfill('0') << std::setw(2) << std::hex
           << (data[ofs + i] & 0xFF);
    }
    for (; i <= 16; i++) {
      dump << " | ";
    }
    for (i = 0; (ofs < len) && (i < 16); i++, ofs++) {
      char c = data[ofs];
      if ((c < ' ') || (c > '~')) {
        c = '.';
      }
      dump << c;
    }
    dump << std::endl;
  }
  return dump.str();
}

std::string Packet::Dump() {
  std::ostringstream dump;
  Packet *pkt;
  uint32_t nb_segs;
  uint32_t len;

  dump << "refcnt chain: ";
  for (pkt = this; pkt; pkt = pkt->next_) {
    dump << pkt->refcnt_ << ' ';
  }
  dump << std::endl;

  dump << "pool chain: ";
  for (pkt = this; pkt; pkt = pkt->next_) {
    int i;

    dump << pkt->pool_ << "(";

    for (i = 0; i < RTE_MAX_NUMA_NODES; i++) {
      if (pframe_pool[i] == pkt->pool_) {
        dump << "P" << i;
      }
    }
    dump << ") ";
  }
  dump << std::endl;

  dump << "dump packet at " << this << ", phys=" << buf_physaddr_
       << ", buf_len=" << buf_len_ << std::endl;
  dump << "  pkt_len=" << pkt_len_ << ", ol_flags=" << std::hex
       << mbuf_.ol_flags << ", nb_segs=" << std::dec << nb_segs_
       << ", in_port=" << mbuf_.port << std::endl;

  nb_segs = nb_segs_;
  pkt = this;
  while (pkt && nb_segs != 0) {
    __rte_mbuf_sanity_check(&pkt->mbuf_, 0);

    dump << "  segment at " << pkt << ", data=" << pkt->head_data()
         << ", data_len=" << std::dec << unsigned{data_len_} << std::endl;

    len = total_len();
    if (len > data_len_) {
      len = data_len_;
    }

    if (len != 0) {
      dump << HexDump(head_data(), len);
    }

    pkt = pkt->next_;
    nb_segs--;
  }

  return dump.str();
}

#define check_offset(field)                                    \
  static_assert(                                               \
      offsetof(Packet, field##_) == offsetof(rte_mbuf, field), \
      "Incompatibility detected between class Packet and struct rte_mbuf");

void Packet::CheckSanity() {
  static_assert(offsetof(Packet, mbuf_) == 0, "mbuf_ must be at offset 0");
  check_offset(buf_addr);
  check_offset(rearm_data);
  check_offset(data_off);
  check_offset(refcnt);
  check_offset(nb_segs);
  check_offset(rx_descriptor_fields1);
  check_offset(pkt_len);
  check_offset(data_len);
  check_offset(buf_len);
  check_offset(pool);
  check_offset(next);

  // TODO: check runtime properties
}

#undef check_offset

}  // namespace bess
