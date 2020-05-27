/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright(c) 2019 Intel Corporation
 */
#ifndef BESS_MODULES_GTPUENCAP_H_
#define BESS_MODULES_GTPUENCAP_H_
/*----------------------------------------------------------------------------------*/
#include "../module.h"
#include "../pb/module_msg.pb.h"
#include "../utils/gtp_common.h"
#include <rte_hash.h>
/*----------------------------------------------------------------------------------*/
/**
 * GTPU header
 */
#define GTPU_VERSION 0x01
#define GTP_PROTOCOL_TYPE_GTP 0x01
#define GTP_GPDU 0xff

/**
 * UDP header
 */
#define UDP_PORT_GTPU 2152
/*----------------------------------------------------------------------------------*/
class GtpuEncap final : public Module {
 public:
  GtpuEncap() { max_allowed_workers_ = Worker::kMaxWorkers; }

  /* Gates: (0) Default, (1) Forward */
  static const gate_idx_t kNumOGates = 2;
  static const Commands cmds;

  CommandResponse Init(const bess::pb::GtpuEncapArg &arg);
  void DeInit() override;
  CommandResponse AddSessionRecord(
      const bess::pb::GtpuEncapAddSessionRecordArg &arg);
  CommandResponse RemoveSessionRecord(
      const bess::pb::GtpuEncapRemoveSessionRecordArg &arg);
  CommandResponse ShowRecords(const bess::pb::EmptyArg &);
  CommandResponse ShowCount(const bess::pb::EmptyArg &);
  void ProcessBatch(Context *ctx, bess::PacketBatch *batch) override;
  // returns the number of active UE sessions
  std::string GetDesc() const override;

 private:
  int dp_session_create(struct session_info *entry);

  struct rte_hash *session_map = NULL;

  uint32_t s1u_sgw_ip = 0; /* S1U IP address */
  int tout_sip_attr = -1;
  int tout_dip_attr = -1;
  int tout_teid = -1;
  int tout_uport = -1;

  /**
   * Number of possible subscribers
   */
  int InitNumSubs = 0;
};
/*----------------------------------------------------------------------------------*/
#endif  // BESS_MODULES_GTPUENCAP_H_
