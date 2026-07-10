// Copyright (c) 2014-2016, The Regents of the University of California.
// Copyright (c) 2016-2017, Nefeli Networks, Inc.
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause
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

#include "wildcard_match.h"

#include <string>
#include <vector>

#include "../utils/endian.h"
#include "../utils/format.h"

using bess::metadata::Attribute;
enum { FieldType = 0, ValueType };

// dst = src & mask. len must be a multiple of sizeof(uint64_t)
static inline void mask(wm_hkey_t &dst, const wm_hkey_t &src,
                        const wm_hkey_t &mask, size_t len) {
  promise(len >= sizeof(uint64_t));
  promise(len <= sizeof(wm_hkey_t));

  for (size_t i = 0; i < len / 8; i++) {
    dst.u64_arr[i] = src.u64_arr[i] & mask.u64_arr[i];
  }
}
static inline void mask_bulk(const wm_hkey_t *src, void *dst, void **dsptr,
                             const wm_hkey_t &mask, int keys, size_t len) {
  promise(len >= sizeof(uint64_t));
  promise(len <= sizeof(wm_hkey_t));
  size_t i = 0;
  wm_hkey_t *dst1 = (wm_hkey_t *)dst;
  wm_hkey_t **dstptr = (wm_hkey_t **)dsptr;

  for (int j = 0; j < keys; j++) {
    for (i = 0; i < len / 8; i++) {
      dst1[j].u64_arr[i] = src[j].u64_arr[i] & mask.u64_arr[i];
    }
    dstptr[j] = &dst1[j];
  }
}

// XXX: this is repeated in many modules. get rid of them when converting .h to
// .hh, etc... it's in defined in some old header
static inline int is_valid_gate(gate_idx_t gate) {
  return (gate < MAX_GATES || gate == DROP_GATE);
}

const Commands WildcardMatch::cmds = {
    {"get_initial_arg", "EmptyArg",
     MODULE_CMD_FUNC(&WildcardMatch::GetInitialArg), Command::THREAD_SAFE},
    {"get_runtime_config", "EmptyArg",
     MODULE_CMD_FUNC(&WildcardMatch::GetRuntimeConfig), Command::THREAD_SAFE},
    {"set_runtime_config", "WildcardMatchConfig",
     MODULE_CMD_FUNC(&WildcardMatch::SetRuntimeConfig), Command::THREAD_UNSAFE},
    {"add", "WildcardMatchCommandAddArg",
     MODULE_CMD_FUNC(&WildcardMatch::CommandAdd), Command::THREAD_SAFE},
    {"delete", "WildcardMatchCommandDeleteArg",
     MODULE_CMD_FUNC(&WildcardMatch::CommandDelete), Command::THREAD_SAFE},
    {"clear", "EmptyArg", MODULE_CMD_FUNC(&WildcardMatch::CommandClear),
     Command::THREAD_SAFE},
    {"set_default_gate", "WildcardMatchCommandSetDefaultGateArg",
     MODULE_CMD_FUNC(&WildcardMatch::CommandSetDefaultGate),
     Command::THREAD_SAFE}};

CommandResponse WildcardMatch::AddFieldOne(const bess::pb::Field &field,
                                           struct WmField *f, uint8_t type) {
  f->size = field.num_bytes();

  if (f->size < 1 || f->size > MAX_FIELD_SIZE) {
    return CommandFailure(EINVAL, "'size' must be 1-%d", MAX_FIELD_SIZE);
  }

  if (field.position_case() == bess::pb::Field::kOffset) {
    f->attr_id = -1;
    f->offset = field.offset();
    if (f->offset < 0 || f->offset > 1024) {
      return CommandFailure(EINVAL, "too small 'offset'");
    }
  } else if (field.position_case() == bess::pb::Field::kAttrName) {
    const char *attr = field.attr_name().c_str();
    f->attr_id =
        (type == FieldType)
            ? AddMetadataAttr(attr, f->size, Attribute::AccessMode::kRead)
            : AddMetadataAttr(attr, f->size, Attribute::AccessMode::kWrite);
    if (f->attr_id < 0) {
      return CommandFailure(-f->attr_id, "add_metadata_attr() failed");
    }
  } else {
    return CommandFailure(EINVAL, "specify 'offset' or 'attr'");
  }

  return CommandSuccess();
}

/* Takes a list of all fields that may be used by rules.
 * Each field needs 'offset' (or 'name') and 'size' in bytes,
 *
 * e.g.: WildcardMatch([{'offset': 26, 'size': 4}, ...]
 * (checks the source IP address)
 *
 * You can also specify metadata attributes
 * e.g.: WildcardMatch([{'name': 'nexthop', 'size': 4}, ...] */

CommandResponse WildcardMatch::Init(const bess::pb::WildcardMatchArg &arg) {
  int size_acc = 0;

  for (int i = 0; i < arg.fields_size(); i++) {
    const auto &field = arg.fields(i);
    CommandResponse err;
    fields_.emplace_back();
    struct WmField &f = fields_.back();

    f.pos = size_acc;

    err = AddFieldOne(field, &f, FieldType);
    if (err.error().code() != 0) {
      return err;
    }

    size_acc += f.size;
  }
  default_gate_ = DROP_GATE;
  total_key_size_ = align_ceil(size_acc, sizeof(uint64_t));
  entries_ = arg.entries();
  // reset size_acc
  size_acc = 0;
  for (int i = 0; i < arg.values_size(); i++) {
    const auto &value = arg.values(i);
    CommandResponse err;
    values_.emplace_back();
    struct WmField &v = values_.back();

    v.pos = size_acc;

    err = AddFieldOne(value, &v, ValueType);
    if (err.error().code() != 0) {
      return err;
    }

    size_acc += v.size;
  }

  total_value_size_ = align_ceil(size_acc, sizeof(uint64_t));

  return CommandSuccess();
}

inline gate_idx_t WildcardMatch::LookupEntry(const wm_hkey_t &key,
                                             gate_idx_t def_gate,
                                             bess::Packet *pkt) {
  struct WmData result = {
      .priority = INT_MIN, .ogate = def_gate, .keyv = {{0}}};
  for (auto &tuple : tuples_) {
    if (tuple.occupied == 0)
      continue;
    const auto &ht = tuple.ht;
    wm_hkey_t key_masked;
    mask(key_masked, key, tuple.mask, total_key_size_);
    WmData *entry = nullptr;
    ht->find_dpdk(&key_masked, ((void **)&entry));
    if (entry && entry->priority >= result.priority) {
      result = *entry;
    }
  }

  /* if lookup was successful, then set values (if possible) */
  if (result.ogate != default_gate_) {
    size_t num_values_ = values_.size();
    for (size_t i = 0; i < num_values_; i++) {
      int value_size = values_[i].size;
      int value_pos = values_[i].pos;
      int value_off = values_[i].offset;
      int value_attr_id = values_[i].attr_id;
      uint8_t *data = pkt->head_data<uint8_t *>() + value_off;

      DLOG(INFO) << "off: " << value_off << ", sz: " << value_size;

      if (value_attr_id < 0) { /* if it is offset-based */
        memcpy(data, reinterpret_cast<uint8_t *>(&result.keyv) + value_pos,
               value_size);
      } else { /* if it is attribute-based */
        typedef struct {
          uint8_t bytes[bess::metadata::kMetadataAttrMaxSize];
        } value_t;
        uint8_t *buf = (uint8_t *)&result.keyv + value_pos;

        DLOG(INFO) << "Setting value " << std::hex
                   << *(reinterpret_cast<uint64_t *>(buf))
                   << " for attr_id: " << value_attr_id
                   << " of size: " << value_size
                   << " at value_pos: " << value_pos;

        switch (value_size) {
          case 1:
            set_attr<uint8_t>(this, value_attr_id, pkt, *((uint8_t *)buf));
            break;
          case 2:
            set_attr<uint16_t>(this, value_attr_id, pkt,
                               *((uint16_t *)((uint8_t *)buf)));
            break;
          case 4:
            set_attr<uint32_t>(this, value_attr_id, pkt,
                               *((uint32_t *)((uint8_t *)buf)));
            break;
          case 8:
            set_attr<uint64_t>(this, value_attr_id, pkt,
                               *((uint64_t *)((uint8_t *)buf)));
            break;
          default: {
            void *mt_ptr =
                _ptr_attr_with_offset<value_t>(attr_offset(value_attr_id), pkt);
            bess::utils::CopySmall(mt_ptr, buf, value_size);
          } break;
        }
      }
    }
  }
  return result.ogate;
}

inline bool WildcardMatch::LookupBulkEntry(wm_hkey_t *key, gate_idx_t def_gate,
                                           int packeti, gate_idx_t *Outgate,
                                           int cnt, bess::PacketBatch *batch) {
  bess::Packet *pkt = nullptr;
  struct WmData *result[cnt];
  uint64_t prev_hitmask = 0;
  uint64_t hitmask = 0;
  wm_hkey_t key_masked[cnt];
  WmData *entry[cnt];
  wm_hkey_t **key_ptr[cnt];

  for (auto tuple = tuples_.begin(); tuple != tuples_.end(); ++tuple) {
    if (tuple->occupied == 0)
      continue;
    const auto &ht = tuple->ht;
    mask_bulk(key, key_masked, (void **)key_ptr, tuple->mask, cnt,
              total_key_size_);
    int num = ht->lookup_bulk_data((const void **)key_ptr, cnt, &hitmask,
                                   (void **)entry);
    if (num == 0)
      continue;

    for (int init = 0; (init < cnt) && (num); init++) {
      if ((hitmask & ((uint64_t)1 << init))) {
        if ((prev_hitmask & ((uint64_t)1 << init)) == 0)
          result[init] = entry[init];
        else if ((prev_hitmask & ((uint64_t)1 << init)) &&
                 (entry[init]->priority >= result[init]->priority)) {
          result[init] = entry[init];
        }

        num--;
      }
    }
    prev_hitmask = prev_hitmask | hitmask;
  }

  for (int init = 0; init < cnt; init++) {
    /* if lookup was successful, then set values (if possible) */
    if (prev_hitmask && (prev_hitmask & ((uint64_t)1 << init))) {
      pkt = batch->pkts()[packeti + init];
      size_t num_values_ = values_.size();
      for (size_t i = 0; i < num_values_; i++) {
        int value_size = values_[i].size;
        int value_pos = values_[i].pos;
        int value_off = values_[i].offset;
        int value_attr_id = values_[i].attr_id;
        uint8_t *data = pkt->head_data<uint8_t *>() + value_off;

        DLOG(INFO) << "off: " << value_off << ", sz: " << value_size;
        if (value_attr_id < 0) { /* if it is offset-based */
          memcpy(data,
                 reinterpret_cast<uint8_t *>(&result[init]->keyv) + value_pos,
                 value_size);
        } else { /* if it is attribute-based */
          typedef struct {
            uint8_t bytes[bess::metadata::kMetadataAttrMaxSize];
          } value_t;
          uint8_t *buf = (uint8_t *)&result[init]->keyv + value_pos;

          DLOG(INFO) << "Setting value " << std::hex
                     << *(reinterpret_cast<uint64_t *>(buf))
                     << " for attr_id: " << value_attr_id
                     << " of size: " << value_size
                     << " at value_pos: " << value_pos;

          switch (value_size) {
            case 1:
              set_attr<uint8_t>(this, value_attr_id, pkt, *((uint8_t *)buf));
              break;
            case 2:
              set_attr<uint16_t>(this, value_attr_id, pkt,
                                 *((uint16_t *)((uint8_t *)buf)));
              break;
            case 4:
              set_attr<uint32_t>(this, value_attr_id, pkt,
                                 *((uint32_t *)((uint8_t *)buf)));
              break;
            case 8:
              set_attr<uint64_t>(this, value_attr_id, pkt,
                                 *((uint64_t *)((uint8_t *)buf)));
              break;
            default: {
              void *mt_ptr = _ptr_attr_with_offset<value_t>(
                  attr_offset(value_attr_id), pkt);
              bess::utils::CopySmall(mt_ptr, buf, value_size);
            } break;
          }
        }
      }
      Outgate[init] = result[init]->ogate;
    } else
      Outgate[init] = def_gate;
  }
  return 1;
}

void WildcardMatch::ProcessBatch(Context *ctx, bess::PacketBatch *batch) {
  gate_idx_t default_gate;
  wm_hkey_t keys[bess::PacketBatch::kMaxBurst] __ymm_aligned;
  int cnt = batch->cnt();
  gate_idx_t Outgate[cnt];

  // Initialize the padding with zero
  for (int i = 0; i < cnt; i++) {
    keys[i].u64_arr[(total_key_size_ - 1) / 8] = 0;
  }
  default_gate = ACCESS_ONCE(default_gate_);
  for (const auto &field : fields_) {
    int offset;
    int pos = field.pos;
    int attr_id = field.attr_id;

    if (attr_id < 0) {
      offset = field.offset;
    } else {
      offset = bess::Packet::mt_offset_to_databuf_offset(attr_offset(attr_id));
    }

    for (int j = 0; j < cnt; j++) {
      char *buf_addr = batch->pkts()[j]->buffer<char *>();

      /* for offset-based attrs we use relative offset */
      if (attr_id < 0) {
        buf_addr += batch->pkts()[j]->data_off();
      }

      char *key = reinterpret_cast<char *>(keys[j].u64_arr) + pos;

      *(reinterpret_cast<uint64_t *>(key)) =
          *(reinterpret_cast<uint64_t *>(buf_addr + offset));
    }
  }

  if (cnt > 64) {
    int icnt = 0;
    for (int lcnt = 0; lcnt < cnt; lcnt = lcnt + icnt) {
      icnt = ((cnt - lcnt) >= 64) ? 64 : cnt - lcnt;
      LookupBulkEntry(&keys[lcnt], default_gate, lcnt, Outgate, icnt, batch);
      for (int j = 0; j < icnt; j++) {
        EmitPacket(ctx, batch->pkts()[j + lcnt], Outgate[j]);
      }
    }
  } else {
    LookupBulkEntry(keys, default_gate, 0, Outgate, cnt, batch);
    for (int j = 0; j < cnt; j++) {
      EmitPacket(ctx, batch->pkts()[j], Outgate[j]);
    }
  }
}

std::string WildcardMatch::GetDesc() const {
  int num_rules = 0;

  for (const auto &tuple : tuples_) {
    if (tuple.occupied == 0)
      continue;
    num_rules += tuple.ht->Count();
  }

  return bess::utils::Format("%zu fields, %d rules", fields_.size(), num_rules);
}

template <typename T>
CommandResponse WildcardMatch::ExtractKeyMask(const T &arg, wm_hkey_t *key,
                                              wm_hkey_t *mask) {
  if ((size_t)arg.values_size() != fields_.size()) {
    return CommandFailure(EINVAL, "must specify %zu values", fields_.size());
  } else if ((size_t)arg.masks_size() != fields_.size()) {
    return CommandFailure(EINVAL, "must specify %zu masks", fields_.size());
  }

  memset(key, 0, sizeof(*key));
  memset(mask, 0, sizeof(*mask));

  for (size_t i = 0; i < fields_.size(); i++) {
    int field_size = fields_[i].size;
    int field_pos = fields_[i].pos;

    uint64_t v = 0;
    uint64_t m = 0;

    bess::pb::FieldData valuedata = arg.values(i);
    if (valuedata.encoding_case() == bess::pb::FieldData::kValueInt) {
      if (!bess::utils::uint64_to_bin(&v, valuedata.value_int(), field_size,
                                      true)) {
        return CommandFailure(EINVAL, "idx %zu: not a correct %d-byte value", i,
                              field_size);
      }
    } else if (valuedata.encoding_case() == bess::pb::FieldData::kValueBin) {
      bess::utils::Copy(reinterpret_cast<uint8_t *>(&v),
                        valuedata.value_bin().c_str(),
                        valuedata.value_bin().size());
    }

    bess::pb::FieldData maskdata = arg.masks(i);
    if (maskdata.encoding_case() == bess::pb::FieldData::kValueInt) {
      if (!bess::utils::uint64_to_bin(&m, maskdata.value_int(), field_size,
                                      true)) {
        return CommandFailure(EINVAL, "idx %zu: not a correct %d-byte mask", i,
                              field_size);
      }
    } else if (maskdata.encoding_case() == bess::pb::FieldData::kValueBin) {
      bess::utils::Copy(reinterpret_cast<uint8_t *>(&m),
                        maskdata.value_bin().c_str(),
                        maskdata.value_bin().size());
    }

    if (v & ~m) {
      return CommandFailure(EINVAL,
                            "idx %zu: invalid pair of "
                            "value 0x%0*" PRIx64
                            " and "
                            "mask 0x%0*" PRIx64,
                            i, field_size * 2, v, field_size * 2, m);
    }

    // Use memcpy, not utils::Copy, to workaround the false positive warning
    // in g++-8
    memcpy(reinterpret_cast<uint8_t *>(key) + field_pos, &v, field_size);
    memcpy(reinterpret_cast<uint8_t *>(mask) + field_pos, &m, field_size);
  }

  return CommandSuccess();
}

template <typename T>
CommandResponse WildcardMatch::ExtractValue(const T &arg, wm_hkey_t *keyv) {
  if ((size_t)arg.valuesv_size() != values_.size()) {
    return CommandFailure(EINVAL, "must specify %zu values", values_.size());
  }

  memset(keyv, 0, sizeof(*keyv));

  for (size_t i = 0; i < values_.size(); i++) {
    int value_size = values_[i].size;
    int value_pos = values_[i].pos;

    uint64_t v = 0;

    bess::pb::FieldData valuedata = arg.valuesv(i);
    if (valuedata.encoding_case() == bess::pb::FieldData::kValueInt) {
      if (!bess::utils::uint64_to_bin(&v, valuedata.value_int(), value_size,
                                      false)) {
        return CommandFailure(EINVAL, "idx %zu: not a correct %d-byte value", i,
                              value_size);
      }
    } else if (valuedata.encoding_case() == bess::pb::FieldData::kValueBin) {
      bess::utils::Copy(reinterpret_cast<uint8_t *>(&v),
                        valuedata.value_bin().c_str(),
                        valuedata.value_bin().size());
    }

    // Use memcpy, not utils::Copy, to workaround the false positive warning
    // in g++-8
    memcpy(reinterpret_cast<uint8_t *>(keyv) + value_pos, &v, value_size);
  }

  return CommandSuccess();
}

int WildcardMatch::FindTuple(wm_hkey_t *mask) {
  for (auto i = 0; i < MAX_TUPLES; i++) {
    if ((tuples_[i].occupied) &&
        (memcmp(&tuples_[i].mask, mask, total_key_size_) == 0)) {
      return i;
    }
  }
  return -ENOENT;
}

int WildcardMatch::AddTuple(wm_hkey_t *mask) {
  CuckooMap<wm_hkey_t, struct WmData, wm_hash, wm_eq> *temp = nullptr;
  for (int i = 0; i < MAX_TUPLES; i++) {
    if (tuples_[i].occupied == 0) {
      bess::utils::Copy(&tuples_[i].mask, mask, sizeof(*mask));
      tuples_[i].params.key_len = total_key_size_;
      if (entries_) {
        tuples_[i].params.entries = entries_;
      }
      temp = new CuckooMap<wm_hkey_t, struct WmData, wm_hash, wm_eq>(
          0, 0, &tuples_[i].params);
      if (temp == nullptr)
        return -ENOSPC;
      if (temp->hash == 0) {
        delete temp;
        return -ENOSPC;
      }
      void *temp1 = tuples_[i].ht;
      tuples_[i].ht = temp;
      if (temp1)
        delete (
            static_cast<CuckooMap<wm_hkey_t, struct WmData, wm_hash, wm_eq> *>(
                temp1));
      tuples_[i].occupied = 1;
      return i;
    }
  }
  return -ENOSPC;
}

bool WildcardMatch::DelEntry(int idx, wm_hkey_t *key) {
  int ret = tuples_[idx].ht->Remove(*key, wm_hash(total_key_size_),
                                    wm_eq(total_key_size_));
  if (ret >= 0) {
    return true;
  }
  if (tuples_[idx].ht->Count() == 0) {
  }
  return false;
}

CommandResponse WildcardMatch::CommandAdd(
    const bess::pb::WildcardMatchCommandAddArg &arg) {
  gate_idx_t gate = arg.gate();
  int priority = arg.priority();
  wm_hkey_t key = {{0}};
  wm_hkey_t mask = {{0}};
  struct WmData data;
  CommandResponse err = ExtractKeyMask(arg, &key, &mask);
  if (err.error().code() != 0) {
    return err;
  }

  if (!is_valid_gate(gate)) {
    return CommandFailure(EINVAL, "Invalid gate: %hu", gate);
  }

  err = ExtractValue(arg, &(data.keyv));
  if (err.error().code() != 0) {
    return err;
  }

  data.priority = priority;
  data.ogate = gate;
  int idx = FindTuple(&mask);
  if (idx < 0) {
    idx = AddTuple(&mask);
    if (idx < 0) {
      return CommandFailure(-idx, "failed to add a new wildcard pattern");
    }
  }
  struct WmData *data_t = new WmData(data);
  int ret = tuples_[idx].ht->insert_dpdk(&key, data_t);
  if (ret < 0)
    return CommandFailure(EINVAL, "failed to add a rule");
  return CommandSuccess();
}

CommandResponse WildcardMatch::CommandDelete(
    const bess::pb::WildcardMatchCommandDeleteArg &arg) {
  wm_hkey_t key;
  wm_hkey_t mask;

  CommandResponse err = ExtractKeyMask(arg, &key, &mask);
  if (err.error().code() != 0) {
    return err;
  }

  int idx = FindTuple(&mask);
  if (idx < 0) {
    return CommandFailure(-idx, "failed to delete a rule");
  }

  int ret = DelEntry(idx, &key);
  if (ret < 0) {
    return CommandFailure(-ret, "failed to delete a rule");
  }

  return CommandSuccess();
}

CommandResponse WildcardMatch::CommandClear(const bess::pb::EmptyArg &) {
  WildcardMatch::Clear();
  return CommandSuccess();
}

void WildcardMatch::Clear() {
  for (auto &tuple : tuples_) {
    if (tuple.occupied) {
      tuple.ht->DeInit();
      delete tuple.ht;
      tuple.ht = nullptr;
      tuple.occupied = 0;
    }
  }
}

// Retrieves a WildcardMatchArg that would reconstruct this module.
CommandResponse WildcardMatch::GetInitialArg(const bess::pb::EmptyArg &) {
  bess::pb::WildcardMatchArg resp;
  for (auto &field : fields_) {
    bess::pb::Field *f = resp.add_fields();
    if (field.attr_id >= 0) {
      f->set_attr_name(all_attrs().at(field.attr_id).name);
    } else {
      f->set_offset(field.offset);
    }
    f->set_num_bytes(field.size);
  }
  return CommandSuccess(resp);
}

// Retrieves a WildcardMatchConfig that would restore this module's
// runtime configuration.
CommandResponse WildcardMatch::GetRuntimeConfig(const bess::pb::EmptyArg &) {
  std::pair<wm_hkey_t, WmData> entry;
  bess::pb::WildcardMatchConfig resp;
  using rule_t = bess::pb::WildcardMatchCommandAddArg;
  const wm_hkey_t *key = 0;
  WmData *data;
  uint32_t *next = 0;
  resp.set_default_gate(default_gate_);

  // Each tuple provides a single mask, which may have many data-matches.
  for (auto &tuple : tuples_) {
    if (tuple.occupied == 0)
      continue;
    wm_hkey_t mask = tuple.mask;
    // Each entry in the hash table has priority, ogate, and the data
    // (one datum per field, under the mask for this field).
    //  using rte method
    while ((tuple.ht->Iterate((const void **)&key, (void **)&data, next)) >=
           (int)0) {
      entry.first = *key;
      entry.second = *data;
      // Create the rule instance
      rule_t *rule = resp.add_rules();
      rule->set_priority(entry.second.priority);
      rule->set_gate(entry.second.ogate);

      uint8_t *entry_data = reinterpret_cast<uint8_t *>(entry.first.u64_arr);
      uint8_t *entry_mask = reinterpret_cast<uint8_t *>(mask.u64_arr);
      // Then fill in each field
      for (auto &field : fields_) {
        bess::pb::FieldData *valuedata = rule->add_values();
        valuedata->set_value_bin(entry_data + field.pos, field.size);
        bess::pb::FieldData *maskdata = rule->add_masks();
        maskdata->set_value_bin(entry_mask + field.pos, field.size);
      }
    }
  }
  // Sort the results so that they're always predictable.
  std::sort(resp.mutable_rules()->begin(), resp.mutable_rules()->end(),
            [this](const rule_t &a, const rule_t &b) {
              // Sort is by priority, then gate, then masks, then values.
              // The precise order is not as important as consistency.
              if (a.priority() != b.priority()) {
                return a.priority() < b.priority();
              }
              if (a.gate() != b.gate()) {
                return a.gate() < b.gate();
              }
              for (size_t i = 0; i < fields_.size(); i++) {
                if (a.masks(i).value_bin() != b.masks(i).value_bin()) {
                  return a.masks(i).value_bin() < b.masks(i).value_bin();
                }
              }
              for (size_t i = 0; i < fields_.size(); i++) {
                if (a.values(i).value_bin() != b.values(i).value_bin()) {
                  return a.values(i).value_bin() < b.values(i).value_bin();
                }
              }
              return false;
            });
  return CommandSuccess(resp);
}

CommandResponse WildcardMatch::CommandSetDefaultGate(
    const bess::pb::WildcardMatchCommandSetDefaultGateArg &arg) {
  default_gate_ = arg.gate();
  return CommandSuccess();
}

// Uses a WildcardMatchConfig to restore this module's runtime config.
// If this returns with an error, the state may be partially restored.
// TODO(torek): consider vetting the entire argument before clobbering state.
CommandResponse WildcardMatch::SetRuntimeConfig(
    const bess::pb::WildcardMatchConfig &arg) {
  WildcardMatch::Clear();
  default_gate_ = arg.default_gate();
  for (int i = 0; i < arg.rules_size(); i++) {
    CommandResponse err = WildcardMatch::CommandAdd(arg.rules(i));
    if (err.error().code() != 0) {
      return err;
    }
  }
  return CommandSuccess();
}

void WildcardMatch::DeInit() {
  for (auto &tuple : tuples_) {
    if (!tuple.ht)
      continue;
    tuple.ht->DeInit();
    tuple.ht = NULL;
  }
}

ADD_MODULE(WildcardMatch, "wm",
           "Multi-field classifier with a wildcard match table")
