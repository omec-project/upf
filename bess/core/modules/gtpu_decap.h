/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright 2019 Intel Corporation
 */
#ifndef BESS_MODULES_GTPUDECAP_H_
#define BESS_MODULES_GTPUDECAP_H_
/*----------------------------------------------------------------------------------*/
#include "../module.h"
#include "../pb/module_msg.pb.h"
#include <rte_hash.h>
/*----------------------------------------------------------------------------------*/
class GtpuDecap final : public Module {
 public:
  GtpuDecap() { max_allowed_workers_ = Worker::kMaxWorkers; }

  void ProcessBatch(Context *ctx, bess::PacketBatch *batch) override;
};
/*----------------------------------------------------------------------------------*/
#endif  // BESS_MODULES_GTPUDECAP_H_
