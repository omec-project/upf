/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright 2021 Open Networking Foundation
 */

#include "flow_measure.h"

#include <rte_errno.h>
#include <rte_jhash.h>

#include "../core/utils/common.h"

/*----------------------------------------------------------------------------------*/
const Commands FlowMeasure::cmds = {
    {"read", "FlowMeasureCommandReadArg",
     MODULE_CMD_FUNC(&FlowMeasure::CommandReadStats), Command::THREAD_SAFE},
    {"flip", "FlowMeasureCommandFlipArg",
     MODULE_CMD_FUNC(&FlowMeasure::CommandFlipFlag), Command::THREAD_SAFE},
};
/*----------------------------------------------------------------------------------*/
CommandResponse FlowMeasure::Init(const bess::pb::FlowMeasureArg &arg) {
  using AccessMode = bess::metadata::Attribute::AccessMode;
  // Leader module decides which buffer side to use.
  if (arg.leader()) {
    const std::lock_guard<std::mutex> lock(flag_mutex_);
    leader_ = true;
    buffer_flag_attr_id_ = AddMetadataAttr(
        arg.flag_attr_name(), sizeof(uint64_t), AccessMode::kWrite);
    current_flag_value_ = Flag::FLAG_VALUE_A;
  } else {
    leader_ = false;
    buffer_flag_attr_id_ = AddMetadataAttr(arg.flag_attr_name(),
                                           sizeof(uint64_t), AccessMode::kRead);
  }
  if (buffer_flag_attr_id_ < 0)
    return CommandFailure(EINVAL, "invalid flag attribute name");
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

  rte_hash_parameters hash_params = {};
  hash_params.entries = kDefaultNumEntries;
  hash_params.key_len = sizeof(TableKey);
  hash_params.hash_func = rte_jhash;
  hash_params.socket_id = static_cast<int>(rte_socket_id());
  hash_params.extra_flag = RTE_HASH_EXTRA_FLAGS_RW_CONCURRENCY;
  if (arg.entries()) {
    hash_params.entries = arg.entries();
  }
  // Create both hash tables.
  std::string name_a = name() + "Ta" + std::to_string(hash_params.socket_id);
  if (name_a.length() > 26 /*RTE_HASH_NAMESIZE - 1*/) {
    return CommandFailure(EINVAL, "invalid hash name A");
  }
  hash_params.name = name_a.c_str();
  table_a_ = rte_hash_create(&hash_params);
  if (!table_a_) {
    return CommandFailure(rte_errno, "could not create hashmap A");
  }
  std::string name_b = name() + "Tb" + std::to_string(hash_params.socket_id);
  if (name_b.length() > 26 /*RTE_HASH_NAMESIZE - 1*/) {
    return CommandFailure(EINVAL, "invalid hash name B");
  }
  hash_params.name = name_b.c_str();
  table_b_ = rte_hash_create(&hash_params);
  if (!table_b_) {
    return CommandFailure(rte_errno, "could not create hashmap B");
  }

  // resize() would require a copyable object.
  std::vector<SessionStats> tmp_a(hash_params.entries);
  std::vector<SessionStats> tmp_b(hash_params.entries);
  table_data_a_.swap(tmp_a);
  table_data_b_.swap(tmp_b);
  VLOG(1) << name() << ": Tables created successfully.";

  return CommandSuccess();
}
/*----------------------------------------------------------------------------------*/
void FlowMeasure::ProcessBatch(Context *ctx, bess::PacketBatch *batch) {
  uint64_t now_ns = tsc_to_ns(rdtsc());
  for (int i = 0; i < batch->cnt(); ++i) {
    Flag cached_current_flag;
    if (leader_) {
      const std::lock_guard<std::mutex> lock(flag_mutex_);
      set_attr<uint64_t>(this, buffer_flag_attr_id_, batch->pkts()[i],
                         static_cast<uint64_t>(current_flag_value_));
      cached_current_flag = current_flag_value_;
    } else {
      const std::lock_guard<std::mutex> lock(flag_mutex_);
      uint64_t flag =
          get_attr<uint64_t>(this, buffer_flag_attr_id_, batch->pkts()[i]);
      if (!Flag_IsValid(flag)) {
        LOG_EVERY_N(WARNING, 100'001) << "Encountered invalid flag: " << flag;
        continue;
      } else {
        current_flag_value_ = static_cast<Flag>(flag);
        cached_current_flag = current_flag_value_;
      }
    }

    uint64_t ts_ns = get_attr<uint64_t>(this, ts_attr_id_, batch->pkts()[i]);
    uint64_t fseid = get_attr<uint64_t>(this, fseid_attr_id_, batch->pkts()[i]);
    uint32_t pdr = get_attr<uint32_t>(this, pdr_attr_id_, batch->pkts()[i]);
    // Discard invalid timestamps.
    if (!ts_ns || now_ns < ts_ns) {
      continue;
    }
    // Pick current side.
    rte_hash *current_hash = nullptr;
    std::vector<SessionStats> *current_data = nullptr;
    switch (cached_current_flag) {
      case Flag::FLAG_VALUE_A:
        current_hash = table_a_;
        current_data = &table_data_a_;
        break;
      case Flag::FLAG_VALUE_B:
        current_hash = table_b_;
        current_data = &table_data_b_;
        break;
      default:
        LOG_EVERY_N(ERROR, 100'001)
            << "Unknown flag value: " << Flag_Name(cached_current_flag) << ".";
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
                 << key.ToString() << ": " << ret << ", " << rte_strerror(-ret);
      continue;
    }
    // Update stats.
    SessionStats &stat = current_data->at(ret);
    const std::lock_guard<std::mutex> lock(stat.mutex);
    uint64_t diff_ns = now_ns - ts_ns;
    if (stat.last_latency == 0) {
      stat.last_latency = diff_ns;
    }
    uint64_t jitter_ns = absdiff(stat.last_latency, diff_ns);
    stat.last_latency = diff_ns;
    stat.latency_histogram.Insert(diff_ns);
    stat.jitter_histogram.Insert(jitter_ns);
    stat.pkt_count += 1;
    stat.byte_count += batch->pkts()[i]->total_len();
  }

  RunNextModule(ctx, batch);
}
/*----------------------------------------------------------------------------------*/
CommandResponse FlowMeasure::CommandReadStats(
    const bess::pb::FlowMeasureCommandReadArg &arg) {
  Flag flag_to_read = static_cast<Flag>(arg.flag_to_read());
  if (!Flag_IsValid(flag_to_read)) {
    return CommandFailure(EINVAL, "invalid flag value");
  }
  // Cache current flag so we don't block the dataplane while reading the stats.
  Flag cached_current_flag;
  {
    const std::lock_guard<std::mutex> lock(flag_mutex_);
    cached_current_flag = current_flag_value_;
  }
  VLOG(1) << name() << ": " << (leader_ ? "leader" : "follower")
          << " last saw buffer flag " << Flag_Name(cached_current_flag)
          << ", now reading from " << Flag_Name(flag_to_read);
  VLOG_IF(1, cached_current_flag == flag_to_read)
      << name()
      << ": requested to read active buffer flag. Either there is no "
         "traffic or the controller is performing invalid requests.";

  bess::pb::FlowMeasureReadResponse resp;
  auto t_start = std::chrono::high_resolution_clock::now();
  rte_hash *current_hash = nullptr;
  std::vector<SessionStats> *current_data = nullptr;
  switch (flag_to_read) {
    case Flag::FLAG_VALUE_INVALID:
      return CommandSuccess(resp);  // return empty stats when no traffic
    case Flag::FLAG_VALUE_A:
      current_hash = table_a_;
      current_data = &table_data_a_;
      break;
    case Flag::FLAG_VALUE_B:
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
    const TableKey *table_key = reinterpret_cast<const TableKey *>(key);
    const SessionStats &session_stat = current_data->at(ret);
    const std::lock_guard<std::mutex> lock(session_stat.mutex);
    const std::vector<double> lat_percs(arg.latency_percentiles().begin(),
                                        arg.latency_percentiles().end());
    const std::vector<double> jitter_percs(arg.jitter_percentiles().begin(),
                                           arg.jitter_percentiles().end());
    const auto lat_summary =
        session_stat.latency_histogram.Summarize(lat_percs);
    const auto jitter_summary =
        session_stat.jitter_histogram.Summarize(jitter_percs);
    bess::pb::FlowMeasureReadResponse::Statistic stat;
    stat.set_fseid(table_key->fseid);
    stat.set_pdr(table_key->pdr);
    for (const auto &lat_perc : lat_summary.percentile_values) {
      stat.mutable_latency()->add_percentile_values_ns(lat_perc);
    }
    for (const auto &jitter_perc : jitter_summary.percentile_values) {
      stat.mutable_jitter()->add_percentile_values_ns(jitter_perc);
    }
    stat.set_total_packets(session_stat.pkt_count);
    stat.set_total_bytes(session_stat.byte_count);
    *resp.add_statistics() = stat;
  }

  if (arg.clear()) {
    VLOG(1) << name() << ": starting hash table clear...";
    rte_hash_reset(current_hash);
    // TODO: this is quite slow
    VLOG(1) << name() << ": hash table clear done, clearing table data...";
    for (auto &stat : *current_data) {
      const std::lock_guard<std::mutex> lock(stat.mutex);
      stat.reset();
    }
    VLOG(1) << name() << ": table data clear done.";
  }

  auto t_done = std::chrono::high_resolution_clock::now();
  if (VLOG_IS_ON(1)) {
    std::chrono::duration<double> diff = t_done - t_start;
    VLOG(1) << name() << ": CommandReadStats took " << diff.count() << "s.";
  }

  return CommandSuccess(resp);
}

CommandResponse FlowMeasure::CommandFlipFlag(
    const bess::pb::FlowMeasureCommandFlipArg &) {
  // Can only flip the flag if leader module.
  if (!leader_) {
    return CommandFailure(EINVAL, "only leaders can flip the flag");
  }
  // Cache flags so we block the dataplane as short as possible.
  Flag cached_old_flag, cached_current_flag;
  {
    const std::lock_guard<std::mutex> lock(flag_mutex_);
    cached_old_flag = current_flag_value_;
    current_flag_value_ = current_flag_value_ == Flag::FLAG_VALUE_A
                              ? Flag::FLAG_VALUE_B
                              : Flag::FLAG_VALUE_A;
    cached_current_flag = current_flag_value_;
  }
  VLOG(1) << name() << ": leader flipped the buffer flag to "
          << Flag_Name(cached_current_flag);
  bess::pb::FlowMeasureFlipResponse resp;
  resp.set_old_flag(static_cast<uint64_t>(cached_old_flag));
  // Wait for pipeline to flush packets with old flag value.
  std::this_thread::sleep_for(std::chrono::milliseconds(10));

  return CommandSuccess(resp);
}

void FlowMeasure::DeInit() {
  rte_hash_free(table_a_);
  rte_hash_free(table_b_);
}

/*----------------------------------------------------------------------------------*/
ADD_MODULE(FlowMeasure, "qos_measure", "Measures QoS metrics")
