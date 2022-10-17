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

#include "track.h"

#include "../message.h"

// Ethernet overhead in bytes
static const size_t kEthernetOverhead = 24;

const std::string Track::kName = "Track";

const GateHookCommands Track::cmds = {{"reset", "EmptyArg",
                                       GATE_HOOK_CMD_FUNC(&Track::CommandReset),
                                       GateHookCommand::THREAD_UNSAFE}};

Track::Track()
    : bess::GateHook(Track::kName, "track", Track::kPriority),
      track_bytes_(),
      worker_stats_() {}

CommandResponse Track::Init(const bess::Gate *, const bess::pb::TrackArg &arg) {
  track_bytes_ = arg.bits();
  return CommandSuccess();
}

CommandResponse Track::CommandReset(const bess::pb::EmptyArg &) {
  worker_stats_ = {};
  return CommandSuccess();
}

void Track::ProcessBatch(const bess::PacketBatch *batch) {
  TrackStats *stat = &worker_stats_[current_worker.wid()];

  size_t cnt = batch->cnt();
  stat->cnt += 1;
  stat->pkts += cnt;

  if (!track_bytes_) {
    return;
  }

  uint64_t bytes = 0;
  for (size_t i = 0; i < cnt; i++) {
    bytes += batch->pkts()[i]->data_len() + kEthernetOverhead;
  }
  stat->bytes += bytes;
}

ADD_GATE_HOOK(Track, "track", "count the packets and batches")
