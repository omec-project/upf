/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright(c) 2019 Intel Corporation
 */

#ifndef BESS_MODULES_COUNTER_H_
#define BESS_MODULES_COUNTER_H_

#include "../module.h"
#include <map>

struct SessionStats {
  uint64_t pkt_count;
  uint64_t byte_count;
};

class Counter final : public Module {
 public:
  Counter() : counters() {}

  static const Commands cmds;
  CommandResponse AddCounter(const bess::pb::CounterAddArg &arg);
  CommandResponse RemoveCounter(const bess::pb::CounterRemoveArg &arg);
  CommandResponse Init(const bess::pb::CounterArg &arg);
  void ProcessBatch(Context *ctx, bess::PacketBatch *batch) override;
  // returns the number of active UE sessions
  std::string GetDesc() const override;

 private:
  std::map<uint32_t, SessionStats> counters;
  std::string name_id;
  bool check_exist;
  int ctr_attr_id;
};

#endif  // BESS_MODULES_COUNTER_H_
