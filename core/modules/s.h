/*
 * SPDX-License-Identifier: BSD-3-Clause
 * Copyright 2014-2016, The Regents of the University of California.
 * Copyright 2016-2017, Nefeli Networks, Inc.
 * Copyright 2021-present Intel Corporation
 */

#ifndef BESS_MODULES_SCARDMATCH_H_
#define BESS_MODULES_SCARDMATCH_H_

#include "../module.h"
#include <rte_cfgfile.h>
#include <rte_config.h>
#include <rte_hash_crc.h>

#include "../pb/module_msg.pb.h"
#include "../utils/cuckoo_map.h"
#include <rte_sched.h>

using bess::utils::HashResult;
using bess::utils::CuckooMap;

#define GBR_PORT 0
#define DROP_PORT 1
#define LAST_QFI 86
#define MAX_FIELDS 8
#define MAX_FIELD_SIZE 8
static_assert(MAX_FIELD_SIZE <= sizeof(uint64_t),"field cannot be larger than 8 bytes");

#define HASH_KEY_SIZE (MAX_FIELDS * MAX_FIELD_SIZE)
#define MAX_SCHED_SUBPORT_PROFILES 1
#define MAX_SCHED_PIPES 1
#define MAX_SCHED_PIPE_PROFILES 1
#define MAX_SCHED_SUBPORTS 1


struct SchField {
  int attr_id;
  int offset;
  int pos;
  int size;
};

  struct schedule
  {
    
    int32_t qfi;
    int32_t subport;
    int32_t pipe;
    int32_t tc;
    int32_t queue;
  } ;

enum { FieldType1 = 0, ValueType1 };

struct SchKey {
  uint64_t u64_arr[MAX_FIELDS];
} __attribute__((packed));

class Sch final : public Module {
 public:
 std::mutex m_lock;
 Sch() : Module(), default_gate_() {
    max_allowed_workers_ = Worker::kMaxWorkers;
    size_t len = sizeof(mask) / sizeof(uint64_t);
    for (size_t i = 0; i < len; i++)
      mask[i] = 0;
  }
  struct rte_sched_subport_profile_params subport_profile[MAX_SCHED_SUBPORT_PROFILES];
  static const Commands cmds;

  static const gate_idx_t kNumOGates = MAX_GATES;
  int cfg_load_port(struct rte_cfgfile *cfg, struct rte_sched_port_params *port_params);
  int cfg_load_pipe(struct rte_cfgfile *cfg, struct rte_sched_pipe_params *pipe_params);
  int cfg_load_subport(struct rte_cfgfile *cfg, struct rte_sched_subport_params *subport_params);
  int cfg_load_subport_profile(struct rte_cfgfile *cfg,struct rte_sched_subport_profile_params *subport_profile);
  int cfg_load_qfi_profile(struct rte_cfgfile *cfg);
  struct schedule scheduler_params[LAST_QFI];
  CommandResponse Init(const bess::pb::SchArg &arg);
  void ProcessBatch(Context *ctx, bess::PacketBatch *batch) override;
  CommandResponse CommandSetDefaultGate(
  const bess::pb::SchCommandSetDefaultGateArg &arg);
  CommandResponse SchedulerInit();
  struct rte_sched_port_params port_params ;
 
  struct rte_sched_subport_params subport_params[MAX_SCHED_SUBPORTS];
  struct rte_sched_pipe_params pipe_profiles[MAX_SCHED_PIPE_PROFILES];
  int app_pipe_to_profile[MAX_SCHED_SUBPORTS][MAX_SCHED_PIPES];  
  uint32_t pipe, subport;
  uint32_t pipes_per_subport;
  uint32_t subports_per_port;
  CommandResponse AddFieldOne(const bess::pb::Field &field,
                              struct SchField *f, uint8_t type);
private:
  struct rte_sched_port *port = NULL;
  gate_idx_t default_gate_; 
  std::vector<struct SchField> fields_;
  std::vector<struct SchField> values_;  

  size_t total_key_size_; 
  size_t total_value_size_;
  uint64_t mask[MAX_FIELDS];

};
#endif
