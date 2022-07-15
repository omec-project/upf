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

#ifndef BESS_MODULES_SCARDMATCH_H_
#define BESS_MODULES_SCARDMATCH_H_

#include "../module.h"

#include <rte_config.h>
#include <rte_hash_crc.h>

#include "../pb/module_msg.pb.h"
#include "../utils/cuckoo_map.h"
#include <rte_sched.h>

using bess::utils::HashResult;
using bess::utils::CuckooMap;


#define MAX_FIELDS 8
#define MAX_FIELD_SIZE 8
static_assert(MAX_FIELD_SIZE <= sizeof(uint64_t),"field cannot be larger than 8 bytes");

#define HASH_KEY_SIZE (MAX_FIELDS * MAX_FIELD_SIZE)
#define MAX_SCHED_SUBPORT_PROFILES 1
#define MAX_SCHED_PIPES 4096
#define MAX_SCHED_PIPE_PROFILES 256
#define MAX_SCHED_SUBPORTS 8


struct SchField {
  int attr_id;
  int offset;
  int pos;
  int size;
};

enum { FieldType1 = 0, ValueType1 };

struct SchKey {
  uint64_t u64_arr[MAX_FIELDS];
} __attribute__((packed));

class Sch final : public Module {
 public:
 Sch() : Module(), default_gate_() {
    max_allowed_workers_ = Worker::kMaxWorkers;
    size_t len = sizeof(mask) / sizeof(uint64_t);
    for (size_t i = 0; i < len; i++)
      mask[i] = 0;
  }

  //static const gate_idx_t kNumOGates = MAX_GATES;
  struct rte_sched_subport_profile_params subport_profile[MAX_SCHED_SUBPORT_PROFILES];
  static const Commands cmds;
    std::map<int, std::pair<int,int>>gbr = { {1,{20,4}},{2,{40,4}},{3,{30,4}},{4,{50,5}},{65,{7,5}},{66,{20,5}},{67,{15,6}},{75,{-1,6}},{71,{56,6}},{72,{56,7}},{73,{56,7}},{74,{56,7}},{76,{56,8}},{5,{10,9}},{6,{60,9}},{7,{70,10}},{8,{80,10}},{9,{90,11}},{69,{5,11}},{70,{55,12}},{79,{65,12}},{80,{68,12}},{82,{19,0}},{83,{22,1}},{84,{24,2}},{85,{21,3}} };

  static const gate_idx_t kNumOGates = MAX_GATES;
  int cfg_load_port(struct rte_cfgfile *cfg, struct rte_sched_port_params *port_params);
  int cfg_load_pipe(struct rte_cfgfile *cfg, struct rte_sched_pipe_params *pipe_params);
  int cfg_load_subport(struct rte_cfgfile *cfg, struct rte_sched_subport_params *subport_params);
  int cfg_load_subport_profile(struct rte_cfgfile *cfg,struct rte_sched_subport_profile_params *subport_profile);
CommandResponse Init(const bess::pb::SchArg &arg);
  void ProcessBatch(Context *ctx, bess::PacketBatch *batch) override;
  CommandResponse CommandSetDefaultGate(
      const bess::pb::SchCommandSetDefaultGateArg &arg);
  CommandResponse SchedulerInit();
   struct rte_sched_port_params port_params ;
 //int cfg_load_subport_profile(struct rte_cfgfile *cfg,	struct rte_sched_subport_profile_params *subport_profile);

 struct rte_sched_subport_params subport_params[MAX_SCHED_SUBPORTS];
struct rte_sched_pipe_params pipe_profiles[MAX_SCHED_PIPE_PROFILES];
int app_pipe_to_profile[MAX_SCHED_SUBPORTS][MAX_SCHED_PIPES];  
uint32_t pipe, subport;
CommandResponse AddFieldOne(const bess::pb::Field &field,
                              struct SchField *f, uint8_t type);
private:
struct rte_sched_port *port = NULL;
gate_idx_t default_gate_; 
  std::vector<struct SchField> fields_;
  std::vector<struct SchField> values_;  

  size_t total_key_size_; /* a multiple of sizeof(uint64_t) */
  size_t total_value_size_;
  uint64_t mask[MAX_FIELDS];
//ffffffffffffffffff
};
#endif
