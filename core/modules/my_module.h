/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright(c) 2021 Open Networking Foundation
 */

#ifndef BESS_MODULES_MYMODULE_H_
#define BESS_MODULES_MYMODULE_H_

#define ENABLE_MODULE 1

#if ENABLE_MODULE

#include <rte_sched.h>

#include "../module.h"
#include "../pb/module_msg.pb.h"

class MyModule final : public Module {
 public:
  MyModule() : scheduler_(nullptr) {
    max_allowed_workers_ = Worker::kMaxWorkers;
  }

  static const Commands cmds;

  /* Gates: (0) Default, (1) Forward */
  static const gate_idx_t kNumOGates = 2;

  void ProcessBatch(Context *ctx, bess::PacketBatch *batch) override;
  CommandResponse Init(const bess::pb::EmptyArg &arg);

 private:
  int foo_ = 1;
  rte_sched_port *scheduler_;
};

#endif  // BESS_MODULES_MYMODULE_H_

#endif
