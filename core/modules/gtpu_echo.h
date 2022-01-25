/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright 2019 Intel Corporation
 */
#ifndef BESS_MODULES_GTPUECHO_H_
#define BESS_MODULES_GTPUECHO_H_
/*----------------------------------------------------------------------------------*/
#include "../module.h"
#include "../pb/module_msg.pb.h"
/*----------------------------------------------------------------------------------*/
/**
 * GTPU header
 */
#define GTPU_VERSION 0x01
#define GTP_PROTOCOL_TYPE_GTP 0x01
#define GTP_GPDU 0xff
#define GTPU_ECHO_RECOVERY 14
#define GTPU_ECHO_REQUEST 0x01
#define GTPU_ECHO_RESPONSE 0x02

/**
 * UDP header
 */
#define UDP_PORT_GTPU 2152
/*----------------------------------------------------------------------------------*/
/**
 * GTPU-Recovery Information Element
 */
typedef struct gtpu_recovery_ie_t {
  uint8_t type;
  uint8_t restart_cntr;
} gtpu_recovery_ie;
/*----------------------------------------------------------------------------------*/
class GtpuEcho final : public Module {
 public:
  GtpuEcho() { max_allowed_workers_ = Worker::kMaxWorkers; }

  /* Gates: (0) Default, (1) Forward */
  static const gate_idx_t kNumOGates = 2;

  CommandResponse Init(const bess::pb::GtpuEchoArg &arg);
  void DeInit() override;
  void ProcessBatch(Context *ctx, bess::PacketBatch *batch) override;

 private:
  bool process_echo_request(bess::Packet *p);
  uint32_t s1u_sgw_ip = 0; /* S1U IP address */
};
/*----------------------------------------------------------------------------------*/
#endif  // BESS_MODULES_GTPUECHO_H_
