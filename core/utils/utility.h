/*
 * SPDX-License-Identifier: BSD-3-Clause
 * Copyright 2014-2016, The Regents of the University of California.
 * Copyright 2016-2017, Nefeli Networks, Inc.
 * Copyright 2021-present Intel Corporation
 */
#ifndef BESS_UTILITY_H_
#define BESS_UTILITY_H_

#include <rte_meter.h>
#include "../packet.h"

namespace bess {
namespace utils {

 void MarkColor(bess::Packet *pkt,  uint8_t color)
 {
  struct rte_mbuf *m = reinterpret_cast<struct rte_mbuf *>(pkt);
  rte_mbuf_sched_color_set(m, color);
 }

}
}
#endif
