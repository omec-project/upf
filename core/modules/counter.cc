/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright(c) 2019 Intel Corporation
 */
#include "counter.h"
/* for GetDesc() */
#include "utils/format.h"
/* for endian functions */
#include <arpa/inet.h>
/*----------------------------------------------------------------------------------*/
const Commands Counter::cmds = {
    {"add", "CounterAddArg", MODULE_CMD_FUNC(&Counter::AddCounter),
     Command::THREAD_SAFE},
    {"remove", "CounterRemoveArg", MODULE_CMD_FUNC(&Counter::RemoveCounter),
     Command::THREAD_SAFE}};

enum Direction {UPLINK = 0x1, DOWNLINK = 0x2};
/*----------------------------------------------------------------------------------*/
CommandResponse Counter::AddCounter(const bess::pb::CounterAddArg &arg) {
  uint32_t ctr_id = arg.ctr_id();

  /* check_exist is still here for over-protection */
  if (counters.find(ctr_id) == counters.end()) {
    SessionStats s = {.ul_pkt_count = 0, .ul_byte_count = 0,
		      .dl_pkt_count = 0, .dl_byte_count = 0};
    counters.insert(std::pair<uint32_t, SessionStats>(ctr_id, s));
  } else
    return CommandFailure(EINVAL, "Unable to add ctr");
  return CommandSuccess();
}
/*----------------------------------------------------------------------------------*/
CommandResponse Counter::RemoveCounter(const bess::pb::CounterRemoveArg &arg) {
  uint32_t ctr_id = arg.ctr_id();

  /* check_exist is still here for over-protection */
  if (counters.find(ctr_id) != counters.end()) {
    std::cerr << this->name() << "[" << ctr_id
              << "]: UL -> " << counters[ctr_id].ul_pkt_count << ", "
              << counters[ctr_id].ul_byte_count
	      << "\t DL -> " << counters[ctr_id].dl_pkt_count << ", "
	      << counters[ctr_id].dl_byte_count
	      << std::endl;
    counters.erase(ctr_id);
  } else {
    return CommandFailure(EINVAL, "Unable to remove ctr");
  }
  return CommandSuccess();
}
/*----------------------------------------------------------------------------------*/
CommandResponse Counter::Init(const bess::pb::CounterArg &arg) {
  static const char *direct_str = "direction";
  name_id = arg.name_id();
  if (name_id == "")
    return CommandFailure(EINVAL, "Invalid counter idx name");
  check_exist = arg.check_exist();

  using AccessMode = bess::metadata::Attribute::AccessMode;
  dir_attr_id = AddMetadataAttr(direct_str, sizeof(uint32_t), AccessMode::kRead);
  ctr_attr_id = AddMetadataAttr(name_id, sizeof(uint32_t), AccessMode::kRead);

  return CommandSuccess();
}
/*----------------------------------------------------------------------------------*/
void Counter::ProcessBatch(Context *ctx, bess::PacketBatch *batch) {
  int cnt = batch->cnt();

  for (int i = 0; i < cnt; i++) {
    uint32_t ctr_id = get_attr<uint32_t>(this, ctr_attr_id, batch->pkts()[i]);
    uint32_t dir_id = get_attr<uint32_t>(this, dir_attr_id, batch->pkts()[i]);

    // check if ctr_id is present
    if (!check_exist || counters.find(ctr_id) != counters.end()) {
      SessionStats s = counters[ctr_id];
      void *mt_ptr =
	_ptr_attr_with_offset<uint32_t>(attr_offset(dir_attr_id), batch->pkts()[i]);
      bess::utils::CopySmall(mt_ptr, &dir_id, 4);
      dir_id = ntohl(dir_id);
      if (dir_id == UPLINK) {
	s.ul_pkt_count += 1;
	s.ul_byte_count += batch->pkts()[i]->total_len();
      } else if (dir_id == DOWNLINK) { /* dir_id == DOWNLINK */
	s.dl_pkt_count += 1;
	s.dl_byte_count += batch->pkts()[i]->total_len();
      } else {
	DLOG(INFO) << "Direction: " << dir_id << std::endl;
      }
      counters.erase(ctr_id);
      counters.insert(std::pair<uint32_t, SessionStats>(ctr_id, s));
    }
  }

  RunNextModule(ctx, batch);
}
/*----------------------------------------------------------------------------------*/
std::string Counter::GetDesc() const {
  return bess::utils::Format("%zu sessions", (size_t)counters.size());
}
/*----------------------------------------------------------------------------------*/
ADD_MODULE(Counter, "counter",
           "Counts the number of packets/bytes in the UP4 pipeline")
