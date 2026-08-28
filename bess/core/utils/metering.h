/*
 * SPDX-License-Identifier: BSD-3-Clause
 * Copyright 2014-2016, The Regents of the University of California.
 * Copyright 2016-2017, Nefeli Networks, Inc.
 * Copyright 2021-present Intel Corporation
 */
#ifndef BESS_UTILS_METERING_H_
#define BESS_UTILS_METERING_H_

#include <string>
#include <type_traits>
#include <vector>

#include "../message.h"
#include "../metadata.h"
#include "../module.h"
#include "../packet.h"
#include "bits.h"
#include "cuckoo_map.h"
#include "endian.h"
#include "format.h"
#include <rte_config.h>
#include <rte_hash_crc.h>
#include <rte_meter.h>

#define MAX_FIELDS 8
#define MAX_FIELD_SIZE 8

static_assert(MAX_FIELD_SIZE <= sizeof(uint64_t),
              "field cannot be larger than 8 bytes");

#define HASH_KEY_SIZE (MAX_FIELDS * MAX_FIELD_SIZE)

#if __BYTE_ORDER__ != __ORDER_LITTLE_ENDIAN__
#error this code assumes little endian architecture (x86)
#endif

struct MeteringField {
  int attr_id;
  int offset;
  int pos;
  int size;
};

namespace bess {
namespace utils {

using Error = std::pair<int, std::string>;

struct MeteringKey {
  uint64_t u64_arr[MAX_FIELDS];
};

// Equality operator for two MeteringKeys
class MeteringKeyEq {
 public:
  explicit MeteringKeyEq(size_t len) : len_(len) {}

  bool operator()(const MeteringKey &lhs, const MeteringKey &rhs) const {
    promise(len_ >= sizeof(uint64_t));
    promise(len_ <= sizeof(MeteringKey));

    for (size_t i = 0; i < len_ / 8; i++) {
      if (lhs.u64_arr[i] != rhs.u64_arr[i]) {
        return false;
      }
    }
    return true;
  }

 private:
  size_t len_;
};

// Hash function for an MeteringKey
class MeteringKeyHash {
 public:
  explicit MeteringKeyHash(size_t len) : len_(len) {}

  HashResult operator()(const MeteringKey &key) const {
    HashResult init_val = 0;

    promise(len_ >= sizeof(uint64_t));
    promise(len_ <= sizeof(MeteringKey));

#if __x86_64
    for (size_t i = 0; i < len_ / 8; i++) {
      init_val = crc32c_sse42_u64(key.u64_arr[i], init_val);
    }
    return init_val;
#else
    return rte_hash_crc(&key, len_, init_val);
#endif
  }

 private:
  size_t len_;
};

template <typename T>
class Metering {
 public:
  struct rte_hash_parameters dpdk_params {
    .name = "Metering", .entries = 1 << 15, .reserved = 0,
    .key_len = sizeof(MeteringKey), .hash_func = rte_hash_crc,
    .hash_func_init_val = 0, .socket_id = (int)rte_socket_id(),
    .extra_flag = RTE_HASH_EXTRA_FLAGS_RW_CONCURRENCY
  };

  using EmTable = CuckooMap<MeteringKey, T, MeteringKeyHash, MeteringKeyEq>;
  Metering() : total_key_size_(0), num_fields_(0) {}

  Error Add(const T &val, const MeteringKey &key) {
    Error err;
    const void *Key_t = (const void *)&key;
    T *val_t = new T(val);
    int ret = table_->insert_dpdk(Key_t, val_t);
    if (!ret) {
      return MakeError(ENOENT, "Dpdk Insert Failed");
    }
    return MakeError(0);
  }

  Error Delete(const MeteringKey &key) {
    Error err;
    bool ret = table_->Remove(key, MeteringKeyHash(total_key_size_),
                              MeteringKeyEq(total_key_size_));
    if (!ret) {
      return MakeError(ENOENT, "rule doesn't exist");
    }
    return MakeError(0);
  }

  void Clear() { table_->Clear(); }
  size_t Count() const { return table_->Count(); }
  void DeInit() { table_->DeInit(); }

  // Find an entry in the table.
  // Returns the value if `key` matches a rule, otherwise `default_value`.
  T Find(const MeteringKey &key, const T &default_value) const {
    const auto &table = table_;
    void *data = nullptr;
    table->find_dpdk(&key, &data);
    if (data) {
      T data_t = *((T *)data);
      return data_t;
    } else

      return default_value;
  }

  uint64_t Find(MeteringKey *keys, T **vals, int n) {
    uint64_t hit_mask = 0;

    const auto &table = table_;
    std::vector<MeteringKey *> key_ptr(n);
    for (int h = 0; h < n; h++)
      key_ptr[h] = &keys[h];
    table->lookup_bulk_data((const void **)key_ptr.data(), n, &hit_mask,
                            (void **)vals);

    return hit_mask;
  }

  uint32_t Total_key_size() const { return total_key_size_; }

  void Init(int size, int entries) {
    std::ostringstream address;
    total_key_size_ = size;
    address << &table_;
    std::string name = "Metering" + address.str();
    dpdk_params.name = name.c_str();
    if (entries) {
      dpdk_params.entries = entries;
    }
    dpdk_params.key_len = size;
    table_.reset(new CuckooMap<MeteringKey, T, MeteringKeyHash, MeteringKeyEq>(
        0, 0, &dpdk_params));
  }

 private:
  Error MakeError(int code, const std::string &msg = "") {
    return std::make_pair(code, msg);
  }

  // aligned total key size
  size_t total_key_size_;
  size_t num_fields_;
  std::unique_ptr<CuckooMap<MeteringKey, T, MeteringKeyHash, MeteringKeyEq>>
      table_;
};

}  // namespace utils
}  // namespace bess

#endif
