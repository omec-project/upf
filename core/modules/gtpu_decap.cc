/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright(c) 2019 Intel Corporation
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
/* for udp header */
#include "utils/udp.h"
/* for gtp header */
#include "utils/gtp.h"
/* for GetDesc() */
#include "utils/format.h"
#include <rte_jhash.h>
/*----------------------------------------------------------------------------------*/
using bess::utils::Gtpv1;
using bess::utils::Ipv4;
using bess::utils::Udp;
/*----------------------------------------------------------------------------------*/
void GtpuDecap::ProcessBatch(Context *ctx, bess::PacketBatch *batch) {
  int cnt = batch->cnt();

  for (int i = 0; i < cnt; i++) {
    bess::Packet *p = batch->pkts()[i];
    /* assuming that this module comes right after EthernetTrim */
    /* pkt_len can be used as the length of IP datagram */
    /* Trim iph->ihl<<2 + sizeof(Udp) + size of Gtpv1 header */
    Ipv4 *iph = p->head_data<Ipv4 *>();
    Gtpv1 *gtph =
        (Gtpv1 *)((uint8_t *)iph + (iph->header_length << 2) + sizeof(Udp));
    batch->pkts()[i]->adj((iph->header_length << 2) + sizeof(Udp) +
                          gtph->header_length());
  }

  RunNextModule(ctx, batch);
}
/*----------------------------------------------------------------------------------*/
ADD_MODULE(GtpuDecap, "gtpu_decap", "first version of gtpu decap module")
