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

#ifndef BESS_MODULES_EXACTMATCH_H_
#define BESS_MODULES_EXACTMATCH_H_

#include <rte_config.h>
#include <rte_hash_crc.h>

#include "../module.h"
#include "../pb/module_msg.pb.h"
#include "../utils/exact_match_table.h"
#include "../utils/format.h"

using bess::utils::Error;
using bess::utils::ExactMatchField;
using bess::utils::ExactMatchKey;
using bess::utils::ExactMatchRuleFields;
using bess::utils::ExactMatchTable;
using google::protobuf::RepeatedPtrField;

typedef enum { FIELD_TYPE = 0, VALUE_TYPE } Type;

class ExactMatch;
class Value {
  friend class ExactMatch;

 public:
  Value(gate_idx_t g = 0) : gate(g) {}
  Value(const Value &v) : gate(v.gate) {}
  gate_idx_t gate;
};

class ValueTuple : public Value {
  friend class ExactMatch;

 public:
  ValueTuple() : Value(), action() {}
  ValueTuple(Value v) : Value(v), action() {}

  ExactMatchKey action;
};

class ExactMatch final : public Module {
 public:
  static const gate_idx_t kNumOGates = MAX_GATES;

  static const Commands cmds;

  ExactMatch()
      : Module(),
        default_gate_(),
        raw_value_size_(),
        total_value_size_(),
        num_values_(),
        values_(),
        table_() {
    max_allowed_workers_ = Worker::kMaxWorkers;
  }

  void ProcessBatch(Context *ctx, bess::PacketBatch *batch) override;

  std::string GetDesc() const override;

  CommandResponse Init(const bess::pb::ExactMatchArg &arg);

  void DeInit() override;

  CommandResponse GetInitialArg(const bess::pb::EmptyArg &arg);
  CommandResponse GetRuntimeConfig(const bess::pb::EmptyArg &arg);
  CommandResponse SetRuntimeConfig(const bess::pb::ExactMatchConfig &arg);
  CommandResponse CommandAdd(const bess::pb::ExactMatchCommandAddArg &arg);
  CommandResponse CommandDelete(
      const bess::pb::ExactMatchCommandDeleteArg &arg);
  CommandResponse CommandClear(const bess::pb::EmptyArg &arg);
  CommandResponse CommandSetDefaultGate(
      const bess::pb::ExactMatchCommandSetDefaultGateArg &arg);

 private:
  CommandResponse AddFieldOne(const bess::pb::Field &field,
                              const bess::pb::FieldData &mask, int idx, Type t);
  void RuleFieldsFromPb(const RepeatedPtrField<bess::pb::FieldData> &fields,
                        bess::utils::ExactMatchRuleFields *rule, Type type);
  Error AddRule(const bess::pb::ExactMatchCommandAddArg &arg);
  size_t num_values() const { return num_values_; }
  ExactMatchField *getVals() { return values_; };
  Error gather_value(const ExactMatchRuleFields &fields, ExactMatchKey *key) {
    if (fields.size() != num_values_) {
      return std::make_pair(
          EINVAL, bess::utils::Format("rule should have %zu fields (has %zu)",
                                      num_values_, fields.size()));
    }

    *key = {};

    for (size_t i = 0; i < fields.size(); i++) {
      int field_size = values_[i].size;
      int field_pos = values_[i].pos;

      const std::vector<uint8_t> &f_obj = fields[i];

      if (static_cast<size_t>(field_size) != f_obj.size()) {
        return std::make_pair(
            EINVAL,
            bess::utils::Format("rule field %zu should have size %d (has %zu)",
                                i, field_size, f_obj.size()));
      }

      memcpy(reinterpret_cast<uint8_t *>(key) + field_pos, f_obj.data(),
             field_size);
    }

    return std::make_pair(0, bess::utils::Format("Success"));
  }
  // Helper for public AddField functions.
  // DoAddValue inserts `field` as the `idx`th field for this table.
  // If `mt_attr_name` is set, the `offset` field of `field` will be ignored and
  // the inserted field will use the offset of `mt_attr_name` as reported by the
  // module `m`.
  // Returns 0 on success, non-zero errno on failure.
  Error DoAddValue(const ExactMatchField &value,
                   const std::string &mt_attr_name, int idx,
                   Module *m = nullptr) {
    if (idx >= MAX_FIELDS) {
      return std::make_pair(
          EINVAL,
          bess::utils::Format("idx %d is not in [0,%d)", idx, MAX_FIELDS));
    }
    ExactMatchField *v = &values_[idx];
    v->size = value.size;
    if (v->size < 1 || v->size > MAX_FIELD_SIZE) {
      return std::make_pair(
          EINVAL, bess::utils::Format("idx %d: 'size' must be in [1,%d]", idx,
                                      MAX_FIELD_SIZE));
    }

    if (mt_attr_name.length() > 0) {
      v->attr_id = m->AddMetadataAttr(
          mt_attr_name, v->size, bess::metadata::Attribute::AccessMode::kWrite);
      if (v->attr_id < 0) {
        return std::make_pair(
            -v->attr_id,
            bess::utils::Format("idx %d: add_metadata_attr() failed", idx));
      }
    } else {
      v->attr_id = -1;
      v->offset = value.offset;
      if (v->offset < 0 || v->offset > 1024) {
        return std::make_pair(
            EINVAL, bess::utils::Format("idx %d: invalid 'offset'", idx));
      }
    }

    int force_be = (v->attr_id < 0);

    if (value.mask == 0) {
      /* by default all bits are considered */
      v->mask = bess::utils::SetBitsHigh<uint64_t>(v->size * 8);
    } else {
      if (!bess::utils::uint64_to_bin(&v->mask, value.mask, v->size,
                                      bess::utils::is_be_system() | force_be)) {
        return std::make_pair(
            EINVAL, bess::utils::Format("idx %d: not a valid %d-byte mask", idx,
                                        v->size));
      }
    }

    if (v->mask == 0) {
      return std::make_pair(EINVAL,
                            bess::utils::Format("idx %d: empty mask", idx));
    }

    num_values_++;

    v->pos = raw_value_size_;
    raw_value_size_ += v->size;
    total_value_size_ = align_ceil(raw_value_size_, sizeof(uint64_t));
    return std::make_pair(0, bess::utils::Format("Success"));
  }
  // Returns the ith value.
  const ExactMatchField &get_value(size_t i) const { return values_[i]; }
  // Set the `idx`th field of this table to one at offset `offset` bytes into a
  // buffer with length `size` and mask `mask`.
  // Returns 0 on success, non-zero errno on failure.
  Error AddValue(int offset, int size, uint64_t mask, int idx) {
    ExactMatchField v = {
        .mask = mask, .attr_id = 0, .offset = offset, .pos = 0, .size = size};
    return DoAddValue(v, "", idx, nullptr);
  }

  // Set the `idx`th field of this table to one at the offset of the
  // `mt_attr_name` metadata field as seen by module `m`, with length `size` and
  // mask `mask`.
  // Returns 0 on success, non-zero errno on failure.
  Error AddValue(Module *m, const std::string &mt_attr_name, int size,
                 uint64_t mask, int idx) {
    ExactMatchField v = {
        .mask = mask, .attr_id = 0, .offset = 0, .pos = 0, .size = size};
    return DoAddValue(v, mt_attr_name, idx, m);
  }
  Error CreateValue(ExactMatchKey &v, const ExactMatchRuleFields &values) {
    Error err;

    if (values.size() == 0) {
      return std::make_pair(EINVAL, "rule has no values");
    }

    if ((err = gather_value(values, &v)).first != 0) {
      return err;
    }

    return std::make_pair(0, bess::utils::Format("Success"));
  }
  void setValues(bess::Packet *pkt, ExactMatchKey &action);

  gate_idx_t default_gate_;
  bool empty_masks_;  // mainly for GetInitialArg

  // unaligend key size, used as an accumulator for calls to AddField()
  size_t raw_value_size_;

  // aligned total key size
  size_t total_value_size_;

  size_t num_values_;
  ExactMatchField values_[MAX_FIELDS];
  ExactMatchTable<ValueTuple> table_;
};

#endif  // BESS_MODULES_EXACTMATCH_H_
