/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright 2019 Intel Corporation
 */
#ifndef BESS_MODULES_GTPUDECAP_H_
#define BESS_MODULES_GTPUDECAP_H_
/*----------------------------------------------------------------------------------*/
#include "../module.h"
#include "../pb/module_msg.pb.h"
#include "gtpu_drop_log.h"
#include <rte_hash.h>
/*----------------------------------------------------------------------------------*/
class GtpuDecap final : public Module {
 public:
  GtpuDecap() { max_allowed_workers_ = Worker::kMaxWorkers; }

  void ProcessBatch(Context *ctx, bess::PacketBatch *batch) override;

 private:
  size_t OuterHeaderLength(bess::Packet *p, const char **malformed);
  GtpuDropLog drop_log_;
};
/*----------------------------------------------------------------------------------*/
#endif  // BESS_MODULES_GTPUDECAP_H_
