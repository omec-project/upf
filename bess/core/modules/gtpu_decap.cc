/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright 2019 Intel Corporation
 */
/* for gtpu_decap decls */
#include "gtpu_decap.h"
/* for rte_zmalloc() */
#include <rte_malloc.h>
/* for IPVERSION */
#include <netinet/ip.h>
/* for be32_t */
#include "utils/endian.h"
/* for ToIpv4Address() */
#include "utils/ip.h"
/* for Ethernet header */
#include "utils/ether.h"
/* for udp header */
#include "utils/udp.h"
/* for gtp header */
#include "utils/gtp.h"
/* for GetDesc() */
#include "utils/format.h"
#include <rte_jhash.h>
/*----------------------------------------------------------------------------------*/
using bess::utils::Ethernet;
using bess::utils::Gtpv1;
using bess::utils::Ipv4;
using bess::utils::Udp;
/*----------------------------------------------------------------------------------*/
void GtpuDecap::ProcessBatch(Context *ctx, bess::PacketBatch *batch) {
  int cnt = batch->cnt();

  for (int i = 0; i < cnt; i++) {
    bess::Packet *p = batch->pkts()[i];
    const char *malformed = nullptr;
    /* Trim the outer IPv4, UDP and GTP-U headers, keeping the Ethernet header:
     * it is copied forward over the last bytes trimmed. The length is checked
     * against the packet here rather than trusted to the parser, so a packet
     * that does not hold its headers is dropped instead of being trimmed past
     * its end. adj() refusing is the last guard: it checks only the bytes it
     * removes, not the Ethernet header copied after them. */
    size_t trim = OuterHeaderLength(p, &malformed);
    Ethernet *eth = p->head_data<Ethernet *>();
    void *new_p = malformed ? nullptr : p->adj(static_cast<uint16_t>(trim));
    if (!new_p) {
      drop_log_.Sample(ctx, name(), malformed ? malformed : "trim refused", p);
      DropPacket(ctx, p);
      continue;
    }
    memcpy(new_p, eth, sizeof(*eth));
    EmitPacket(ctx, p, 0);
  }
}
/*----------------------------------------------------------------------------------*/
/* Length of the outer IPv4, UDP and GTP-U headers (with optional fields and
 * extension headers), if the first segment holds all of them after the
 * Ethernet header; otherwise 0, with the header that does not fit named in
 * `malformed`. */
size_t GtpuDecap::OuterHeaderLength(bess::Packet *p, const char **malformed) {
  const size_t len = p->head_len();
  const char *head = p->head_data<const char *>();

  const size_t ip_off = sizeof(Ethernet);
  if (len < ip_off + sizeof(Ipv4)) {
    *malformed = "outer IPv4 header";
    return 0;
  }
  const Ipv4 *iph = reinterpret_cast<const Ipv4 *>(head + ip_off);
  const size_t ihl = iph->header_length << 2;
  if (ihl < sizeof(Ipv4) || ip_off + ihl + sizeof(Udp) > len) {
    *malformed = "outer IPv4 or UDP header";
    return 0;
  }

  const size_t gtp_off = ip_off + ihl + sizeof(Udp);
  const Gtpv1 *gtph = reinterpret_cast<const Gtpv1 *>(head + gtp_off);
  const size_t gtp_len = gtph->header_length(len - gtp_off);
  if (gtp_len == 0) {
    *malformed = "GTP-U header";
    return 0;
  }

  return ihl + sizeof(Udp) + gtp_len;
}
/*----------------------------------------------------------------------------------*/
ADD_MODULE(GtpuDecap, "gtpu_decap", "first version of gtpu decap module")
