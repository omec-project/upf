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
/*
 * SPDX-License-Identifier: BSD-3-Clause
 * Copyright 2014-2016, The Regents of the University of California.
 * Copyright 2016-2017, Nefeli Networks, Inc.
 * Copyright 2021-present Intel Corporation
 */
/*
 * SPDX-License-Identifier: BSD-3-Clause
 * Copyright 2014-2016, The Regents of the University of California.
 * Copyright 2016-2017, Nefeli Networks, Inc.
 * Copyright 2021-present Intel Corporation
 */

#include "utils/endian.h"
#include "utils/format.h"
#include "../packet_pool.h"
#include <rte_cycles.h>
#include <string>
#include <vector>
#include "../utils/ether.h"
#include "../utils/ip.h"
#include "../utils/udp.h"
#include <rte_cfgfile.h>
#include "cfg_file.c"
#include "s.h"
#include "../drivers/pmd.h"

using namespace std;

typedef enum { FIELD_TYPE = 0, VALUE_TYPE } Type;
using bess::metadata::Attribute;
using bess::utils::Ethernet;
using bess::utils::Ipv4;

const Commands Sch::cmds = {
    {"set_default_gate", "SchCommandSetDefaultGateArg",
     MODULE_CMD_FUNC(&Sch::CommandSetDefaultGate), Command::THREAD_SAFE}};

CommandResponse Sch::Init(const bess::pb::SchArg &arg) {
  int size_acc = 0;

  for (int i = 0; i < arg.fields_size(); i++) {
    const auto &field = arg.fields(i);
    CommandResponse err;
    fields_.emplace_back();
    struct SchField &f = fields_.back();
    f.pos = size_acc;
    err = AddFieldOne(field, &f, FieldType1);
    if (err.error().code() != 0) {
      return err;
    }

    size_acc += f.size;
  }

  uint8_t *cs = (uint8_t *)&mask;
  for (int i = 0; i < size_acc; i++) {
    cs[i] = 0xff;
  }

  SchedulerInit();
  return CommandSuccess();
}

CommandResponse Sch::AddFieldOne(const bess::pb::Field &field,
                                 struct SchField *f, uint8_t type) {
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
        (type == FieldType1)
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

void Sch::ProcessBatch(Context *ctx, bess::PacketBatch *batch) {
  //int log=0;
  uint32_t col =-1;
  uint16_t ogate=0;
  SchKey keys[bess::PacketBatch::kMaxBurst] __ymm_aligned;
  bess::Packet *pkt = nullptr;
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
  struct rte_mbuf *m[cnt];
  bool flag=0;
  for (int j = 0; j < cnt; j++) {
    pkt = batch->pkts()[j];
    flag=1; 
    uint8_t key2 = (uint8_t) (keys[j].u64_arr[0]);
   
    m[j] = reinterpret_cast<struct rte_mbuf *>(pkt);
    
    col=RTE_COLOR_GREEN;
  
    uint32_t qf= (uint32_t) key2;
    if( (scheduler_params[qf].subport == -1)||(qf>85)||(qf<=0) )
    {
     EmitPacket(ctx, pkt, DROP_PORT);
     //if(log)  
     continue;
    }
        
    rte_sched_port_pkt_write(port, m[j], /*subport*/ scheduler_params[qf].subport, /*pipe*/ scheduler_params[qf].pipe,
                             /*tc*/ scheduler_params[qf].tc, /*queue*/ scheduler_params[qf].queue, /*color*/ (enum rte_color)col);//RTE_COLOR_YELLLOW);
    
   }
   
if(flag)
 {  
   m_lock.lock();
   int u = rte_sched_port_enqueue(port, m,cnt );
   m_lock.unlock();

  if(u != cnt)
  {
    if(u==0)
     {
       for (int j = 0; j < cnt; j++) 
        {
         pkt = batch->pkts()[j];
         EmitPacket(ctx, pkt, DROP_PORT);
         continue;
        }
     }
    else 
     {
       for (int j = 0; j < cnt; j++) 
        {
        pkt = batch->pkts()[j];          
        struct rte_mbuf *m = reinterpret_cast<struct rte_mbuf *>(pkt);
        uint8_t ans = rte_sched_port_pkt_read_color(m);
        if(ans == 254)
        { 
          EmitPacket(ctx, pkt, DROP_PORT);
        }
       }
     }
     

  }

  struct rte_mbuf *tx_mbufs[cnt];
  int retu = 0;
  if (1)
   {   
    retu=0;

    m_lock.lock();
    retu = rte_sched_port_dequeue(port, tx_mbufs, (cnt)/*queue_length*/);
    m_lock.unlock();

    int k=0;
    while (retu)
    { 
   uint8_t ans = rte_sched_port_pkt_read_color(tx_mbufs[k]);

   bess::Packet *pkt2 = reinterpret_cast<bess::Packet *>(tx_mbufs[k]);    

   if(ans == 254)
    ogate = DROP_GATE;
  else
    ogate = GBR_PORT;
   
  EmitPacket(ctx, pkt2, ogate);
  retu--;
  k++;
  }

 }
 
 }
    

}


CommandResponse Sch::CommandSetDefaultGate(
    const bess::pb::SchCommandSetDefaultGateArg &arg) {
  default_gate_ = arg.gate();
  return CommandSuccess();
}

CommandResponse Sch::SchedulerInit() {

  char *p;
  if( ( p = getcwd(NULL, 0)) == NULL) {
        perror("failed to get current directory\n");
    } 
    string s(p);
    s= s+ "/conf/scheduler.cfg";
    
    struct rte_cfgfile *file = rte_cfgfile_load(s.c_str(), 0);

  if (file == NULL)
    {
      return CommandFailure(EINVAL, "scheduler config file not loaded");      
    }
  else           
      std::cout<< "config file loaded-hrrah!!!"<<std::endl;

 for(int i=0; i<LAST_QFI; i++)
  {
   scheduler_params[i].qfi = scheduler_params[i].subport = scheduler_params[i].pipe = scheduler_params[i].tc = scheduler_params[i].queue =-1;
  }

  cfg_load_port(file, &port_params);
	cfg_load_subport(file, subport_params);
	cfg_load_subport_profile(file, subport_profile);
  cfg_load_pipe(file, pipe_profiles);
  cfg_load_qfi_profile(file);
	rte_cfgfile_close(file);
 
for(int i=0; i< MAX_SCHED_SUBPORTS; i++)  //print subport info and params just
{
 subport_params[i].pipe_profiles = pipe_profiles;
 subport_params[i].n_pipe_profiles = sizeof(pipe_profiles) /	sizeof(struct rte_sched_pipe_params);
 subport_params[i].n_max_pipe_profiles = MAX_SCHED_PIPE_PROFILES;
 
}


  port_params.name = "port_Scheduler_0";
	port_params.mtu = 6 + 6 + 4 + 4 + 2 + 1500;
	port_params.frame_overhead = RTE_SCHED_FRAME_OVERHEAD_DEFAULT;
	port_params.n_subport_profiles = 1;
	port_params.subport_profiles = subport_profile;
	port_params.n_max_subport_profiles = MAX_SCHED_SUBPORT_PROFILES;
	port_params.n_pipes_per_subport = MAX_SCHED_PIPES;
  char port_name[32];
	port_params.socket = rte_socket_id() == LCORE_ID_ANY ? 0 : rte_socket_id();//socketid;
	snprintf(port_name, sizeof(port_name), "port_%d", /*portid*/0);
	port_params.name = port_name;
  
  port = rte_sched_port_config(&port_params);
	if (port == NULL){
		rte_exit(EXIT_FAILURE, "Unable to config Sched port\n");
	}
  
	for (subport = 0; subport < port_params.n_subports_per_port; subport ++) {
   
	  int err = rte_sched_subport_config(port, subport, &subport_params[subport],0);
		if (err) {
			rte_exit(EXIT_FAILURE, "Unable to config Sched subport %u, err=%d\n",
					subport, err);
		}
   
		uint32_t n_pipes_per_subport = subport_params[subport].n_pipes_per_subport_enabled;

   for (pipe = 0; pipe < n_pipes_per_subport; pipe++) 
    {
			if (app_pipe_to_profile[subport][pipe] != -1) 
      {
				err = rte_sched_pipe_config(port, subport, pipe,
						app_pipe_to_profile[subport][pipe]);
				if (err) 
        {
					rte_exit(EXIT_FAILURE, "Unable to config Sched pipe %u "
							"for profile %d, err=%d\n", pipe,
							app_pipe_to_profile[subport][pipe], err);
				}
			}

		}
	}  
return CommandSuccess();
}
////////////////////////////////////////////////////////////////

ADD_MODULE(Sch, "Sch", "Multi-field classifier with a Sched")
