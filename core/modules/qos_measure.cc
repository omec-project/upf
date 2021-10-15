/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright(c) 2021 Open Networking Foundation
 */

#include "qos_measure.h"

#include <rte_errno.h>
#include <rte_jhash.h>

#include "../core/utils/common.h"

/*----------------------------------------------------------------------------------*/
const Commands QosMeasure::cmds = {
    {"read", "QosMeasureCommandReadArg",
     MODULE_CMD_FUNC(&QosMeasure::CommandReadStats), Command::THREAD_SAFE},
};
/*----------------------------------------------------------------------------------*/
CommandResponse QosMeasure::Init(const bess::pb::QosMeasureArg &arg) {
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
  buffer_flag_attr_id_ = AddMetadataAttr(arg.flag_attr_name(), sizeof(uint64_t),
                                         AccessMode::kRead);
  if (buffer_flag_attr_id_ < 0)
    return CommandFailure(EINVAL, "invalid flag attribute name");

  LOG(INFO) << "TS attr ID " << ts_attr_id_;
  LOG(INFO) << "FSEID attr ID " << fseid_attr_id_;
  LOG(INFO) << "PDR attr ID " << pdr_attr_id_;

  rte_hash_parameters hash_params = {};
  hash_params.entries = kDefaultNumEntries;
  hash_params.key_len = sizeof(TableKey);
  hash_params.hash_func = rte_jhash;
  hash_params.socket_id = static_cast<int>(rte_socket_id());
  hash_params.extra_flag = RTE_HASH_EXTRA_FLAGS_RW_CONCURRENCY;
  if (arg.entries()) {
    hash_params.entries = arg.entries();
  }
  // Find existing tables or create new ones.
  std::string name_a =
      name() + "_table_a_" + std::to_string(hash_params.socket_id);
  if (name_a.size() > RTE_HASH_NAMESIZE - 1) {
    return CommandFailure(EINVAL, "invalid hash name");
  }
  hash_params.name = name_a.c_str();
  VLOG(1) << "Finding table " << hash_params.name << " ...";
  // TODO: only needed in global design, remove otherwise.
  table_a_ = rte_hash_find_existing(hash_params.name);
  if (!table_a_) {
    VLOG(1) << "Table " << hash_params.name << " does not exist, creating it.";
    table_a_ = rte_hash_create(&hash_params);
  }
  if (!table_a_) {
    return CommandFailure(rte_errno, "could not create hashmap");
  }
  std::string name_b =
      name() + "_table_b_" + std::to_string(hash_params.socket_id);
  hash_params.name = name_b.c_str();
  if (name_b.size() > RTE_HASH_NAMESIZE - 1) {
    return CommandFailure(EINVAL, "invalid hash name");
  }
  VLOG(1) << "Finding table " << hash_params.name << " ...";
  // TODO: only needed in global design, remove otherwise.
  table_b_ = rte_hash_find_existing(hash_params.name);
  if (!table_b_) {
    VLOG(1) << "Table " << hash_params.name << " does not exist, creating it.";
    table_b_ = rte_hash_create(&hash_params);
  }
  if (!table_b_) {
    return CommandFailure(rte_errno, "could not create hashmap");
  }
  LOG(INFO) << "TableKey size: " << sizeof(TableKey)
            << ", SessionStats size: " << sizeof(SessionStats) << ".";

  // resize() would require a copyable object.
  std::vector<SessionStats> tmp_a(hash_params.entries);
  std::vector<SessionStats> tmp_b(hash_params.entries);
  table_data_a_.swap(tmp_a);
  table_data_b_.swap(tmp_b);
  LOG(INFO) << "Tables created successfully.";

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
    int32_t flag =
        get_attr<uint64_t>(this, buffer_flag_attr_id_, batch->pkts()[i]);
    if (!bess::pb::BufferFlag_IsValid(flag)) {
      LOG_EVERY_N(WARNING, 100'001) << "Encountered invalid flag: " << flag;
      continue;
    }
    auto f = bess::pb::BufferFlag(flag);
    LOG_EVERY_N(WARNING, 100'001) << "flag: " << bess::pb::BufferFlag_Name(f);
    // Discard invalid timestamps.
    if (!ts_ns || now_ns < ts_ns) {
      continue;
    }
    uint64_t diff_ns = now_ns - ts_ns;
    // Pick current side.
    rte_hash *current_hash = nullptr;
    std::vector<SessionStats> *current_data = nullptr;
    switch (flag) {
      case bess::pb::BufferFlag::FLAG_VALUE_A:
        current_hash = table_a_;
        current_data = &table_data_a_;
        break;
      case bess::pb::BufferFlag::FLAG_VALUE_B:
        current_hash = table_b_;
        current_data = &table_data_b_;
        break;
      default:
        LOG_EVERY_N(ERROR, 100'001) << "Unknown flag value: " << flag << ".";
        continue;
    }
    // Find or create session.
    TableKey key(fseid, pdr);
    int32_t ret = rte_hash_lookup(current_hash, &key);
    if (ret == -ENOENT) {
      ret = rte_hash_add_key(current_hash, &key);
    }
    if (ret < 0) {
      LOG(ERROR) << "Failed to lookup or insert session stats for key "
                 << key.ToString() << ": " << ret << ", " << rte_strerror(ret);
      continue;
    }
    LOG_EVERY_N(WARNING, 100'001) << "rte_hash_lookup/insert = " << ret;
    // Update stats.
    SessionStats &stat = current_data->at(ret);
    const std::lock_guard<std::mutex> lock(stat.mutex);
    if (stat.last_latency == 0) {
      stat.last_latency = diff_ns;
    }
    uint64_t jitter_ns = absdiff(stat.last_latency, diff_ns);
    stat.last_latency = diff_ns;
    stat.latency_histogram.Insert(diff_ns);
    stat.jitter_histogram.Insert(jitter_ns);
    stat.pkt_count += 1;
    stat.byte_count += batch->pkts()[i]->total_len();
    LOG_EVERY_N(WARNING, 100'001)
        << "FSEID: " << fseid << ", PDR: " << pdr << ": " << diff_ns << "ns.";
  }

  RunNextModule(ctx, batch);
}
/*----------------------------------------------------------------------------------*/
// command module qosMeasureOut read QosMeasureCommandReadArg {'clear': False}
CommandResponse QosMeasure::CommandReadStats(
    const bess::pb::QosMeasureCommandReadArg &arg) {
  bess::pb::QosMeasureReadResponse resp;
  auto t_start = std::chrono::high_resolution_clock::now();
  rte_hash *current_hash = nullptr;
  std::vector<SessionStats> *current_data = nullptr;
  switch (arg.flag()) {
    case bess::pb::BufferFlag::FLAG_VALUE_A:
      current_hash = table_a_;
      current_data = &table_data_a_;
      break;
    case bess::pb::BufferFlag::FLAG_VALUE_B:
      current_hash = table_b_;
      current_data = &table_data_b_;
      break;
    default:
      return CommandFailure(EINVAL, "invalid flag value");
  }
  const void *key = nullptr;
  void *data = nullptr;
  uint32_t next = 0;
  int32_t ret = 0;
  while (ret = rte_hash_iterate(current_hash, &key, &data, &next), ret >= 0) {
    LOG_EVERY_N(WARNING, 1'001)
        << ret << ", key " << key << ", data " << data << ", next " << next;
    const TableKey *table_key = reinterpret_cast<const TableKey *>(key);
    const SessionStats &session_stat = current_data->at(ret);
    const std::lock_guard<std::mutex> lock(session_stat.mutex);
    const auto lat_summary =
        session_stat.latency_histogram.Summarize({50., 90., 99., 99.9});
    const auto jitter_summary =
        session_stat.jitter_histogram.Summarize({50., 90., 99., 99.9});
    LOG_EVERY_N(WARNING, 1'001) << SummaryToString(lat_summary) << ".";
    bess::pb::QosMeasureReadResponse::Statistic stat;
    stat.set_fseid(table_key->fseid);
    stat.set_pdr(table_key->pdr);
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
  }

  if (arg.clear()) {
    LOG(WARNING) << "starting hash table clear...";
    rte_hash_reset(current_hash);
    // TODO: this is quite slow
    LOG(WARNING) << "hash table clear done, clearing table data...";
    for (auto &stat : *current_data) {
      const std::lock_guard<std::mutex> lock(stat.mutex);
      stat.reset();
    }
    LOG(WARNING) << "table data clear done.";
  }

  auto t_done = std::chrono::high_resolution_clock::now();
  if (VLOG_IS_ON(1)) {
    std::chrono::duration<double> diff = t_done - t_start;
    VLOG(1) << "CommandReadStats took " << diff.count() << "s.";
  }

  return CommandSuccess(resp);
}
/*----------------------------------------------------------------------------------*/
ADD_MODULE(QosMeasure, "qos_measure", "Measures QoS metrics")
