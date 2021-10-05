/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright(c) 2021 Open Networking Foundation
 */

#ifndef BESS_MODULES_QOS_MEASURE_H_
#define BESS_MODULES_QOS_MEASURE_H_

#include <unordered_map>

#include "../core/utils/histogram.h"
#include "../module.h"

class QosMeasure final : public Module {
 public:
  struct SessionStats {
    uint64_t pkt_count;
    uint64_t byte_count;
    uint64_t last_latency;
    Histogram<uint64_t> latency_histogram;
    Histogram<uint64_t> jitter_histogram;
    SessionStats()
        : pkt_count(0),
          byte_count(0),
          last_latency(0),
          latency_histogram(/*num buckets*/ 1000, /*bucket width ns*/ 1000),
          jitter_histogram(/*num buckets*/ 1000, /*bucket width ns*/ 1000) {}
  };

  QosMeasure() : ts_attr_id_(-1), fseid_attr_id_(-1), pdr_attr_id_(-1) {
    max_allowed_workers_ = Worker::kMaxWorkers;
  }

  static const Commands cmds;
  CommandResponse Init(const bess::pb::EmptyArg &arg);
  void ProcessBatch(Context *ctx, bess::PacketBatch *batch) override;
  std::string GetDesc() const override { return ""; };
  CommandResponse CommandReadStats(const bess::pb::QosMeasureReadArg &arg);

 private:
  std::unordered_map<uint64_t, SessionStats> stats_;
  int ts_attr_id_;
  int fseid_attr_id_;
  int pdr_attr_id_;
};

#endif  // BESS_MODULES_QOS_MEASURE_H_
