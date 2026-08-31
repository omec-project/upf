/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright 2022 Intel Corporation
 */

#ifndef BESS_MODULES_GTPU_PATH_MONITORING_H_
#define BESS_MODULES_GTPU_PATH_MONITORING_H_

#include "../module.h"
#include "../pb/module_msg.pb.h"

#include <unordered_map>
#include <vector>

// Updates sequence number by incrementing by 1
class GtpuPathMonitoring final : public Module {
 public:
  static const Commands cmds;

  GtpuPathMonitoring() : Module() { max_allowed_workers_ = 1; }

  void ProcessBatch(Context *ctx, bess::PacketBatch *batch) override;

  CommandResponse Init(const bess::pb::EmptyArg &arg);

  CommandResponse CommandAdd(
      const bess::pb::GtpuPathMonitoringCommandAddDeleteArg &arg);

  CommandResponse CommandDelete(
      const bess::pb::GtpuPathMonitoringCommandAddDeleteArg &arg);

  CommandResponse CommandClear(
      const bess::pb::GtpuPathMonitoringCommandClearArg &arg);

  CommandResponse CommandReadStats(
      const bess::pb::GtpuPathMonitoringCommandReadArg &arg);

 private:
  struct Values {
    uint32_t m_count{0};  // Count number of samples
    uint64_t m_min{std::numeric_limits<uint64_t>::max()};  // Minimum latency
    uint64_t m_mean{0};                                    // Average latency
    uint64_t m_max{0};                                     // Maximum latency
  };

  void Clear();

  std::unordered_map<uint32_t, std::unordered_map<uint16_t, uint64_t>>
      m_storedData;  // Store timestamp {dst_IP, {sequence_number, latency}}
  std::unordered_map<uint32_t, Values> m_latency;  // Store latency per dstIp
  std::unordered_map<uint32_t, uint32_t>
      m_dstIp;              // GTP Tunnel destination {gNB IP, counter}
  uint16_t m_seqNumber{0};  // gtpu echo sequence number
};

#endif  // BESS_MODULES_GTPU_PATH_MONITORING_H_
