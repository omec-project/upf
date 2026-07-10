/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright 2021 Open Networking Foundation
 */

#ifndef BESS_MODULES_QOS_MEASURE_H_
#define BESS_MODULES_QOS_MEASURE_H_

#include <rte_hash.h>

#include <mutex>

#include "../core/utils/histogram.h"
#include "../module.h"

class FlowMeasure final : public Module {
 public:
  FlowMeasure()
      : leader_(false),
        current_flag_value_(),
        table_a_(nullptr),
        table_b_(nullptr),
        ts_attr_id_(-1),
        fseid_attr_id_(-1),
        pdr_attr_id_(-1) {
    // Multi-writer support is not enabled on the hash maps.
    max_allowed_workers_ = 1;
  }

  static constexpr uint32_t kDefaultNumEntries = 1 << 15;
  static const Commands cmds;
  CommandResponse Init(const bess::pb::FlowMeasureArg &arg);
  void DeInit() override;
  void ProcessBatch(Context *ctx, bess::PacketBatch *batch) override;
  std::string GetDesc() const override { return ""; };
  CommandResponse CommandReadStats(
      const bess::pb::FlowMeasureCommandReadArg &arg);
  CommandResponse CommandFlipFlag(
      const bess::pb::FlowMeasureCommandFlipArg &arg);

 private:
  // Flag represents a collection of possible values to select buffer sides.
  enum class Flag {
    FLAG_VALUE_INVALID = 0,
    FLAG_VALUE_A,
    FLAG_VALUE_B,
    FLAG_VALUE_MAX = FLAG_VALUE_B,
  };

  template <typename T>
  static constexpr bool Flag_IsValid(T value) {
    Flag flag = static_cast<Flag>(value);
    return flag > Flag::FLAG_VALUE_INVALID && flag <= Flag::FLAG_VALUE_MAX;
  }

  static const std::string Flag_Name(const Flag &flag) {
    switch (flag) {
      case Flag::FLAG_VALUE_INVALID:
        return "FLAG_VALUE_INVALID";
      case Flag::FLAG_VALUE_A:
        return "FLAG_VALUE_A";
      case Flag::FLAG_VALUE_B:
        return "FLAG_VALUE_B";
      default:
        return "";
    }
  }

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
      ss << "{ fseid: " << fseid << ", pdr: " << pdr << " }";
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
    Histogram<uint64_t> latency_histogram;
    Histogram<uint64_t> jitter_histogram;
    mutable std::mutex mutex;
    static constexpr uint64_t kBucketWidthNs = 1000;  // accuracy: 1 us
    static constexpr uint64_t kNumBuckets = 100;      // range: 0 - 100 us
    SessionStats()
        : pkt_count(0),
          byte_count(0),
          last_latency(0),
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
      latency_histogram.Reset();
      jitter_histogram.Reset();
    }
  };
  bool leader_;
  Flag current_flag_value_;  // protected by flag_mutex_
  mutable std::mutex flag_mutex_;
  rte_hash *table_a_;
  rte_hash *table_b_;
  std::vector<SessionStats> table_data_a_;
  std::vector<SessionStats> table_data_b_;
  int ts_attr_id_;
  int fseid_attr_id_;
  int pdr_attr_id_;
  int buffer_flag_attr_id_;
};

#endif  // BESS_MODULES_QOS_MEASURE_H_
