/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright(c) 2019 Intel Corporation
 */
/* for gtpu_encap decls */
#include "gtpu_encap.h"
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
using bess::utils::be16_t;
using bess::utils::be32_t;
using bess::utils::Gtpv1;
using bess::utils::Ipv4;
using bess::utils::ToIpv4Address;
using bess::utils::Udp;

enum { DEFAULT_GATE = 0, FORWARD_GATE };
/*----------------------------------------------------------------------------------*/
// Template for generating UDP packets without data
struct [[gnu::packed]] PacketTemplate {
  Ipv4 iph;
  Udp udph;
  Gtpv1 gtph;

  PacketTemplate() {
    gtph.version = GTPU_VERSION;
    gtph.pt = GTP_PROTOCOL_TYPE_GTP;
    gtph.spare = 0;
    gtph.ex = 0;
    gtph.seq = 0;
    gtph.pdn = 0;
    gtph.type = GTP_GPDU;
    gtph.length = (be16_t)0;  // to fill in
    gtph.teid = (be32_t)0;    // to fill in
    udph.src_port = (be16_t)UDP_PORT_GTPU;
    udph.dst_port = (be16_t)UDP_PORT_GTPU;
    udph.length = (be16_t)0;  // to fill in
    /* calculated by L4Checksum module in line */
    udph.checksum = 0;
    iph.version = IPVERSION;
    iph.header_length = (sizeof(Ipv4) >> 2);
    iph.type_of_service = 0;
    iph.length = (be16_t)0;  // to fill in
    iph.id = (be16_t)0x513;
    iph.fragment_offset = (be16_t)0;
    iph.ttl = 64;
    iph.protocol = IPPROTO_UDP;
    /* calculated by IPChecksum module in line */
    iph.checksum = 0;
    iph.src = (be32_t)0;  // to fill in
    iph.dst = (be32_t)0;  // to fill in
  }
};
static PacketTemplate outer_ip_template;
/*----------------------------------------------------------------------------------*/
void GtpuEncap::ProcessBatch(Context *ctx, bess::PacketBatch *batch) {
  int cnt = batch->cnt();

  for (int i = 0; i < cnt; i++) {
    bess::Packet *p = batch->pkts()[i];

    /* check attributes' values now */
    uint32_t at_tout_sip;
    bess::metadata::mt_offset_t off = attr_offset(tout_sip_attr);
    at_tout_sip = get_attr_with_offset<uint32_t>(off, p);

    uint32_t at_tout_dip;
    off = attr_offset(tout_dip_attr);
    at_tout_dip = get_attr_with_offset<uint32_t>(off, p);

    uint32_t at_tout_teid;
    off = attr_offset(tout_teid);
    at_tout_teid = get_attr_with_offset<uint32_t>(off, p);

    uint16_t at_tout_uport;
    off = attr_offset(tout_uport);
    at_tout_uport = get_attr_with_offset<uint16_t>(off, p);

#if DEBUG
    /* checking values now */
    std::cerr << "Tunnel out sip: " << at_tout_sip
              << ", real: " << data[i]->ul_s1_info.sgw_addr.u.ipv4_addr
              << std::endl;
    std::cerr << "Tunnel out dip: " << (at_tout_dip)
              << ", real: " << data[i]->ul_s1_info.enb_addr.u.ipv4_addr
              << std::endl;
    std::cerr << "Tunnel out teid: " << (at_tout_teid)
              << ", real: " << data[i]->dl_s1_info.enb_teid << std::endl;
    std::cerr << "Tunnel out udp port: " << at_tout_uport
              << ", real: " << UDP_PORT_GTPU << std::endl;
#endif

    /* assuming that this module comes right after EthernetDecap */
    /* pkt_len can be used as the length of IP datagram */
    uint16_t pkt_len = p->total_len();
    Ipv4 *iph = p->head_data<Ipv4 *>();

    /* pre-allocate space for encaped header(s) */
    char *new_p = static_cast<char *>(
        p->prepend(sizeof(Udp) + sizeof(Gtpv1) + sizeof(Ipv4)));
    if (new_p == NULL) {
      /* failed to prepend header space for encaped packet */
      EmitPacket(ctx, p, DEFAULT_GATE);
      DLOG(INFO) << "prepend() failed!" << std::endl;
      continue;
    }

    /* setting GTPU pointer */
    Gtpv1 *gtph = (Gtpv1 *)(new_p + sizeof(Ipv4) + sizeof(Udp));

    /* copying template content */
    bess::utils::Copy(new_p, &outer_ip_template, sizeof(outer_ip_template));

    /* setting gtpu header */
    gtph->length = (be16_t)(pkt_len);
    gtph->teid = (be32_t)(at_tout_teid);

    /* setting outer UDP header */
    Udp *udph = (Udp *)(new_p + sizeof(Ipv4));
    udph->length = (be16_t)(pkt_len + sizeof(Gtpv1) + sizeof(Udp));
    udph->src_port = udph->dst_port = (be16_t)(at_tout_uport);

    /* setting outer IP header */
    iph = (Ipv4 *)(new_p);
    iph->length =
        (be16_t)(pkt_len + sizeof(Gtpv1) + sizeof(Udp) + sizeof(Ipv4));
    iph->src = (be32_t)(at_tout_sip);
    iph->dst = (be32_t)(at_tout_dip);
    EmitPacket(ctx, p, FORWARD_GATE);
  }
}
/*----------------------------------------------------------------------------------*/
CommandResponse GtpuEncap::Init(const bess::pb::EmptyArg &) {

  using AccessMode = bess::metadata::Attribute::AccessMode;
  tout_sip_attr = AddMetadataAttr("tunnel_out_src_ip4addr", sizeof(uint32_t),
				  AccessMode::kRead);
  DLOG(INFO) << "tout_sip_attr: " << tout_sip_attr << std::endl;
  tout_dip_attr = AddMetadataAttr("tunnel_out_dst_ip4addr", sizeof(uint32_t),
                                  AccessMode::kRead);
  DLOG(INFO) << "tout_dip_attr: " << tout_dip_attr << std::endl;
  tout_teid =
	  AddMetadataAttr("tunnel_out_teid", sizeof(uint32_t), AccessMode::kRead);
  DLOG(INFO) << "tout_teid: " << tout_teid << std::endl;
  tout_uport = AddMetadataAttr("tunnel_out_udp_port", sizeof(uint16_t),
			       AccessMode::kRead);
  DLOG(INFO) << "tout_uport: " << tout_uport << std::endl;
  return CommandSuccess();
}
/*----------------------------------------------------------------------------------*/
ADD_MODULE(GtpuEncap, "gtpu_encap", "first version of gtpu encap module")
