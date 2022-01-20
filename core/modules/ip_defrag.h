/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright 2019 Intel Corporation
 */
#ifndef BESS_MODULES_IPDEFRAG_H_
#define BESS_MODULES_IPDEFRAG_H_
/*----------------------------------------------------------------------------------*/
#include "../module.h"
#include "../pb/module_msg.pb.h"
#include <rte_cycles.h>
#include <rte_ip_frag.h>
/*----------------------------------------------------------------------------------*/
class IPDefrag final : public Module {
 public:
  IPDefrag() { max_allowed_workers_ = Worker::kMaxWorkers; }

  /* Gates: (0) Default, (1) Forward */
  static const gate_idx_t kNumOGates = 2;

  CommandResponse Init(const bess::pb::IPDefragArg &arg);
  void DeInit() override;
  void ProcessBatch(Context *ctx, bess::PacketBatch *batch) override;

 private:
  bess::Packet *IPReassemble(Context *ctx, bess::Packet *p);
  struct rte_ip_frag_tbl *ift = NULL; /* hold frags for reassembly */
  struct rte_ip_frag_death_row
      ifdr;         /* for retiring outdated frags (internal bookkeeping) */
  uint64_t cur_tsc; /* for calculating retiring time */
  uint64_t defrag_cycles;

  /**
   * Max number of flows to maintain
   */
  uint32_t num_flows;

  /**
   * NUMA node where mem shall be allocated for IP frags
   */
  int32_t numa;
};
/*----------------------------------------------------------------------------------*/
#endif  // BESS_MODULES_IPDEFRAG_H_
