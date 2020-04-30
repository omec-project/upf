/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright(c) 2019 Intel Corporation
 */
#ifndef BESS_MODULES_GTPUPARSER_H_
#define BESS_MODULES_GTPUPARSER_H_
/*----------------------------------------------------------------------------------*/
#include "../module.h"
/* for endian types */
#include "utils/endian.h"
using bess::utils::be16_t;
using bess::utils::be32_t;
/*----------------------------------------------------------------------------------*/
/**
 * EPC Metadata
 */
typedef struct EpcMetadata
{
	be16_t l4_sport;
	be16_t l4_dport;
	be16_t inner_l4_sport;
	be16_t inner_l4_dport;
	be32_t teid;
} EpcMetadata;
/*----------------------------------------------------------------------------------*/
class GtpuParser final : public Module {
 public:
	GtpuParser() { max_allowed_workers_ = Worker::kMaxWorkers; }

	/* Gates: (0) Default, (1) Forward */
	static const gate_idx_t kNumOGates = 2;
	CommandResponse Init(const bess::pb::EmptyArg &);
	void ProcessBatch(Context *ctx, bess::PacketBatch *batch) override;
};
/*----------------------------------------------------------------------------------*/
#endif // BESS_MODULES_GTPUPARSER_H_
