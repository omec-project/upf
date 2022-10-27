/*
 * SPDX-License-Identifier: BSD-3-Clause
 * Copyright 2014-2016, The Regents of the University of California.
 * Copyright 2016-2017, Nefeli Networks, Inc.
 * Copyright 2021-present Intel Corporation
 */
#include "qos.h"
#include "utils/endian.h"
#include "utils/format.h"

#include <rte_cycles.h>
#include <string>
#include <vector>

typedef enum { FIELD_TYPE = 0, VALUE_TYPE } Type;
using bess::metadata::Attribute;
#define metering_test 0
static inline int is_valid_gate(gate_idx_t gate) {
  return (gate == METER_GATE || (gate > METER_RED_GATE && gate < MAX_GATES) ||
          gate == DROP_GATE);
}

const Commands Qos::cmds = {
    {"add", "QosCommandAddArg", MODULE_CMD_FUNC(&Qos::CommandAdd),
     Command::THREAD_SAFE},
    {"delete", "QosCommandDeleteArg", MODULE_CMD_FUNC(&Qos::CommandDelete),
     Command::THREAD_SAFE},
    {"clear", "EmptyArg", MODULE_CMD_FUNC(&Qos::CommandClear),
     Command::THREAD_SAFE},
    {"set_default_gate", "QosCommandSetDefaultGateArg",
     MODULE_CMD_FUNC(&Qos::CommandSetDefaultGate), Command::THREAD_SAFE}};

CommandResponse Qos::AddFieldOne(const bess::pb::Field &field,
                                 struct MeteringField *f, uint8_t type) {
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

CommandResponse Qos::Init(const bess::pb::QosArg &arg) {
  int size_acc = 0;
  int value_acc = 0;

  for (int i = 0; i < arg.fields_size(); i++) {
    const auto &field = arg.fields(i);
    CommandResponse err;
    fields_.emplace_back();
    struct MeteringField &f = fields_.back();
    f.pos = size_acc;
    err = AddFieldOne(field, &f, FieldType);
    if (err.error().code() != 0) {
      return err;
    }

    size_acc += f.size;
  }
  default_gate_ = DROP_GATE;
  total_key_size_ = align_ceil(size_acc, sizeof(uint64_t));
  for (int i = 0; i < arg.values_size(); i++) {
    const auto &field = arg.values(i);
    CommandResponse err;
    values_.emplace_back();
    struct MeteringField &f = values_.back();
    f.pos = value_acc;
    err = AddFieldOne(field, &f, ValueType);
    if (err.error().code() != 0) {
      return err;
    }

    value_acc += f.size;
  }

  total_value_size_ = align_ceil(value_acc, sizeof(uint64_t));

  uint8_t *cs = (uint8_t *)&mask;
  for (int i = 0; i < size_acc; i++) {
    cs[i] = 0xff;
  }

  table_.Init(total_key_size_, arg.entries());
  return CommandSuccess();
}

void Qos::ProcessBatch(Context *ctx, bess::PacketBatch *batch) {
  gate_idx_t default_gate;
  MeteringKey keys[bess::PacketBatch::kMaxBurst] __ymm_aligned;
  bess::Packet *pkt = nullptr;
  default_gate = ACCESS_ONCE(default_gate_);
  int cnt = batch->cnt();  
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
      pkt = batch->pkts()[j];
      char *buf_addr = pkt->buffer<char *>();

      /* for offset-based attrs we use relative offset */
      if (attr_id < 0) {
        buf_addr += pkt->data_off();
      }

      char *key = reinterpret_cast<char *>(keys[j].u64_arr) + pos;

      *(reinterpret_cast<uint64_t *>(key)) =
          *(reinterpret_cast<uint64_t *>(buf_addr + offset));

      size_t len = reinterpret_cast<size_t>(total_key_size_ / sizeof(uint64_t));

      for (size_t i = 0; i < len; i++) {
        keys[j].u64_arr[i] = keys[j].u64_arr[i] & mask[i];
      }
    }
  }

  int icnt=0;
  for(int lcnt=0; lcnt<cnt ;lcnt=lcnt+icnt )
   {
    icnt = ((cnt-lcnt)>=64) ? 64 : cnt-lcnt  ;
    value *val[icnt];
    uint64_t hit_mask = table_.Find(keys+lcnt, val, icnt);

    for (int j = 0; j < icnt; j++) {
      pkt = batch->pkts()[j+lcnt];
      if ((hit_mask & ((uint64_t)1ULL << j)) == 0) {
        EmitPacket(ctx, pkt, default_gate);
        continue;
      }

    uint16_t ogate = val[j]->ogate;
    DLOG(INFO) << "ogate : " << ogate << std::endl;

    // meter if ogate is 0
    if (ogate == METER_GATE) {
      uint64_t time = rte_rdtsc();
      uint32_t pkt_len = pkt->total_len() - val[j]->deduct_len;
      uint8_t color = rte_meter_trtcm_color_blind_check(&val[j]->m, &val[j]->p,
                                                        time, pkt_len);

      DLOG(INFO) << "color : " << color << std::endl;
      // update ogate to color specific gate
      if (color == RTE_COLOR_GREEN) {
        ogate = METER_GREEN_GATE;
      } else if (color == RTE_COLOR_YELLOW) {
        ogate = METER_YELLOW_GATE;
      } else if (color == RTE_COLOR_RED) {
       ogate = METER_RED_GATE;
       }
      }

      // update values
     size_t num_values_ = values_.size();
     for (size_t i = 0; i < num_values_; i++) {
      int value_size = values_[i].size;
      int value_pos = values_[i].pos;
      int value_off = values_[i].offset;
      int value_attr_id = values_[i].attr_id;
      uint8_t *data = pkt->head_data<uint8_t *>() + value_off;

      if (value_attr_id < 0) { /* if it is offset-based */
        memcpy(data, reinterpret_cast<uint8_t *>(&(val[j]->Data)) + value_pos,
               value_size);
       } else { /* if it is attribute-based */
        typedef struct {
          uint8_t bytes[bess::metadata::kMetadataAttrMaxSize];
       } value_t;
       uint8_t *buf = (uint8_t *)(&(val[j]->Data)) + value_pos;

        DLOG(INFO) << "Setting value " << std::hex
                   << *(reinterpret_cast<uint64_t *>(buf))
                   << " for attr_id: " << value_attr_id
                   << " of size: " << value_size
                   << " at value_pos: " << value_pos << std::endl;

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
    EmitPacket(ctx, pkt, ogate);
  }
}

}

template <typename T>
CommandResponse Qos::ExtractKey(const T &arg, MeteringKey *key) {
  if ((size_t)arg.fields_size() != fields_.size()) {
    return CommandFailure(EINVAL, "must specify %zu masks", fields_.size());
  }

  memset(key, 0, sizeof(*key));

  for (size_t i = 0; i < fields_.size(); i++) {
    int field_size = fields_[i].size;
    int field_pos = fields_[i].pos;

    // uint64_t v = 0;
    uint64_t k = 0;

    bess::pb::FieldData fieldsdata = arg.fields(i);
    if (fieldsdata.encoding_case() == bess::pb::FieldData::kValueInt) {
      if (!bess::utils::uint64_to_bin(&k, fieldsdata.value_int(), field_size,
                                      false)) {
        return CommandFailure(EINVAL, "idx %zu: not a correct %d-byte mask", i,
                              field_size);
      }
    } else if (fieldsdata.encoding_case() == bess::pb::FieldData::kValueBin) {
      bess::utils::Copy(reinterpret_cast<uint8_t *>(&k),
                        fieldsdata.value_bin().c_str(),
                        fieldsdata.value_bin().size());
    }

    memcpy(reinterpret_cast<uint8_t *>(key) + field_pos, &k, field_size);
  }
  return CommandSuccess();
}

template <typename T>
CommandResponse Qos::ExtractKeyMask(const T &arg, MeteringKey *key,
                                    MeteringKey *val, MKey *l) {
  if ((size_t)arg.fields_size() != fields_.size()) {
    return CommandFailure(EINVAL, "must specify %zu masks", fields_.size());
  }

  memset(key, 0, sizeof(*key));
  memset(val, 0, sizeof(*val));

  for (size_t i = 0; i < fields_.size(); i++) {
    int field_size = fields_[i].size;
    int field_pos = fields_[i].pos;

    uint64_t k = 0;

    bess::pb::FieldData fieldsdata = arg.fields(i);
    if (fieldsdata.encoding_case() == bess::pb::FieldData::kValueInt) {
      if (!bess::utils::uint64_to_bin(&k, fieldsdata.value_int(), field_size,
                                      false)) {
        return CommandFailure(EINVAL, "idx %zu: not a correct %d-byte mask", i,
                              field_size);
      }
    } else if (fieldsdata.encoding_case() == bess::pb::FieldData::kValueBin) {
      bess::utils::Copy(reinterpret_cast<uint8_t *>(&k),
                        fieldsdata.value_bin().c_str(),
                        fieldsdata.value_bin().size());
    }

    memcpy(reinterpret_cast<uint8_t *>(key) + field_pos, &k, field_size);
  }

  for (size_t i = 0; i < fields_.size(); i++) {
    int field_size = fields_[i].size;
    int field_pos = fields_[i].pos;

    uint64_t k = 0;

    bess::pb::FieldData fieldsdata = arg.fields(i);
    if (fieldsdata.encoding_case() == bess::pb::FieldData::kValueInt) {
      if (!bess::utils::uint64_to_bin(&k, fieldsdata.value_int(), field_size,
                                      false)) {
        return CommandFailure(EINVAL, "idx %zu: not a correct %d-byte mask", i,
                              field_size);
      }
    } else if (fieldsdata.encoding_case() == bess::pb::FieldData::kValueBin) {
      bess::utils::Copy(reinterpret_cast<uint8_t *>(&k),
                        fieldsdata.value_bin().c_str(),
                        fieldsdata.value_bin().size());
    }

    memcpy(reinterpret_cast<uint8_t *>(l) + field_pos, &k, field_size);
  }

  for (size_t i = 0; i < values_.size(); i++) {
    int val_size = values_[i].size;
    int val_pos = values_[i].pos;

    uint64_t v = 0;
    bess::pb::FieldData valuedata = arg.values(i);
    if (valuedata.encoding_case() == bess::pb::FieldData::kValueInt) {
      if (!bess::utils::uint64_to_bin(&v, valuedata.value_int(), val_size,
                                      false)) {
        return CommandFailure(EINVAL, "idx %zu: not a correct %d-byte value", i,
                              val_size);
      }
    } else if (valuedata.encoding_case() == bess::pb::FieldData::kValueBin) {
      bess::utils::Copy(reinterpret_cast<uint8_t *>(&v),
                        valuedata.value_bin().c_str(),
                        valuedata.value_bin().size());
    }

    memcpy(reinterpret_cast<uint8_t *>(val) + val_pos, &v, val_size);
  }
  return CommandSuccess();
}

CommandResponse Qos::CommandAdd(const bess::pb::QosCommandAddArg &arg) {
  gate_idx_t gate = arg.gate();

  if (!is_valid_gate(gate)) {
    return CommandFailure(EINVAL, "Invalid gate: %hu", gate);
  }
  MeteringKey key = {{0}};

  MKey l;
  value v;
  v.ogate = gate;
  CommandResponse err = ExtractKeyMask(arg, &key, &v.Data, &l);

  if (err.error().code() != 0) {
    return err;
  }

  if (gate == METER_GATE) {
    uint64_t cir = arg.cir();
    uint64_t pir = arg.pir();
    uint64_t cbs = arg.cbs();
    uint64_t pbs = arg.pbs();
    uint64_t ebs = arg.ebs();

    if (arg.optional_deduct_len_case() ==
        bess::pb::QosCommandAddArg::OPTIONAL_DEDUCT_LEN_NOT_SET) {
      v.deduct_len = 14;  // Exclude Ethernet header by default
    } else {
      v.deduct_len = arg.deduct_len();
    }

    DLOG(INFO) << "Adding entry"
               << " cir: " << cir << " pir: " << pir << " cbs: " << cbs
               << " pbs: " << pbs << " ebs: " << ebs << std::endl;

    struct rte_meter_trtcm_params app_trtcm_params = {
        .cir = cir, .pir = pir, .cbs = cbs, .pbs = pbs};

    int ret = rte_meter_trtcm_profile_config(&v.p, &app_trtcm_params);
    if (ret)
      return CommandFailure(
          ret, "Insert Failed - rte_meter_trtcm_profile_config failed");

    ret = rte_meter_trtcm_config(&v.m, &v.p);
    if (ret) {
      return CommandFailure(ret,
                            "Insert Failed - rte_meter_trtcm_config failed");
    }
  }

  table_.Add(v, key);
  return CommandSuccess();
}

CommandResponse Qos::CommandDelete(const bess::pb::QosCommandDeleteArg &arg) {
  MeteringKey key;
  CommandResponse err = ExtractKey(arg, &key);
  table_.Delete(key);
  return CommandSuccess();
}

CommandResponse Qos::CommandClear(__attribute__((unused))
                                  const bess::pb::EmptyArg &) {
  Qos::Clear();
  return CommandSuccess();
}

void Qos::Clear() {
  table_.Clear();
}

void Qos::DeInit() {
  table_.DeInit();
}

CommandResponse Qos::CommandSetDefaultGate(
    const bess::pb::QosCommandSetDefaultGateArg &arg) {
  default_gate_ = arg.gate();
  return CommandSuccess();
}

std::string Qos::GetDesc() const {
  return bess::utils::Format("%zu fields, %zu rules", fields_.size(),
                             table_.Count());
}

ADD_MODULE(Qos, "qos", "Multi-field classifier with a QOS")
