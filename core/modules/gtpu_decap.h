/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright(c) 2019 Intel Corporation
 */
#ifndef BESS_MODULES_GTPUDECAP_H_
#define BESS_MODULES_GTPUDECAP_H_
/*----------------------------------------------------------------------------------*/
#include "../module.h"
#include "../pb/module_msg.pb.h"
#include "../utils/gtp_common.h"
#include <rte_hash.h>
/*----------------------------------------------------------------------------------*/
class GtpuDecap final : public Module {
 public:
  GtpuDecap() { max_allowed_workers_ = Worker::kMaxWorkers; }

  /* Gates: (0) Default, (1) Forward */
  static const gate_idx_t kNumOGates = 2;

  CommandResponse Init(const bess::pb::GtpuDecapArg &arg);
  void ProcessBatch(Context *ctx, bess::PacketBatch *batch) override;

 private:
  struct rte_hash *session_map = NULL;
};
/*----------------------------------------------------------------------------------*/
#endif  // BESS_MODULES_GTPUDECAP_H_
