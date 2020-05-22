// Copyright (c) 2014-2016, The Regents of the University of California.
// Copyright (c) 2016-2017, Nefeli Networks, Inc.
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:
//
// * Redistributions of source code must retain the above copyright notice, this
// list of conditions and the following disclaimer.
//
// * Redistributions in binary form must reproduce the above copyright notice,
// this list of conditions and the following disclaimer in the documentation
// and/or other materials provided with the distribution.
//
// * Neither the names of the copyright holders nor the names of their
// contributors may be used to endorse or promote products derived from this
// software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
// AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
// IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
// ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE
// LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
// CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
// SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
// INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
// CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
// ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
// POSSIBILITY OF SUCH DAMAGE.

#include "counter.h"
/*----------------------------------------------------------------------------------*/
const Commands Counter::cmds = {
    {"add", "CounterAddArg", MODULE_CMD_FUNC(&Counter::AddCounter),
     Command::THREAD_SAFE},
    {"remove", "CounterRemoveArg", MODULE_CMD_FUNC(&Counter::RemoveCounter),
     Command::THREAD_SAFE}};
/*----------------------------------------------------------------------------------*/
CommandResponse Counter::AddCounter(const bess::pb::CounterAddArg &arg) {
  uint32_t ctr_id = arg.ctr_id();

  if (list.find(ctr_id) != list.end()) {
    SessionStats s = {.pkt_count = 0, .byte_count = 0};
    list[ctr_id] = s;
  } else
    return CommandFailure(EINVAL, "Unable to add ctr");
  return CommandSuccess();
}
/*----------------------------------------------------------------------------------*/
CommandResponse Counter::RemoveCounter(const bess::pb::CounterRemoveArg &arg) {
  uint32_t ctr_id = arg.ctr_id();

  if (list.find(ctr_id) != list.end()) {
    list.erase(ctr_id);
  } else {
    return CommandFailure(EINVAL, "Unable to remove ctr");
  }
  return CommandSuccess();
}
/*----------------------------------------------------------------------------------*/
CommandResponse Counter::Init(const bess::pb::CounterArg &arg) {

  std::string name_id = arg.name_id();
  if (name_id == "")
    return CommandFailure(EINVAL, "Invalid counter idx name");
  using AccessMode = bess::metadata::Attribute::AccessMode;
  attr_id = AddMetadataAttr(name_id, sizeof(uint32_t), AccessMode::kRead);

  return CommandSuccess();
}
/*----------------------------------------------------------------------------------*/
void Counter::ProcessBatch(Context *ctx, bess::PacketBatch *batch) {
  int cnt = batch->cnt();

  for (int i = 0; i < cnt; i++) {
    int ctr_id = get_attr<uint32_t>(this, attr_id, batch->pkts()[i]);
    // check if ctr_id is present
    if (list.find(ctr_id) != list.end()) {
      list[ctr_id].pkt_count += 1;
      list[ctr_id].byte_count += batch->pkts()[i]->total_len();
    }
  }

  RunNextModule(ctx, batch);
}
/*----------------------------------------------------------------------------------*/
ADD_MODULE(Counter, "counter",
           "Counts the number of packets/bytes in the UP4 pipeline")
