/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright(c) 2021 Open Networking Foundation
 */

#ifndef BESS_MODULES_QOS_MEASURE_H_
#define BESS_MODULES_QOS_MEASURE_H_

#include <rte_hash.h>

#include <mutex>

#include "../core/utils/histogram.h"
#include "../module.h"

class QosMeasure final : public Module {
 public:
  QosMeasure()
      : table_a_(nullptr),
        table_b_(nullptr),
        ts_attr_id_(-1),
        fseid_attr_id_(-1),
        pdr_attr_id_(-1) {
    // Multi-writer support is not enabled on the hash maps.
    max_allowed_workers_ = 1;
  }

  static constexpr uint32_t kMaxNumEntries = 1 << 15;
  static const Commands cmds;
  CommandResponse Init(const bess::pb::QosMeasureArg &arg);
  void ProcessBatch(Context *ctx, bess::PacketBatch *batch) override;
  std::string GetDesc() const override { return ""; };
  CommandResponse CommandReadStats(
      const bess::pb::QosMeasureCommandReadArg &arg);

 private:
  // TableKey encapsulates all information used to identify a flow and is used
  // as the lookup key in the hash tables. It is packed and aligned to
  // calculating a hash over the raw bytes of the struct is ok.
  struct __attribute__((packed, aligned(16))) TableKey {
    uint64_t fseid;
    uint64_t pdr;
    TableKey(uint64_t fseid, uint64_t pdr) : fseid(fseid), pdr(pdr) {}
    TableKey() : fseid(0), pdr(0) {}
    std::string ToString() const {
      std::stringstream ss;
      ss << "{ fseid: " << fseid << ", pdr: " << pdr + " }";
      return ss.str();
    }
  };
  static_assert(std::is_trivially_copyable<TableKey>::value,
                "TableKey must be is_trivially_copyable.");

  // SessionStats ...
  struct SessionStats {
    uint64_t pkt_count;
    uint64_t byte_count;
    uint64_t last_latency;
    uint64_t last_clear_time;
    Histogram<uint64_t> latency_histogram;
    Histogram<uint64_t> jitter_histogram;
    mutable std::mutex mutex;
    static constexpr uint64_t kBucketWidthNs = 1000;  // accuracy: 1 us
    static constexpr uint64_t kNumBuckets = 100;      // range: 0 - 100 us
    SessionStats()
        : pkt_count(0),
          byte_count(0),
          last_latency(0),
          last_clear_time(tsc_to_ns(rdtsc())),
          latency_histogram(kNumBuckets, kBucketWidthNs),
          jitter_histogram(kNumBuckets, kBucketWidthNs) {}
    // Move allowed, copy not allowed.
    SessionStats(const SessionStats &) = delete;
    SessionStats(SessionStats &&) noexcept = default;
    SessionStats &operator=(const SessionStats &) = delete;
    SessionStats &operator=(SessionStats &&) = default;
    void reset() {
      pkt_count = 0;
      byte_count = 0;
      last_latency = 0;
      last_clear_time = tsc_to_ns(rdtsc());
      latency_histogram.Reset();
      jitter_histogram.Reset();
    }
  };
  rte_hash *table_a_;
  rte_hash *table_b_;
  std::vector<SessionStats> table_data_a_;
  std::vector<SessionStats> table_data_b_;
  int ts_attr_id_;
  int fseid_attr_id_;
  int pdr_attr_id_;
};

#endif  // BESS_MODULES_QOS_MEASURE_H_
