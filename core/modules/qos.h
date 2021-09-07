// Copyright (c) 2014-2016, The Regents of the University of California.
// Copyright (c) 2016-2017, Nefeli Networks, Inc.
// Copyright (c) 2021 Intel Corporation
// All rights reserved
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

#ifndef BESS_MODULES_QOS_H_
#define BESS_MODULES_QOS_H_

#include "../module.h"

#include <rte_config.h>
#include <rte_hash_crc.h>

#include "../pb/module_msg.pb.h"
#include "../utils/metering.h"

using bess::utils::Metering;
using bess::utils::MeteringKey;

#define MAX_FIELDS 8
#define MAX_FIELD_SIZE 8
static_assert(MAX_FIELD_SIZE <= sizeof(uint64_t),
              "field cannot be larger than 8 bytes");

#define HASH_KEY_SIZE (MAX_FIELDS * MAX_FIELD_SIZE)
#define METER_GATE 0
#define METER_GREEN_GATE 1
#define METER_YELLOW_GATE 2
#define METER_RED_GATE 3

#if __BYTE_ORDER__ != __ORDER_LITTLE_ENDIAN__
#error this code assumes little endian architecture (x86)
#endif

enum { FieldType = 0, ValueType };

struct value {
  gate_idx_t ogate;
  uint64_t cir;
  uint64_t pir;
  uint64_t cbs;
  uint64_t pbs;
  uint64_t ebs;
  int64_t adjust_meter_packet_length;
  struct rte_meter_trtcm_profile p;
  struct rte_meter_trtcm m;
  MeteringKey Data;
} __attribute__((packed));

struct MKey {
  uint8_t key1;
  uint8_t key2;
} __attribute__((packed));

class Qos final : public Module {
 public:
  static const gate_idx_t kNumOGates = MAX_GATES;

  static const Commands cmds;

  Qos() : Module(), default_gate_(), total_key_size_(), fields_() {
    max_allowed_workers_ = Worker::kMaxWorkers;
    size_t len = sizeof(mask) / sizeof(uint64_t);
    for (size_t i = 0; i < len; i++)
      mask[i] = 0;
  }

  CommandResponse Init(const bess::pb::QosArg &arg);
  void ProcessBatch(Context *ctx, bess::PacketBatch *batch) override;
  CommandResponse CommandAdd(const bess::pb::QosCommandAddArg &arg);
  CommandResponse CommandDelete(const bess::pb::QosCommandDeleteArg &arg);
  CommandResponse CommandClear(const bess::pb::EmptyArg &arg);
  CommandResponse CommandSetDefaultGate(
      const bess::pb::QosCommandSetDefaultGateArg &arg);
  template <typename T>
  CommandResponse ExtractKeyMask(const T &arg, MeteringKey *key,
                                 MeteringKey *val, MKey *l);
  template <typename T>
  CommandResponse ExtractKey(const T &arg, MeteringKey *key);
  CommandResponse AddFieldOne(const bess::pb::Field &field,
                              struct MeteringField *f, uint8_t type);
  gate_idx_t LookupEntry(const MeteringKey &key, gate_idx_t def_gate);
  void DeInit();
  std::string GetDesc() const override;

 private:
  int DelEntry(MeteringKey *key);
  void Clear();
  gate_idx_t default_gate_;
  size_t total_key_size_; /* a multiple of sizeof(uint64_t) */
  size_t total_value_size_;
  std::vector<struct MeteringField> fields_;
  std::vector<struct MeteringField> values_;
  Metering<value> table_;
  uint64_t mask[MAX_FIELDS];
};

#endif  // BESS_MODULES_QOS_H
