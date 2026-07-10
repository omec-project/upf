/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright 2020 Intel Corporation
 */
/* for ip_frag decls */
#include "ip_frag.h"
/* for rte_zmalloc() */
#include <rte_malloc.h>
/* for be32_t */
#include "utils/endian.h"
/* for ToIpv4Address() */
#include "utils/ip.h"
/* for eth header */
#include "utils/ether.h"
/*----------------------------------------------------------------------------------*/
using bess::utils::be16_t;
using bess::utils::be32_t;
using bess::utils::Ethernet;
using bess::utils::Ipv4;
using bess::utils::ToIpv4Address;

enum { DEFAULT_GATE = 0, FORWARD_GATE };
/*----------------------------------------------------------------------------------*/
const Commands IPFrag::cmds = {{"get_eth_mtu", "EmptyArg",
                                MODULE_CMD_FUNC(&IPFrag::GetEthMTU),
                                Command::THREAD_SAFE}};
/*----------------------------------------------------------------------------------*/
/**
 * Returns NULL under two conditions: (1) if the packet failed to fragment due
 * to e.g., DF bit on and IP4 datagram size > MTU, or (2) if the packet
 * successfully fragmented (new mbufs created) and the original IP4 datagram
 * needs to be freed up. Returns Packet ptr if the packet size < MTU
 */
bess::Packet *IPFrag::FragmentPkt(Context *ctx, bess::Packet *p) {
  struct rte_ether_hdr *ethh = static_cast<struct rte_ether_hdr *>(
      static_cast<void *>(p->head_data<char *>()));
  struct rte_ipv4_hdr *iph =
      (struct rte_ipv4_hdr *)((unsigned char *)ethh +
                              sizeof(struct rte_ether_hdr));
  struct rte_mbuf *m = (struct rte_mbuf *)p;

  if (RTE_ETH_IS_IPV4_HDR(m->packet_type) &&
      unlikely((eth_mtu - RTE_ETHER_CRC_LEN) < p->total_len())) {
    volatile int32_t res;
    struct rte_ether_hdr ethh_copy;
    int32_t j;
    struct rte_mbuf *frag_tbl[BATCH_SIZE];
    unsigned char *orig_ip_payload;
    uint16_t orig_data_offset;

    /* if the datagram is saying not to fragment (DF), we drop the packet */
    if ((iph->fragment_offset & RTE_IPV4_HDR_DF_FLAG) == RTE_IPV4_HDR_DF_FLAG) {
      EmitPacket(ctx, p, DEFAULT_GATE);
      return NULL;
    }

    /* retrieve Ethernet header */
    rte_memcpy(&ethh_copy, ethh, sizeof(struct rte_ether_hdr));

    /* remove the Ethernet header and trailer from the input packet */
    rte_pktmbuf_adj(m, (uint16_t)sizeof(struct rte_ether_hdr));

    /* retrieve orig ip payload for later re-use in ip frags */
    orig_ip_payload = rte_pktmbuf_mtod_offset(m, unsigned char *,
                                              sizeof(struct rte_ipv4_hdr));
    orig_data_offset = 0;

    /* fragment the IPV4 packet */
    res = rte_ipv4_fragment_packet(
        m, &frag_tbl[0], BATCH_SIZE,
        eth_mtu - RTE_ETHER_CRC_LEN - RTE_ETHER_HDR_LEN, m->pool,
        indirect_pktmbuf_pool->pool());

    if (unlikely(res < 0)) {
      EmitPacket(ctx, p, DEFAULT_GATE);
      return NULL;
    } else {
      /* now copy the Ethernet header + IP payload to each frag */
      for (j = 0; j < res; j++) {
        m = frag_tbl[j];
        ethh = (struct rte_ether_hdr *)rte_pktmbuf_prepend(
            m, (uint16_t)sizeof(struct rte_ether_hdr));
        if (ethh == NULL)
          rte_panic("No headroom in mbuf.\n");
        /* remove chained mbufs (as they are not needed) */
        struct rte_mbuf *del_mbuf = m->next;
        while (del_mbuf != NULL) {
          rte_pktmbuf_free_seg(del_mbuf);
          del_mbuf = del_mbuf->next;
        }

        /* setting mbuf metadata */
        m->l2_len = sizeof(struct rte_ether_hdr);
        m->data_len = m->pkt_len;
        m->nb_segs = 1;
        m->next = NULL;
        rte_memcpy(ethh, &ethh_copy, sizeof(struct rte_ether_hdr));

        ethh =
            (struct rte_ether_hdr *)rte_pktmbuf_mtod(m, struct rte_ether_hdr *);
        iph = (struct rte_ipv4_hdr *)(ethh + 1);

        /* copy ip payload */
        unsigned char *ip_payload =
            (unsigned char *)((unsigned char *)iph +
                              ((iph->version_ihl & RTE_IPV4_HDR_IHL_MASK)
                               << 2));
        uint16_t ip_payload_len =
            m->pkt_len - sizeof(struct rte_ether_hdr) -
            ((iph->version_ihl & RTE_IPV4_HDR_IHL_MASK) << 2);

        /* if total frame size is less than minimum transmission unit, add IP
         * padding */
        if (unlikely(ip_payload_len + sizeof(struct rte_ipv4_hdr) +
                         sizeof(struct rte_ether_hdr) + RTE_ETHER_CRC_LEN <
                     RTE_ETHER_MIN_LEN)) {
          /* update ip->ihl first */
          iph->version_ihl &= 0xF0;
          iph->version_ihl |=
              (RTE_IPV4_HDR_IHL_MASK & (PADDED_IPV4_HDR_SIZE >> 2));
          /* update ip->tot_len */
          iph->total_length = ntohs(ip_payload_len + PADDED_IPV4_HDR_SIZE);
          /* update l3_len */
          m->l3_len = PADDED_IPV4_HDR_SIZE;
          /* update data_len & pkt_len */
          m->data_len = m->pkt_len = m->pkt_len + IP_PADDING_LEN;
          /* ip_payload is currently the place you would add 0s */
          memset(ip_payload, 0, IP_PADDING_LEN);

          /* re-set ip_payload to the right `offset` (location) now */
          ip_payload += IP_PADDING_LEN;
        }
        rte_memcpy(ip_payload, orig_ip_payload + orig_data_offset,
                   ip_payload_len);
        orig_data_offset += ip_payload_len;
        iph->hdr_checksum = 0;
        iph->hdr_checksum = rte_ipv4_cksum((struct rte_ipv4_hdr *)iph);
      }
      for (int i = 0; i < res; i++)
        EmitPacket(ctx, (bess::Packet *)frag_tbl[i], FORWARD_GATE);

      /* free original mbuf */
      DropPacket(ctx, p);

      /* all fragments successfully forwarded. Return NULL */
      return NULL;
    }
  }

  return p;
}
/*----------------------------------------------------------------------------------*/
void IPFrag::ProcessBatch(Context *ctx, bess::PacketBatch *batch) {
  int cnt = batch->cnt();
  for (int i = 0; i < cnt; i++) {
    bess::Packet *p = batch->pkts()[i];
    p = FragmentPkt(ctx, p);
    if (p)
      EmitPacket(ctx, p, FORWARD_GATE);
  }
}
/*----------------------------------------------------------------------------------*/
CommandResponse IPFrag::GetEthMTU(const bess::pb::EmptyArg &) {
  bess::pb::IPFragArg arg;
  arg.set_mtu(eth_mtu);
  DLOG(INFO) << "Ethernet MTU Size: " << eth_mtu;
  return CommandSuccess(arg);
}
/*----------------------------------------------------------------------------------*/
void IPFrag::DeInit() {
  if (indirect_pktmbuf_pool != NULL) {
    /* free allocated IP frags */
    delete indirect_pktmbuf_pool;
    indirect_pktmbuf_pool = NULL;
  }
}
/*----------------------------------------------------------------------------------*/
CommandResponse IPFrag::Init(const bess::pb::IPFragArg &arg) {
  eth_mtu = arg.mtu();
  std::string pool_name = this->name() + "_indirect_mbuf_pool";

  if (eth_mtu <= RTE_ETHER_MIN_LEN)
    return CommandFailure(EINVAL, "Invalid MTU size!");

  indirect_pktmbuf_pool = new bess::DpdkPacketPool();
  if (indirect_pktmbuf_pool == NULL)
    return CommandFailure(ENOMEM, "Cannot create indirect mempool!");
  return CommandSuccess();
}
/*----------------------------------------------------------------------------------*/
ADD_MODULE(IPFrag, "ip_frag", "IPv4 Fragmentation module")
