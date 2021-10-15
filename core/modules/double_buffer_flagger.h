/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright(c) 2021 Open Networking Foundation
 */

#ifndef BESS_MODULES_DOUBLEBUFFER_H_
#define BESS_MODULES_DOUBLEBUFFER_H_

#include <mutex>

#include "../module.h"

class DoubleBufferFlagger final : public Module {
 public:
  DoubleBufferFlagger() : flag_attr_id_(-1), current_flag_value_() {
    max_allowed_workers_ = Worker::kMaxWorkers;
  }

  static const Commands cmds;
  CommandResponse Init(const bess::pb::DoubleBufferFlaggerArg &arg);
  void ProcessBatch(Context *ctx, bess::PacketBatch *batch) override;
  std::string GetDesc() const override { return ""; };
  CommandResponse CommandSetNewFlagValue(
      const bess::pb::DoubleBufferCommandSetNewFlagValueArg &arg);
  CommandResponse CommandReadFlagValue(const bess::pb::EmptyArg &arg);

 private:
  static constexpr size_t kMaxAttributeSize = 8;
  mutable std::mutex mutex_;
  int flag_attr_id_;
  bess::pb::BufferFlag current_flag_value_;
};

#endif  // BESS_MODULES_DOUBLEBUFFER_H_
