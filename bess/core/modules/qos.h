/*
 * SPDX-License-Identifier: BSD-3-Clause
 * Copyright 2014-2016, The Regents of the University of California.
 * Copyright 2016-2017, Nefeli Networks, Inc.
 * Copyright 2021-present Intel Corporation
 */
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
  int64_t deduct_len;
  struct rte_meter_trtcm_profile p;
  struct rte_meter_trtcm m;
  MeteringKey Data;
};

struct MKey {
  uint8_t key1;
  uint8_t key2;
};

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
  void DeInit() override;
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
