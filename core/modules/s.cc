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

//#include "qos.h"
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
//#include "../utils/a.h"
//#include "cfg_file.h"
#include "conf.h"
#include "../drivers/pmd.h"

using namespace std;

//int app_pipe_to_profile[MAX_SCHED_SUBPORTS][MAX_SCHED_PIPES];
typedef enum { FIELD_TYPE = 0, VALUE_TYPE } Type;
using bess::metadata::Attribute;
using bess::utils::Ethernet;
using bess::utils::Ipv4;

//#define metering_test 0

const Commands Sch::cmds = {
    {"set_default_gate", "SchCommandSetDefaultGateArg",
     MODULE_CMD_FUNC(&Sch::CommandSetDefaultGate), Command::THREAD_SAFE}};

CommandResponse Sch::Init(const bess::pb::SchArg &arg) {
  //int size_acc = 0;
  //int value_acc = 0;
  //uint64_t a = arg.q();
  
  //default_gate_ = 0;
//////////////////////////////////////////////////
  int size_acc = 0;
  //int value_acc = 0;

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
  /*
  default_gate_ = 1;//DROP_GATE;
  total_key_size_ = align_ceil(size_acc, sizeof(uint64_t));
  for (int i = 0; i < arg.values_size(); i++) {
    const auto &field = arg.values(i);
    CommandResponse err;
    values_.emplace_back();
    struct SchField &f = values_.back();
    f.pos = value_acc;
    err = AddFieldOne(field, &f, ValueType1);
    if (err.error().code() != 0) {
      return err;
    }

    value_acc += f.size;
  }

  total_value_size_ = align_ceil(value_acc, sizeof(uint64_t));
  */

  //std::cout << "mask0=" << mask[0]<<mask[1]<<mask[2]<<mask[3]<<mask[4]<<mask[5]<<mask[6]<<mask[7] << std::endl;
  uint8_t *cs = (uint8_t *)&mask;
  for (int i = 0; i < size_acc; i++) {
    cs[i] = 0xff;
  }

  //std::cout << "mask1=" << mask[0]<<mask[1]<<mask[2]<<mask[3]<<mask[4]<<mask[5]<<mask[6]<<mask[7] << std::endl;
  //table_.Init(total_key_size_, arg.entries());
  //return CommandSuccess();
////////////////////////////////////////////////
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

#if 0
void Sch::ProcessBatch(Context *ctx, bess::PacketBatch *batch) {
bess::Packet *pkt = nullptr;
//gate_idx_t default_gate;
  gate_idx_t  ogate=0;
 // default_gate = ACCESS_ONCE(default_gate_);
//
  int cnt = batch->cnt();
//  std::cout<<"sch:cnt="<<cnt <<std::endl;


  for (int j = 0; j < cnt; j++) {
    pkt = batch->pkts()[j];
    //if ((hit_mask & ((uint64_t)1ULL << j)) == 0) 
    {
      EmitPacket(ctx, pkt, ogate);
      //continue;
    }
  }


}
#endif

//#if 0
void Sch::ProcessBatch(Context *ctx, bess::PacketBatch *batch) {
int log=0;
uint32_t col =-1;
  ////////////////////////////////////////////////////////
  gate_idx_t default_gate;
  uint16_t ogate=0;
  SchKey keys[bess::PacketBatch::kMaxBurst] __ymm_aligned;
  bess::Packet *pkt = nullptr;
  default_gate = ACCESS_ONCE(default_gate_);
//std::cout<<"pb cald"<<std::endl;
  int cnt = batch->cnt();
  //value *val[cnt];

//#if 0
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
  //    pkt1 = batch->pkts()[j];
      
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
   //     std::cout << "mask[i]="<<mask[i] << "keys[j].u64_arr[i] ="<<keys[j].u64_arr[i];
      }

 //std::cout <<"keys0" << keys[j].u64_arr[0] <<" 1="<< keys[j].u64_arr[1] <<" 2=" << keys[j].u64_arr[2] << " 3="<< keys[j].u64_arr[3]<< " 4="<<keys[j].u64_arr[4]<<" 5"<<keys[j].u64_arr[5]<<" 6="<<keys[j].u64_arr[6]<<" 7="<<keys[j].u64_arr[7]<<std::endl; 

    }
  }
//#endif
  ////////////////////////////////////////////////////////
  if (log)
  std::cout <<"a"<<std::endl;
  // std::cout << "enqueue started" << hit_mask<<std::endl;
  
  struct rte_mbuf *m[cnt];
  bool flag=0;
  for (int j = 0; j < cnt; j++) {
    pkt = batch->pkts()[j];
    flag=1; 
    //ogate = 0;
    uint8_t key2 = (uint8_t) (keys[j].u64_arr[0]);
  #if 0
    size_t num_values_ = values_.size();
    for (size_t i = 0; i < num_values_; i++) {
      
      int value_size = values_[i].size;
      int value_pos = values_[i].pos;
      int value_off = values_[i].offset;
      int value_attr_id = values_[i].attr_id;
      uint8_t *data = pkt->head_data<uint8_t *>() + value_off;

  //std::cout <<"d"<<std::endl;
      if (value_attr_id < 0) { /* if it is offset-based */
        memcpy(data, reinterpret_cast<uint8_t *>(&(val[j]->Data)) + value_pos,
               value_size);
      } else { /* if it is attribute-based */
        typedef struct {
          uint8_t bytes[bess::metadata::kMetadataAttrMaxSize];
        } value_t;
        uint8_t *buf = (uint8_t *)(&(val[j]->Data)) + value_pos;

  //std::cout <<"e"<<std::endl;
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
#endif
////////////////////////////////////////////////////////////////////////////////////
//Schedule packet

  //Ethernet *eth = pkt->head_data<Ethernet *>();
  
  //Ipv4 *iph = (Ipv4 *)((unsigned char *)eth + sizeof(Ethernet));

  
    m[j] = reinterpret_cast<struct rte_mbuf *>(pkt);
    
    //ip = reinterpret_cast<struct rte_ipv4_hdr *>(iph);
    //m[j]->l2_len = sizeof(*eth);
    //m[j]->l3_len = sizeof(*iph);
    //auto itr = gbr.find(val[j]->q);
    //int h =itr->second.second;

    if(log)
    cout << "key2=" << static_cast<unsigned>(key2) << std::endl;

   // auto itr = gbr.find(key2);
   // int h =itr->second.second;
   // if(log)
   // std::cout <<"h="<<h<<std::endl;

 
    //if(key2==1)
      col=RTE_COLOR_GREEN;
    //else if(key2==2)
      //col=RTE_COLOR_YELLOW;

    uint32_t qf= (uint32_t) key2;
    if( (scheduler_params[qf].subport == -1)||(qf>85)||(qf<=0) )
    {
     EmitPacket(ctx, pkt, DROP_PORT);
     if(log)
     std::cout << "drop=" <<std::endl;
     continue;
    }
 if(log)
  {std::cout << "qf=" <<qf << "subport="<< scheduler_params[qf].subport << "pipe=" << scheduler_params[qf].pipe <<
                             "tc=" <<scheduler_params[qf].tc << "queue=" << scheduler_params[qf].queue <<std::endl;
  }                          
  rte_sched_port_pkt_write(port, m[j], /*subport*/ scheduler_params[qf].subport, /*pipe*/ scheduler_params[qf].pipe,
                             /*tc*/ scheduler_params[qf].tc, /*queue*/ scheduler_params[qf].queue, /*color*/ (enum rte_color)col);//RTE_COLOR_YELLLOW);


  }
   
if(flag)
 {  
  uint16_t queue_length;
  //std::cout<<"a1-cnt="<<cnt<<std::endl;
  //m = reinterpret_cast<struct rte_mbuf *>(batch->pkts()[0]);
  int u = rte_sched_port_enqueue(port, m,cnt );
  if(log) 
    {
    std::cout<<"a2-u-enqueue-no-of-packets="<<u<<"cnt="<<cnt<<std::endl;
    //std::cout << "enqueued =" <<cnt<<"color="<<col<<std::endl;
    }

  if(u != cnt)
  {
   for(int h =0;h<cnt;h++)
   {
    if(log)
    std::cout << h << "." <<" enqueued pkt Color=" <<  rte_sched_port_pkt_read_color(m[h]) << std::endl;
    
   }

  }
  //sleep(1);
  //if(cnt && (u==0))
  {
    rte_sched_queue_stats queue_stats;
    // uint16_t queue_length1;     
    int err = rte_sched_queue_read_stats(port, 0, &queue_stats,&queue_length);

    if (err) 
    {
       std::cout << "rte_Sched_queue_read_stats failed-Queue0=" << err <<std::endl;
       return;
    }
    else
    {
    if(log)
     std::cout<< "Queue 0" << ": current length " << queue_length
         << ", packets " << queue_stats.n_pkts << ", packets dropped "
         << queue_stats.n_pkts_dropped << ", bytes " << queue_stats.n_bytes
         << ", bytes dropped " << queue_stats.n_bytes_dropped <<std::endl;

    }
  }
  //std::cout << "Enqueue end" << count2<<std::endl;
    
    struct rte_mbuf *tx_mbufs[cnt/*queue_length*/];
    int retu = 0;
    uint32_t subport, traffic_class, queue,pipe;

  if (1/*queue_length*/)
   {   
    retu=0;

    if(log)
    std::cout <<"queue_length="<<queue_length<<std::endl;
    
    retu = rte_sched_port_dequeue(port, tx_mbufs, cnt/*queue_length*/);
    
    if(log)
     std::cout << "dequeue-no-of-packets="<<retu<<std::endl;

    int k=0;
    while (retu)
    { //count1++;
    if(log)
    std::cout << "k="<<k<<std::endl;    
    rte_sched_port_pkt_read_tree_path(port, tx_mbufs[k],
				&subport, &pipe, &traffic_class, &queue);
    if(log)
    std::cout << k << "." <<"dequeued pkt Color=" <<  rte_sched_port_pkt_read_color(tx_mbufs[k]) << std::endl;
    
    if (log)
    std::cout << "dequeued subport=" <<subport << "pipe=" <<pipe << "dequeued packet traffic class =" << traffic_class<< "queue=" << queue << std::endl;
    
    bess::Packet *pkt2 = reinterpret_cast<bess::Packet *>(tx_mbufs[k]);    

   if(traffic_class != 12)
    ogate = GBR_PORT;//traffic_class;
   else if(traffic_class == 12)
    ogate = NONGBR_PORT;
   else 
    ogate = DROP_GATE;
if(log)
  std::cout<<"before emit pkt"<<std::endl;
     EmitPacket(ctx, pkt2, ogate);
     retu--;
     k++;
  } ;//while(retu);

 }
 else
 {
   for (int j = 0; j < cnt; j++) {
    pkt = batch->pkts()[j];
    //if ((hit_mask & (1ULL << j)) == 0) 
    {
      EmitPacket(ctx, pkt, default_gate);
      continue;
    }
   }

 }
}
  //std::cout <<"dequeue end ="<<count1<<std::endl;

#if post_enqueue_color
      int olor = rte_Sched_port_pkt_read_color(tx_mbufs[0]);
      if(log)
      std::cout<<"color-y1="<<olor<<std::endl;
      
       olor = rte_Sched_port_pkt_read_color(tx_mbufs[1]);
      if(log)
      std::cout<<"color-y2="<<olor<<std::endl;
   
      olor = rte_Sched_port_pkt_read_color(tx_mbufs[2]);
      std::cout<<"color-y3="<<olor<<std::endl;
   
     olor = rte_Sched_port_pkt_read_color(tx_mbufs[3]);
      std::cout<<"color-y4="<<olor<<std::endl;
   
     olor = rte_Sched_port_pkt_read_color(tx_mbufs[4]);
      std::cout<<"color-y5="<<olor<<std::endl;
   
     olor = rte_Sched_port_pkt_read_color(tx_mbufs[5]);
      std::cout<<"color-y6="<<olor<<std::endl;
   
     olor = rte_Sched_port_pkt_read_color(tx_mbufs[6]);
      std::cout<<"color-y7="<<olor<<std::endl;
   
     olor = rte_Sched_port_pkt_read_color(tx_mbufs[7]);
      std::cout<<"color-y8="<<olor<<std::endl;
 #endif  
     
  
#ifdef stats1   
   rte_Sched_queue_stats queue_stats;
   //   rte_Sched_queue_stats queue_stats1;
   //for (uint32_t queueno = 0; queueno < 1; ++queue) 
   {
     uint16_t queue_length;// uint16_t queue_length1;     
     int err = rte_Sched_queue_read_stats(port, ogate, &queue_stats,&queue_length);

     if (err) {
       LOG(ERROR) << "rte_Sched_queue_read_stats failed-Queue0=" << err;
       return;
     }

     //LOG_EVERY_N(INFO, 1024)
         std::cout<< "Queue 0" << ": current length " << queue_length
         << ", packets " << queue_stats.n_pkts << ", packets dropped "
         << queue_stats.n_pkts_dropped << ", bytes " << queue_stats.n_bytes
         << ", bytes dropped " << queue_stats.n_bytes_dropped <<std::endl;


     //err = rte_Sched_queue_read_stats(port, 1, &queue_stats1,&queue_length1);

     /*if (err) {
       LOG(ERROR) << "rte_Sched_queue_read_stats failed-Queue1= " << err;
       return;
     }*/
          /*std::cout<<"\n" <<"Queue 1" << ": current length " << queue_length1
         << ", packets " << queue_stats1.n_pkts << ", packets dropped "
         << queue_stats1.n_pkts_dropped << ", bytes " << queue_stats1.n_bytes
         << ", bytes dropped " << queue_stats1.n_bytes_dropped;
*/
   }
 #endif


}
//#endif

CommandResponse Sch::CommandSetDefaultGate(
    const bess::pb::SchCommandSetDefaultGateArg &arg) {
  default_gate_ = arg.gate();
  return CommandSuccess();
}

/*
std::string Sch::GetDesc() const {
  return bess::utils::Format("%zu fields, %zu rules", fields_.size(),
                             table_.Count());
}
*/
/////////////////////////////////////////////////////////////////
CommandResponse Sch::SchedulerInit() {
//struct rte_Sched_subport_profile_params subport_profile[1];
#if 0
int err;
struct rte_eth_link link;
	err = rte_eth_link_get(portid, &link);
	if (err < 0)
		rte_exit(EXIT_FAILURE,
			 "rte_eth_link_get: err=%d, port=%u: %s\n",
			 err, portid, rte_strerror(-err));
#endif
  
  char *p;
  
  if( ( p = getcwd(NULL, 0)) == NULL) {
        perror("failed to get current directory\n");
    } 
    string s(p);
    s= s+ "/conf/profile1.cfg";
    
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
 

for(int i=0; i< MAX_SCHED_SUBPORT_PROFILES; i++)  //print subport profile routine just
{
    std::cout << "subport_profile[" << i << "].tb_rate=" << " "<<subport_profile[i].tb_rate <<std::endl;//= 1250000000;
		std::cout << "subport_profile[" << i << "].tb_size=" << " "<<subport_profile[i].tb_size<<std::endl;// = 1000000;
    std::cout << "subport_profile[" << i << "].tc_period=" << " "<<subport_profile[i].tc_period<<std::endl;// = 10;
    
    for(int j=0;j<13;j++)
    {
		  std::cout<<"     " << "subport_profile[" << i << "].tc_rate[" <<j<< "]=" << " "<<subport_profile[i].tc_rate[j]<<std::endl;// = 1250000000;    
    }
    //std::cout << "\n" << std::endl;
}


for(int i=0; i< MAX_SCHED_PIPE_PROFILES; i++)  //print pipe profile just 
{
		std::cout<<	"pipe_profiles[" << i <<"].tb_rate="<< " "	<<pipe_profiles[i].tb_rate<<std::endl;// = 305175;
		std::cout<<	"pipe_profiles[" <<i <<"].tb_size="<< " "<<pipe_profiles[i].tb_size <<std::endl;//= 1000000;

		std::cout<<	"pipe_profiles[" <<i <<"].tc_period="<< " "<<pipe_profiles[i].tc_period<<std::endl;
    std::cout<<	"pipe_profiles[" <<i << "].tc_ov_weight="<< " "<<pipe_profiles[i].tc_ov_weight <<std::endl;

    for(int j=0;j<13;j++)
    {
    std::cout<< "pipe_profiles[" << i << "].tc_rate[" <<j<<"]" << " "<<pipe_profiles[i].tc_rate[j] <<std::endl;
    }
  std::cout <<"pipe_profiles["<<i << "].wrr_weights[0]="<< " "<<pipe_profiles[i].wrr_weights[0]<< " "<< pipe_profiles[i].wrr_weights[1] << " "<< pipe_profiles[i].wrr_weights[2] << " "<< pipe_profiles[i].wrr_weights[3]<<std::endl;
	std::cout << "\n"<<std::endl;
 }


for(int i=0; i< MAX_SCHED_SUBPORTS; i++)  //print subport info and params just
{
 subport_params[i].pipe_profiles = pipe_profiles;
 subport_params[i].n_pipe_profiles = sizeof(pipe_profiles) /	sizeof(struct rte_sched_pipe_params);
 subport_params[i].n_max_pipe_profiles = MAX_SCHED_PIPE_PROFILES;
 
 std::cout << "i=" << i << " "<< "subport_params[i].n_pipes_per_subport_enabled = " << subport_params[i].n_pipes_per_subport_enabled <<std::endl;

 std::cout << "i="<<i << " "<< "subport_params[" << i << "].pipe_profiles="<<subport_params[i].pipe_profiles << 
 "subport_params[" << i << "].n_pipe_profiles=" << subport_params[i].n_pipe_profiles << 
 "subport_params[" << i << "].n_max_pipe_profiles=" << subport_params[i].n_max_pipe_profiles <<std::endl;

  for(int j=0;j<13;j++)
  {
    std::cout << "subport_params[" << i << "].qsize[" << j << "] = " << subport_params[i].qsize[j] << std::endl;

  }


}


  port_params.name = "port_Scheduler_0";
	port_params.socket = 0; /* computed */
  //port_params.rate = port_params->rate;//1250305175; /* computed */
  std::cout<<"rate="<<port_params.rate <<std::endl;
	port_params.mtu = 6 + 6 + 4 + 4 + 2 + 1500;
	port_params.frame_overhead = RTE_SCHED_FRAME_OVERHEAD_DEFAULT;
	//port_params.n_subports_per_port = port_params->n_subports_per_port;
	port_params.n_subport_profiles = 1;
	port_params.subport_profiles = subport_profile;
	port_params.n_max_subport_profiles = MAX_SCHED_SUBPORT_PROFILES;
	port_params.n_pipes_per_subport = MAX_SCHED_PIPES;
  std::cout<<"rate="<<port_params.rate<<std::endl;
   char port_name[32];
	port_params.socket = rte_socket_id() == LCORE_ID_ANY ? 0 : rte_socket_id();//socketid;
	snprintf(port_name, sizeof(port_name), "port_%d", /*portid*/0);
	port_params.name = port_name;
  
///////....................................................................................////
std::cout << "rte_sched_port_config started"<<std::endl;
	port = rte_sched_port_config(&port_params);
	if (port == NULL){
		rte_exit(EXIT_FAILURE, "Unable to config Sched port\n");
	}
  std::cout << "rte_sched_port_config end"<<std::endl;

	for (subport = 0; subport < port_params.n_subports_per_port; subport ++) {
    std::cout << "rte_sched_subport_config started"<<std::endl;

	  int err = rte_sched_subport_config(port, subport, &subport_params[subport],0);
		if (err) {
			rte_exit(EXIT_FAILURE, "Unable to config Sched subport %u, err=%d\n",
					subport, err);
		}
     std::cout << "subport = "<<subport << "done"<<std::endl;

		uint32_t n_pipes_per_subport = subport_params[subport].n_pipes_per_subport_enabled;

    std::cout<<"subport_params[subport].n_pipes_per_subport_enabled="<<subport_params[subport].n_pipes_per_subport_enabled<<std::endl;
		for (pipe = 0; pipe < n_pipes_per_subport; pipe++) 
    { std::cout << "subport="<<subport<<"pipe="<<pipe<<std::endl;
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
      std::cout << "pipe=" <<pipe << "done"<<std::endl;
		}
     std::cout << "pipe end"<<std::endl;
	}  //sub-port  end.
//  std::cout << "returning s.cc-process"<<std::endl;
return CommandSuccess();
}
////////////////////////////////////////////////////////////////

ADD_MODULE(Sch, "Sch", "Multi-field classifier with a Sched")
