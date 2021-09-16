/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright(c) 2021 Open Networking Foundation
 */

#include "my_module.h"

#include "../packet_pool.h"

#if ENABLE_MODULE

#define MAX_SCHED_SUBPORT_PROFILES 8
#define MAX_SCHED_PIPES 4096
#define MAX_SCHED_PIPE_PROFILES 256
#define MAX_SCHED_SUBPORTS 8

enum {
  DEFAULT_GATE = 0,
  FORWARD_GATE,
};

const Commands MyModule::cmds = {};

namespace {
// void DumpSchedStats(rte_sched_port *sched) {
//   rte_sched_queue_stats queue_stats;
//   for (uint32_t queue = 0; queue < 1; ++queue) {
//     uint16_t queue_length;
//     int err =
//         rte_sched_queue_read_stats(sched, queue, &queue_stats,
//         &queue_length);
//     if (err) {
//       LOG(ERROR) << "rte_sched_queue_read_stats failed with " << err;
//       return;
//     }
//     LOG_EVERY_N(INFO, 1024)
//         << "Queue " << queue << ": current length " << queue_length
//         << ", packets " << queue_stats.n_pkts << ", packets dropped "
//         << queue_stats.n_pkts_dropped << ", bytes " << queue_stats.n_bytes
//         << ", bytes dropped " << queue_stats.n_bytes_dropped;
//   }
// }
}  // namespace

void MyModule::ProcessBatch(Context *ctx, bess::PacketBatch *batch) {
  (void)ctx;
  (void)batch;

  bess::PacketPool *pool = bess::PacketPool::GetDefaultPool(0);
  (void)pool;

  for (int i = 0; i < batch->cnt(); ++i) {
    bess::Packet *p = batch->pkts()[i];
    struct bess::Packet *copy = bess::Packet::copy(p);
    DropPacket(ctx, p);
    if (!copy) {
      LOG(ERROR) << "packet copy failed";
      continue;
    }

    // Classification.
    struct rte_mbuf *c = reinterpret_cast<struct rte_mbuf *>(copy);
    rte_sched_port_pkt_write(scheduler_, c, /*subport*/ 0, /*pipe*/ 0,
                             /*tc*/ 1, /*queue*/ 0, /*color*/ RTE_COLOR_GREEN);

    struct rte_mbuf *mbufs[] = {c};
    // Drops packets automatically when full.
    int enqueued = rte_sched_port_enqueue(scheduler_, mbufs, 1);
    // LOG_IF(WARNING, enqueued > 0) << "enqueued: " << enqueued;
    if (enqueued == 0) {
      batch->pkts()[i] = nullptr;
    }

    struct rte_mbuf *tx_mbufs[4] = {};
    int dequeued = rte_sched_port_dequeue(scheduler_, tx_mbufs, 4);
    // LOG_IF(WARNING, dequeued > 0) << "dequeued: " << dequeued;
    for (int j = 0; j < dequeued; ++j) {
      EmitPacket(ctx, reinterpret_cast<bess::Packet *>(tx_mbufs[j]),
                 DEFAULT_GATE);
    }
  }
  // DumpSchedStats(scheduler_);
}

// void MyModule::DeInit() {
//   /* do nothing */
// }

CommandResponse MyModule::Init(const bess::pb::EmptyArg &arg) {
  (void)arg;
  struct rte_sched_subport_profile_params
      subport_profile[MAX_SCHED_SUBPORT_PROFILES] = {
          {
              .tb_rate = 1250000000,
              .tb_size = 1000000,
              .tc_rate = {1250000000, 1250000000, 1250000000, 1250000000,
                          1250000000, 1250000000, 1250000000, 1250000000,
                          1250000000, 1250000000, 1250000000, 1250000000,
                          1250000000},
              .tc_period = 10,
          },
      };
  struct rte_sched_pipe_params pipe_profiles[MAX_SCHED_PIPE_PROFILES] = {
      {
          /* Profile #0 */
          .tb_rate = 1250000000,  // 305175,
          .tb_size = 1000000,
          /* .tc_rate = */
          {1250000000, 305175, 305175, 305175, 305175, 305175, 305175, 305175,
           305175, 305175, 305175, 305175, 305175},  // 305175
          .tc_period = 40,
          // #ifdef RTE_SCHED_SUBPORT_TC_OV
          .tc_ov_weight = 1,
          // #endif
          .wrr_weights = {1, 1, 1, 1},
      },
  };
  struct rte_sched_subport_params subport_params[MAX_SCHED_SUBPORTS] = {
      {
          .n_pipes_per_subport_enabled = 1,
          .qsize = {64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64},
          .pipe_profiles = pipe_profiles,
          .n_pipe_profiles = 1,
          .n_max_pipe_profiles = 1,
      },
  };
  // int app_pipe_to_profile[MAX_SCHED_SUBPORTS][MAX_SCHED_PIPES] = {};
  struct rte_sched_port_params port_params = {};
  port_params.name = "port_scheduler_0";
  port_params.socket = rte_socket_id() == LCORE_ID_ANY ? 0 : rte_socket_id();
  port_params.rate = 1250000000;  // bytes per sec
  port_params.mtu = 6 + 6 + 4 + 4 + 2 + 1500;
  port_params.frame_overhead = RTE_SCHED_FRAME_OVERHEAD_DEFAULT;
  port_params.n_subports_per_port = 1;
  port_params.n_subport_profiles = 1;
  port_params.subport_profiles = subport_profile;
  port_params.n_max_subport_profiles = MAX_SCHED_SUBPORT_PROFILES;
  port_params.n_pipes_per_subport = 1;  // MAX_SCHED_PIPES;
  scheduler_ = rte_sched_port_config(&port_params);
  if (scheduler_ == NULL) {
    return CommandFailure(EINVAL, "rte_sched_port_config failed");
  }
  for (unsigned int subport = 0; subport < port_params.n_subports_per_port;
       ++subport) {
    int err = rte_sched_subport_config(scheduler_, subport,
                                       &subport_params[subport], 0);
    if (err) {
      CommandFailure(EINVAL,
                     "Unable to config sched "
                     "subport %u, err=%d\n",
                     subport, err);
    }
    uint32_t n_pipes_per_subport =
        subport_params[subport].n_pipes_per_subport_enabled;
    for (unsigned int pipe = 0; pipe < n_pipes_per_subport; ++pipe) {
      // if (app_pipe_to_profile[subport][pipe] != -1) {
      int32_t profile = 0;  // app_pipe_to_profile[subport][pipe];
      err = rte_sched_pipe_config(scheduler_, subport, pipe, profile);
      if (err) {
        CommandFailure(EINVAL,
                       "Unable to config sched pipe %u "
                       "for profile %d, err=%d\n",
                       pipe, profile, err);
      }
      // }
    }
  }

  return CommandSuccess();
}

ADD_MODULE(MyModule, "my_module", "pass through module")

#endif
