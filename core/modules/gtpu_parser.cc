/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright(c) 2019 Intel Corporation
 */
/* for GTP parser */
#include "gtpu_parser.h"
/* for ethernet header  */
#include "utils/ether.h"
/* for ip header  */
#include "utils/ip.h"
/* for udp header */
#include "utils/udp.h"
/* for tcp header */
#include "utils/tcp.h"
/* for gtp header */
#include "utils/gtp.h"
/*----------------------------------------------------------------------------------*/
using bess::utils::Ethernet;
using bess::utils::Gtpv1;
using bess::utils::Ipv4;
using bess::utils::Tcp;
using bess::utils::Udp;

enum { DEFAULT_GATE = 0, FORWARD_GATE };
const unsigned short UDP_PORT_GTPU = 2152;
/*----------------------------------------------------------------------------------*/
void GtpuParser::ProcessBatch(Context *ctx, bess::PacketBatch *batch) {
  int cnt = batch->cnt();
  struct EpcMetadata epc_meta;
  Tcp *tcph = NULL;
  Udp *udph = NULL;
  Gtpv1 *gtph = NULL;
  Ipv4 *iph = NULL;
  Ethernet *eth = NULL;

  for (int i = 0; i < cnt; i++) {
    bess::Packet *p = batch->pkts()[i];
    eth = p->head_data<Ethernet *>();
    if (eth->ether_type != (be16_t)(Ethernet::kIpv4) &&
        eth->ether_type != (be16_t)(Ethernet::kArp)) {
      EmitPacket(ctx, p, DEFAULT_GATE);
      continue;
    }
    /* memset epc_meta */
    memset(&epc_meta, 0, sizeof(struct EpcMetadata));

    iph = (Ipv4 *)(eth + 1);
    switch (iph->protocol) {
      case Ipv4::kTcp:
        tcph = (Tcp *)((char *)iph + (iph->header_length << 2));
        epc_meta.l4_sport = tcph->src_port;
        epc_meta.l4_dport = tcph->dst_port;
        break;
      case Ipv4::kUdp:
        udph = (Udp *)((char *)iph + (iph->header_length << 2));
        epc_meta.l4_sport = udph->src_port;
        epc_meta.l4_dport = udph->dst_port;
        if (udph->dst_port == (be16_t)(UDP_PORT_GTPU)) {
          gtph = (Gtpv1 *)(udph + 1);
          epc_meta.teid = gtph->teid;
          /* reuse iph, tcph, and udph for innser headers too */
          iph = (Ipv4 *)((char *)gtph + gtph->header_length());
          if (iph->protocol == Ipv4::kTcp) {
            tcph = (Tcp *)((char *)iph + (iph->header_length << 2));
            epc_meta.inner_l4_sport = tcph->src_port;
            epc_meta.inner_l4_dport = tcph->dst_port;
          } else if (iph->protocol == Ipv4::kUdp) {
            udph = (Udp *)((char *)iph + (iph->header_length << 2));
            epc_meta.inner_l4_sport = udph->src_port;
            epc_meta.inner_l4_dport = udph->dst_port;
          }
        }
        break;
      case Ipv4::kIcmp: {
        /* do nothing */
      } break;
      default:
        /* nothing here at the moment */
        break;
    }

    set_attr<struct EpcMetadata>(this, 0, p, epc_meta);
    EmitPacket(ctx, p, FORWARD_GATE);
  }
}
/*----------------------------------------------------------------------------------*/
CommandResponse GtpuParser::Init(const bess::pb::EmptyArg &) {
  using AccessMode = bess::metadata::Attribute::AccessMode;
  AddMetadataAttr("epc_metadata", sizeof(struct EpcMetadata),
                  AccessMode::kWrite);

  return CommandSuccess();
}
/*----------------------------------------------------------------------------------*/
ADD_MODULE(GtpuParser, "gtpu_parser", "parsing module for gtp traffic")
