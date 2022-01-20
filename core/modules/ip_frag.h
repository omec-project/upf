/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright 2020 Intel Corporation
 */
#ifndef BESS_MODULES_IPFRAG_H_
#define BESS_MODULES_IPFRAG_H_
/*----------------------------------------------------------------------------------*/
#include <rte_cycles.h>
#include <rte_ip_frag.h>
/* for ipv4 header */
#include <rte_ip.h>
/* for RTE_ETHER macros */
#include "../module.h"
#include "../pb/module_msg.pb.h"
#include "rte_ether.h"
/*----------------------------------------------------------------------------------*/
/**
 * RX_NUM_DESC < 1024:
 * But increased sensivity kernel packet processing core sched jitters
 */
#define RX_NUM_DESC 2048

/**
 * macro to config tx ring size.
 */
#define TX_NUM_DESC RX_NUM_DESC

/**
 * macro to set the batch size
 */
#define BATCH_SIZE 64

/**
 * DPDK default value optimial.
 */
#define MBUF_CACHE_SIZE 512

#define IP_PADDING_LEN 28
#define PADDED_IPV4_HDR_SIZE (sizeof(struct rte_ipv4_hdr) + IP_PADDING_LEN)
/**
 * NUM_MBUFS >= 2x RX_NUM_DESC::
 *              Else rte_eth_dev_start(...) { FAIL; ...}
 *      NUM_MBUFS >= 1.5x MBUF_CACHE_SIZE::
 *              Else rte_pktmbuf_pool_create(...) { FAIL; ...}
 */
#define NUM_MBUFS                                                 \
  (TX_NUM_DESC * 2) > (1.5 * MBUF_CACHE_SIZE) ? (TX_NUM_DESC * 2) \
                                              : (2 * MBUF_CACHE_SIZE)
/*----------------------------------------------------------------------------------*/
class IPFrag final : public Module {
 public:
  IPFrag() { max_allowed_workers_ = Worker::kMaxWorkers; }

  /* Gates: (0) Default, (1) Forward */
  static const gate_idx_t kNumOGates = 2;
  static const Commands cmds;

  CommandResponse Init(const bess::pb::IPFragArg &arg);
  void DeInit() override;
  void ProcessBatch(Context *ctx, bess::PacketBatch *batch) override;
  CommandResponse GetEthMTU(const bess::pb::EmptyArg &);

 private:
  bess::Packet *FragmentPkt(Context *ctx, bess::Packet *p);
  bess::DpdkPacketPool *indirect_pktmbuf_pool = NULL;
  int eth_mtu = RTE_ETHER_MAX_LEN;
};
/*----------------------------------------------------------------------------------*/
#endif  // BESS_MODULES_IPFRAG_H_
