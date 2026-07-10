/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright 2019 Intel Corporation
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
/*----------------------------------------------------------------------------------*/
void GtpuParser::set_gtp_parsing_attrs(be32_t *sip, be32_t *dip, be16_t *sp,
                                       be16_t *dp, be32_t *teid, be32_t *tipd,
                                       uint8_t *protoid, bess::Packet *p) {
  /* set src_ip */
  set_attr<uint32_t>(this, src_ip_id, p, sip->raw_value());
  /* set dst_ip */
  set_attr<uint32_t>(this, dst_ip_id, p, dip->raw_value());
  /* set src_port_id */
  set_attr<uint16_t>(this, src_port_id, p, sp->raw_value());
  /* set dst_port_id */
  set_attr<uint16_t>(this, dst_port_id, p, dp->raw_value());
  /* set tied_id */
  set_attr<uint32_t>(this, teid_id, p, teid->raw_value());
  /* tunnel_ip4_dst_id  */
  set_attr<uint32_t>(this, tunnel_ip4_dst_id, p, tipd->raw_value());
  /* proto_id */
  set_attr<uint8_t>(this, proto_id, p, *protoid);
}
/*----------------------------------------------------------------------------------*/
void GtpuParser::ProcessBatch(Context *ctx, bess::PacketBatch *batch) {
  int cnt = batch->cnt();
  Tcp *tcph = NULL;
  Udp *udph = NULL;
  Gtpv1 *gtph = NULL;
  Ipv4 *iph = NULL;
  Ethernet *eth = NULL;
  static const uint32_t _const_val = 0xFFFFFFFFu;

  for (int i = 0; i < cnt; i++) {
    bess::Packet *p = batch->pkts()[i];
    eth = p->head_data<Ethernet *>();
    if (eth->ether_type != (be16_t)(Ethernet::kIpv4) &&
        eth->ether_type != (be16_t)(Ethernet::kArp)) {
      EmitPacket(ctx, p, DEFAULT_GATE);
      continue;
    }

    iph = (Ipv4 *)(eth + 1);
    switch (iph->protocol) {
      case Ipv4::kTcp:
        tcph = (Tcp *)((char *)iph + (iph->header_length << 2));
        set_gtp_parsing_attrs(&iph->src, &iph->dst, &tcph->src_port,
                              &tcph->dst_port, (be32_t *)&_const_val,
                              (be32_t *)&_const_val, &iph->protocol, p);
        break;
      case Ipv4::kUdp:
        udph = (Udp *)((char *)iph + (iph->header_length << 2));
        if (udph->dst_port == (be16_t)(UDP_PORT_GTPU)) {
          Ipv4 *old_iph = iph;
          gtph = (Gtpv1 *)(udph + 1);
          be32_t teid = (be32_t)gtph->teid.value();
          /* reuse iph, tcph, and udph for innser headers too */
          iph = (Ipv4 *)((char *)gtph + gtph->header_length());
          switch (iph->protocol) {
            case Ipv4::kTcp:
              tcph = (Tcp *)((char *)iph + (iph->header_length << 2));
              set_gtp_parsing_attrs(&iph->src, &iph->dst, &tcph->src_port,
                                    &tcph->dst_port, (be32_t *)&teid,
                                    &old_iph->dst, &iph->protocol, p);
              break;
            case Ipv4::kUdp:
              udph = (Udp *)((char *)iph + (iph->header_length << 2));
              set_gtp_parsing_attrs(&iph->src, &iph->dst, &udph->src_port,
                                    &udph->dst_port, (be32_t *)&teid,
                                    &old_iph->dst, &iph->protocol, p);
              break;
            case Ipv4::kEsp:
              // ESP has no ports, encrypted payload
              set_gtp_parsing_attrs(&iph->src, &iph->dst, (be16_t *)&_const_val,
                                    (be16_t *)&_const_val, (be32_t *)&teid,
                                    &old_iph->dst, &iph->protocol, p);
              break;
            default:
              set_gtp_parsing_attrs(&iph->src, &iph->dst, (be16_t *)&_const_val,
                                    (be16_t *)&_const_val, (be32_t *)&teid,
                                    &old_iph->dst, &iph->protocol, p);
              break;
          }
        } else {
          set_gtp_parsing_attrs(&iph->src, &iph->dst, &udph->src_port,
                                &udph->dst_port, (be32_t *)&_const_val,
                                (be32_t *)&_const_val, &iph->protocol, p);
        }
        break;
      case Ipv4::kIcmp:
        set_gtp_parsing_attrs(&iph->src, &iph->dst, (be16_t *)&_const_val,
                              (be16_t *)&_const_val, (be32_t *)&_const_val,
                              (be32_t *)&_const_val, &iph->protocol, p);
        break;
      case Ipv4::kEsp:
        set_gtp_parsing_attrs(&iph->src, &iph->dst, (be16_t *)&_const_val,
                              (be16_t *)&_const_val, (be32_t *)&_const_val,
                              (be32_t *)&_const_val, &iph->protocol, p);
        break;
      default:
        /* nothing here at the moment */
        break;
    }

    EmitPacket(ctx, p, FORWARD_GATE);
  }
}
/*----------------------------------------------------------------------------------*/
CommandResponse GtpuParser::Init(const bess::pb::EmptyArg &) {
  using AccessMode = bess::metadata::Attribute::AccessMode;
  src_ip_id = AddMetadataAttr("src_ip", sizeof(uint32_t), AccessMode::kWrite);
  dst_ip_id = AddMetadataAttr("dst_ip", sizeof(uint32_t), AccessMode::kWrite);
  src_port_id =
      AddMetadataAttr("src_port", sizeof(uint16_t), AccessMode::kWrite);
  dst_port_id =
      AddMetadataAttr("dst_port", sizeof(uint16_t), AccessMode::kWrite);
  teid_id = AddMetadataAttr("teid", sizeof(uint32_t), AccessMode::kWrite);
  tunnel_ip4_dst_id =
      AddMetadataAttr("tunnel_ipv4_dst", sizeof(uint32_t), AccessMode::kWrite);
  proto_id = AddMetadataAttr("ip_proto", sizeof(uint8_t), AccessMode::kWrite);

  return CommandSuccess();
}
/*----------------------------------------------------------------------------------*/
ADD_MODULE(GtpuParser, "gtpu_parser", "parsing module for gtp traffic")
