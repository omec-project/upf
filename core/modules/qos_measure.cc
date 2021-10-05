/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright(c) 2021 Open Networking Foundation
 */

#include "qos_measure.h"

#include "../core/utils/common.h"

/*----------------------------------------------------------------------------------*/
const Commands QosMeasure::cmds = {
    {"read", "QosMeasureReadArg",
     MODULE_CMD_FUNC(&QosMeasure::CommandReadStats), Command::THREAD_UNSAFE},
};
/*----------------------------------------------------------------------------------*/
CommandResponse QosMeasure::Init(const bess::pb::EmptyArg &arg) {
  (void)arg;
  using AccessMode = bess::metadata::Attribute::AccessMode;
  ts_attr_id_ =
      AddMetadataAttr("timestamp", sizeof(uint64_t), AccessMode::kRead);
  if (ts_attr_id_ < 0)
    return CommandFailure(EINVAL, "invalid metadata declaration");
  fseid_attr_id_ =
      AddMetadataAttr("fseid", sizeof(uint64_t), AccessMode::kRead);
  if (fseid_attr_id_ < 0)
    return CommandFailure(EINVAL, "invalid metadata declaration");
  pdr_attr_id_ = AddMetadataAttr("pdr_id", sizeof(uint32_t), AccessMode::kRead);
  if (pdr_attr_id_ < 0)
    return CommandFailure(EINVAL, "invalid metadata declaration");
  LOG(INFO) << "TS attr ID " << ts_attr_id_;
  LOG(INFO) << "FSEID attr ID " << fseid_attr_id_;
  LOG(INFO) << "PDR attr ID " << pdr_attr_id_;

  const auto &attrs = all_attrs();
  for (const auto &attribute : attrs) {
    LOG(WARNING) << attribute.name << ": size " << attribute.size
                 << " access mode " << static_cast<int>(attribute.mode)
                 << " scope id " << attribute.scope_id;
  }

  return CommandSuccess();
}
/*----------------------------------------------------------------------------------*/
namespace {
std::string SummaryToString(const Histogram<uint64_t>::Summary &summary) {
  std::stringstream ss;
  ss << "count " << summary.count << ", above_range " << summary.above_range
     << ", avg " << summary.avg << ", total " << summary.total;
  for (const auto percentile : summary.percentile_values) {
    ss << "\n\tpercentile: " << percentile;
  }
  return ss.str();
}
}  // namespace

void QosMeasure::ProcessBatch(Context *ctx, bess::PacketBatch *batch) {
  uint64_t now_ns = tsc_to_ns(rdtsc());
  for (int i = 0; i < batch->cnt(); ++i) {
    uint64_t ts_ns = get_attr<uint64_t>(this, ts_attr_id_, batch->pkts()[i]);
    uint64_t fseid = get_attr<uint64_t>(this, fseid_attr_id_, batch->pkts()[i]);
    uint32_t pdr = get_attr<uint32_t>(this, pdr_attr_id_, batch->pkts()[i]);
    // Discard invalid timestamps.
    if (now_ns < ts_ns) {
      continue;
    }
    uint64_t diff = now_ns - ts_ns;
    uint64_t index = fseid ^ static_cast<uint64_t>(pdr);
    SessionStats &stat = stats_[index];
    if (stat.last_latency == 0) {
      stat.last_latency = diff;
    }
    uint64_t jitter = absdiff(stat.last_latency, diff);
    stat.last_latency = diff;
    stat.latency_histogram.Insert(diff);
    stat.jitter_histogram.Insert(jitter);
    stat.pkt_count += 1;
    stat.byte_count += batch->pkts()[i]->total_len();
    LOG_EVERY_N(WARNING, 100'000) << diff << ", " << fseid << ", " << pdr;
  }

  RunNextModule(ctx, batch);
}
/*----------------------------------------------------------------------------------*/
// command module qosMeasure read QosMeasureReadArg {'clear': False}
CommandResponse QosMeasure::CommandReadStats(
    const bess::pb::QosMeasureReadArg &arg) {
  bess::pb::QosMeasureReadResponse resp;
  for (auto &e : stats_) {
    const auto &index = e.first;
    SessionStats &session_stat = e.second;
    bess::pb::QosMeasureReadResponse::Statistic stat;
    const auto lat_summary =
        session_stat.latency_histogram.Summarize({50., 90., 99., 99.9});
    const auto jitter_summary =
        session_stat.jitter_histogram.Summarize({50., 90., 99., 99.9});
    LOG(WARNING) << SummaryToString(lat_summary);
    stat.set_fseid(index);
    stat.set_latency_50_ns(lat_summary.percentile_values[0]);
    stat.set_latency_90_ns(lat_summary.percentile_values[1]);
    stat.set_latency_99_ns(lat_summary.percentile_values[2]);
    stat.set_latency_99_9_ns(lat_summary.percentile_values[3]);
    stat.set_jitter_50_ns(jitter_summary.percentile_values[0]);
    stat.set_jitter_90_ns(jitter_summary.percentile_values[1]);
    stat.set_jitter_99_ns(jitter_summary.percentile_values[2]);
    stat.set_jitter_99_9_ns(jitter_summary.percentile_values[3]);
    stat.set_total_packets(session_stat.pkt_count);
    stat.set_total_bytes(session_stat.byte_count);
    *resp.add_statistics() = stat;
    if (arg.clear()) {
      session_stat.pkt_count = 0;
      session_stat.byte_count = 0;
      session_stat.last_latency = 0;
      session_stat.latency_histogram.Reset();
      session_stat.jitter_histogram.Reset();
    }
  }

  return CommandSuccess(resp);
}
/*----------------------------------------------------------------------------------*/
ADD_MODULE(QosMeasure, "qos_measure", "Measures QoS metrics")
