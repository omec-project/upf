/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright 2019 Intel Corporation
 */
/* for gtpu_echo decls */
#include "gtpu_echo.h"
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
/* for eth header */
#include "utils/ether.h"
/*----------------------------------------------------------------------------------*/
using bess::utils::be16_t;
using bess::utils::be32_t;
using bess::utils::Ethernet;
using bess::utils::Gtpv1;
using bess::utils::Ipv4;
using bess::utils::ToIpv4Address;
using bess::utils::Udp;

enum { DEFAULT_GATE = 0, FORWARD_GATE };
/*----------------------------------------------------------------------------------*/
bool GtpuEcho::process_echo_request(bess::Packet *p) {
  Ethernet *eth = p->head_data<Ethernet *>();
  Ipv4 *iph = (Ipv4 *)((unsigned char *)eth + sizeof(Ethernet));
  Udp *udp = (Udp *)((unsigned char *)iph + (iph->header_length << 2));
  Gtpv1 *gtph = (Gtpv1 *)((unsigned char *)udp + sizeof(Udp));
  struct gtpu_recovery_ie_t *recovery_ie = NULL;

  /* re-use space (if available) left in Ethernet padding for recovery_ie */
  if ((p->total_len() - (sizeof(Ethernet) + iph->length.value())) >
      sizeof(struct gtpu_recovery_ie_t)) {
    recovery_ie = (struct gtpu_recovery_ie_t *)((char *)gtph + sizeof(Gtpv1) +
                                                gtph->length.value());
  } else {
    /* otherwise prepend payload to the frame */
    recovery_ie = (struct gtpu_recovery_ie_t *)p->append(
        sizeof(struct gtpu_recovery_ie_t));
    if (recovery_ie == NULL) {
      LOG(WARNING) << "Couldn't append " << sizeof(struct gtpu_recovery_ie_t)
                   << " bytes to mbuf";
      return false;
    }
  }

  gtph->type = GTPU_ECHO_RESPONSE;
  gtph->length =
      be16_t(gtph->length.value() + sizeof(struct gtpu_recovery_ie_t));
  recovery_ie->type = GTPU_ECHO_RECOVERY;
  recovery_ie->restart_cntr = 0;

  /* Swap src and dest IP addresses */
  std::swap(iph->src, iph->dst);
  iph->length = be16_t(iph->length.value() + sizeof(struct gtpu_recovery_ie_t));
  /* Reset checksum. This will be computed by next module in line */
  iph->checksum = 0;

  /* Swap src and dst UDP ports */
  std::swap(udp->src_port, udp->dst_port);
  udp->length = be16_t(udp->length.value() + sizeof(struct gtpu_recovery_ie_t));
  /* Reset checksum. This will be computed by next module in line */
  udp->checksum = 0;
  return true;
}
/*----------------------------------------------------------------------------------*/
void GtpuEcho::ProcessBatch(Context *ctx, bess::PacketBatch *batch) {
  int cnt = batch->cnt();
  for (int i = 0; i < cnt; i++) {
    bess::Packet *p = batch->pkts()[i];

    EmitPacket(ctx, p, (process_echo_request(p)) ? FORWARD_GATE : DEFAULT_GATE);
  }
}
/*----------------------------------------------------------------------------------*/
void GtpuEcho::DeInit() {
  /* do nothing */
}
/*----------------------------------------------------------------------------------*/
CommandResponse GtpuEcho::Init(const bess::pb::GtpuEchoArg &arg) {
  s1u_sgw_ip = arg.s1u_sgw_ip();

  if (s1u_sgw_ip == 0)
    return CommandFailure(EINVAL, "Invalid S1U SGW IP address!");

  return CommandSuccess();
}
/*----------------------------------------------------------------------------------*/
ADD_MODULE(GtpuEcho, "gtpu_echo", "first version of gtpu echo module")
