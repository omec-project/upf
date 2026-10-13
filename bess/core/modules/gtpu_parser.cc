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
/* Whether this is an IPv4 fragment other than the first: a non-zero offset. */
static inline bool is_later_fragment(const Ipv4 *iph) {
  return (iph->fragment_offset.value() & 0x1FFF) != 0;
}
/*----------------------------------------------------------------------------------*/
void GtpuParser::ProcessBatch(Context *ctx, bess::PacketBatch *batch) {
  int cnt = batch->cnt();

  for (int i = 0; i < cnt; i++) {
    bess::Packet *p = batch->pkts()[i];
    const char *malformed = nullptr;
    gate_idx_t gate = Parse(p, &malformed);
    if (malformed) {
      drop_log_.Sample(ctx, name(), malformed, p);
    }
    EmitPacket(ctx, p, gate);
  }
}
/*----------------------------------------------------------------------------------*/
/* Sets the attributes pdrLookup matches on and returns FORWARD_GATE, or returns
 * DEFAULT_GATE for a packet that is not to be classified. Every read is checked
 * against the bytes the first segment holds before it is made. A packet that
 * claims more than it holds is reported through `malformed`; one that is
 * simply not an IPv4 G-PDU, such as an Error Indication or an IPv6 payload, is
 * dropped without being reported. */
gate_idx_t GtpuParser::Parse(bess::Packet *p, const char **malformed) {
  static const uint32_t _const_val = 0xFFFFFFFFu;
  const size_t len = p->head_len();
  char *head = p->head_data<char *>();

  if (len < sizeof(Ethernet)) {
    *malformed = "Ethernet header";
    return DEFAULT_GATE;
  }
  Ethernet *eth = reinterpret_cast<Ethernet *>(head);
  const bool ipv4 = eth->ether_type == (be16_t)(Ethernet::kIpv4);
  if (!ipv4 && eth->ether_type != (be16_t)(Ethernet::kArp)) {
    return DEFAULT_GATE;
  }

  /* ARP takes the IPv4 path as it always has; only its reads are bounded. Its
   * first octet gives an IHL of 0, so the IHL is validated for IPv4 alone. */
  const size_t ip_off = sizeof(Ethernet);
  if (len < ip_off + sizeof(Ipv4)) {
    *malformed = "outer IPv4 header";
    return DEFAULT_GATE;
  }
  Ipv4 *iph = reinterpret_cast<Ipv4 *>(head + ip_off);
  const size_t ihl = iph->header_length << 2;
  if (ipv4 && (ihl < sizeof(Ipv4) || ip_off + ihl > len)) {
    *malformed = "outer IPv4 header length";
    return DEFAULT_GATE;
  }
  const size_t l4_off = ip_off + ihl;

  /* A fragment after the first carries no transport header, so there are no
   * ports to read: it is classified on its addresses, as ICMP is. */
  if (ipv4 && is_later_fragment(iph)) {
    set_gtp_parsing_attrs(&iph->src, &iph->dst, (be16_t *)&_const_val,
                          (be16_t *)&_const_val, (be32_t *)&_const_val,
                          (be32_t *)&_const_val, &iph->protocol, p);
    return FORWARD_GATE;
  }

  switch (iph->protocol) {
    case Ipv4::kTcp: {
      if (l4_off + 2 * sizeof(be16_t) > len) {
        *malformed = "TCP ports";
        return DEFAULT_GATE;
      }
      Tcp *tcph = reinterpret_cast<Tcp *>(head + l4_off);
      set_gtp_parsing_attrs(&iph->src, &iph->dst, &tcph->src_port,
                            &tcph->dst_port, (be32_t *)&_const_val,
                            (be32_t *)&_const_val, &iph->protocol, p);
      break;
    }
    case Ipv4::kUdp: {
      if (l4_off + sizeof(Udp) > len) {
        *malformed = "UDP header";
        return DEFAULT_GATE;
      }
      Udp *udph = reinterpret_cast<Udp *>(head + l4_off);
      if (ipv4 && udph->dst_port == (be16_t)(UDP_PORT_GTPU)) {
        return ParseGtpu(p, iph, l4_off + sizeof(Udp), malformed);
      }
      set_gtp_parsing_attrs(&iph->src, &iph->dst, &udph->src_port,
                            &udph->dst_port, (be32_t *)&_const_val,
                            (be32_t *)&_const_val, &iph->protocol, p);
      break;
    }
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

  return FORWARD_GATE;
}
/*----------------------------------------------------------------------------------*/
/* The GTP-U part of Parse(), from the GTP-U header at `gtp_off`. */
gate_idx_t GtpuParser::ParseGtpu(bess::Packet *p, Ipv4 *outer_iph,
                                 size_t gtp_off, const char **malformed) {
  static const uint32_t _const_val = 0xFFFFFFFFu;
  const size_t len = p->head_len();
  char *head = p->head_data<char *>();

  Gtpv1 *gtph = reinterpret_cast<Gtpv1 *>(head + gtp_off);
  const size_t gtp_len = gtph->header_length(len - gtp_off);
  if (gtp_len == 0) {
    *malformed = "GTP-U header";
    return DEFAULT_GATE;
  }

  /* TS 29.281 5.1: version 1, and PT 1 for GTP (0 is GTP'). The spare bit is
   * deliberately not checked: the receiver shall not evaluate it (NOTE 0 of
   * Figure 5.1-1). */
  if (gtph->version != GTPU_VERSION || gtph->pt != GTP_PROTOCOL_TYPE_GTP) {
    *malformed = "GTP-U version or protocol type";
    return DEFAULT_GATE;
  }

  /* Only a G-PDU carries a packet to classify. Any other message on this path
   * (an Error Indication, an End Marker) reaches no PDR. */
  if (gtph->type != GTP_GPDU) {
    return DEFAULT_GATE;
  }

  const size_t inner_off = gtp_off + gtp_len;
  if (inner_off >= len) {
    *malformed = "inner IPv4 header";
    return DEFAULT_GATE;
  }
  Ipv4 *iph = reinterpret_cast<Ipv4 *>(head + inner_off);
  if (iph->version != 4) {
    return DEFAULT_GATE;
  }
  const size_t ihl = iph->header_length << 2;
  if (inner_off + sizeof(Ipv4) > len || ihl < sizeof(Ipv4) ||
      inner_off + ihl > len) {
    *malformed = "inner IPv4 header";
    return DEFAULT_GATE;
  }
  const size_t l4_off = inner_off + ihl;

  be32_t teid = (be32_t)gtph->teid.value();
  /* A fragment after the first carries no transport header (see Parse()). */
  const uint8_t proto = is_later_fragment(iph) ? 0 : iph->protocol;
  switch (proto) {
    case Ipv4::kTcp:
    case Ipv4::kUdp: {
      if (l4_off + 2 * sizeof(be16_t) > len) {
        *malformed = "inner ports";
        return DEFAULT_GATE;
      }
      /* TCP and UDP both start with the source and destination ports. */
      Udp *ports = reinterpret_cast<Udp *>(head + l4_off);
      set_gtp_parsing_attrs(&iph->src, &iph->dst, &ports->src_port,
                            &ports->dst_port, (be32_t *)&teid, &outer_iph->dst,
                            &iph->protocol, p);
      break;
    }
    default:
      /* ESP, later fragments and the rest: no ports to classify on. */
      set_gtp_parsing_attrs(&iph->src, &iph->dst, (be16_t *)&_const_val,
                            (be16_t *)&_const_val, (be32_t *)&teid,
                            &outer_iph->dst, &iph->protocol, p);
      break;
  }

  return FORWARD_GATE;
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
