/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright 2019 Intel Corporation
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
  Counter() : counters() { max_allowed_workers_ = Worker::kMaxWorkers; }

  static const Commands cmds;
  CommandResponse AddCounter(const bess::pb::CounterAddArg &arg);
  CommandResponse RemoveCounter(const bess::pb::CounterRemoveArg &arg);
  CommandResponse RemoveAllCounters(const bess::pb::EmptyArg &);
  CommandResponse Init(const bess::pb::CounterArg &arg);
  void ProcessBatch(Context *ctx, bess::PacketBatch *batch) override;
  // returns the number of active UE sessions
  std::string GetDesc() const override;

 private:
#ifdef HASHMAP_BASED
  std::map<uint32_t, SessionStats> counters;
#else
  SessionStats *counters;
  uint32_t curr_count;
#endif
  std::string name_id;
  bool check_exist;
  int ctr_attr_id;
  uint32_t total_count;
};

#endif  // BESS_MODULES_COUNTER_H_
