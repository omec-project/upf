/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright 2019 Intel Corporation
 */
/* for ip_defrag decls */
#include "ip_defrag.h"
/* for rte_zmalloc() */
#include <rte_malloc.h>
/* for be32_t */
#include "utils/endian.h"
/* for ToIpv4Address() */
#include "utils/ip.h"
/* for CalculateIpv4Checksum() */
#include "utils/checksum.h"
/* for eth header */
#include "utils/ether.h"
/*----------------------------------------------------------------------------------*/
using bess::utils::be16_t;
using bess::utils::be32_t;
using bess::utils::Ethernet;
using bess::utils::Ipv4;
using bess::utils::ToIpv4Address;

#define PREFETCH_OFFSET 8
#define IP_FRAG_TBL_BUCKET_ENTRIES 16
enum { DEFAULT_GATE = 0, FORWARD_GATE };
/*----------------------------------------------------------------------------------*/
/**
 * Returns NULL if packet is fragmented and needs more for reassembly.
 * Returns Packet ptr if the packet is unfragmented, or is freshly reassembled.
 */
bess::Packet *IPDefrag::IPReassemble(Context *ctx, bess::Packet *p) {
  Ethernet *eth = p->head_data<Ethernet *>();
  if (eth->ether_type != (be16_t)(Ethernet::kIpv4))
    return p;
  Ipv4 *iph = (Ipv4 *)((unsigned char *)eth + sizeof(Ethernet));

  if (rte_ipv4_frag_pkt_is_fragmented((struct rte_ipv4_hdr *)iph)) {
    struct rte_mbuf *mo, *m;
    struct rte_ipv4_hdr *ip;

    /* prepare mbuf: setup l2_len/l3_len */
    m = reinterpret_cast<struct rte_mbuf *>(p);
    ip = reinterpret_cast<struct rte_ipv4_hdr *>(iph);
    m->l2_len = sizeof(*eth);
    m->l3_len = sizeof(*iph);

    /* process this fragment */
    mo = rte_ipv4_frag_reassemble_packet(ift, &ifdr, m, cur_tsc, ip);
    if (mo == NULL) {
      /* no packet to process just yet */
      p = NULL;
      return p;
    }
    /* we have our packet reassembled */
    if (mo != m) {
      /* move mbuf data in the first segment */
      if (rte_pktmbuf_linearize(mo) == 0) {
        p = reinterpret_cast<bess::Packet *>(mo);
        eth = p->head_data<Ethernet *>();
        iph = (Ipv4 *)((unsigned char *)eth + sizeof(Ethernet));
      } else {
        DLOG(INFO) << "Failed to linearize rte_mbuf. "
                   << "Is there enough tail room?" << std::endl;
        EmitPacket(ctx, p, DEFAULT_GATE);
        return NULL;
      }
    }
  }

  // Recalculate checksum
  iph->checksum = 0;
  iph->checksum = CalculateIpv4Checksum(*iph);

  return p;
}
/*----------------------------------------------------------------------------------*/
void IPDefrag::ProcessBatch(Context *ctx, bess::PacketBatch *batch) {
  /* retire outdated frags (if needed) */
  if (ifdr.cnt != 0)
    rte_ip_frag_free_death_row(&ifdr, PREFETCH_OFFSET);
  cur_tsc = rte_rdtsc();

  int cnt = batch->cnt();
  for (int i = 0; i < cnt; i++) {
    bess::Packet *p = batch->pkts()[i];
    p = IPReassemble(ctx, p);
    if (p)
      EmitPacket(ctx, p, FORWARD_GATE);
  }
}
/*----------------------------------------------------------------------------------*/
void IPDefrag::DeInit() {
  if (ift != NULL) {
    /* free allocated IP frags */
    rte_ip_frag_table_destroy(ift);
    ift = NULL;
  }
}
/*----------------------------------------------------------------------------------*/
CommandResponse IPDefrag::Init(const bess::pb::IPDefragArg &arg) {
  num_flows = arg.num_flows();

  if (num_flows <= 0)
    return CommandFailure(EINVAL, "Invalid num_flows!");

  numa = arg.numa();

  defrag_cycles = (rte_get_tsc_hz() + MS_PER_S - 1) / MS_PER_S * num_flows;

  cur_tsc = 0;

  ift = rte_ip_frag_table_create(num_flows, IP_FRAG_TBL_BUCKET_ENTRIES,
                                 num_flows * IP_FRAG_TBL_BUCKET_ENTRIES,
                                 defrag_cycles, numa);
  if (ift == NULL) {
    std::cerr << "Could not allocate memory for reassembly table "
              << "for NUMA node " << numa << ". Trying SOCKET_ID_ANY...";
    ift = rte_ip_frag_table_create(num_flows, IP_FRAG_TBL_BUCKET_ENTRIES,
                                   num_flows * IP_FRAG_TBL_BUCKET_ENTRIES,
                                   defrag_cycles, SOCKET_ID_ANY);
    if (ift == NULL)
      return CommandFailure(ENOMEM,
                            "SOCKET_ID_ANY memory allocation failed."
                            "Can't allocate memory for reassembly table!");
  }

  ifdr.cnt = 0;
  return CommandSuccess();
}
/*----------------------------------------------------------------------------------*/
ADD_MODULE(IPDefrag, "ip_defrag", "IP Reassembly module")
