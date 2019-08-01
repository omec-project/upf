#ifndef BESS_MODULES_GTPUENCAP_H_
#define BESS_MODULES_GTPUENCAP_H_
/*----------------------------------------------------------------------------------*/
#include <rte_hash.h>
#include "../module.h"
#include "../pb/module_msg.pb.h"
#include "../utils/gtp_common.h"
/*----------------------------------------------------------------------------------*/
/**
 * GTPU header
 */
#define GTPU_VERSION			0x01
#define GTP_PROTOCOL_TYPE_GTP		0x01
#define GTP_GPDU			0xff

/**
 * UDP header
 */
#define UDP_PORT_GTPU			2152
/*----------------------------------------------------------------------------------*/
class GtpuEncap final : public Module {
 public:
	GtpuEncap() {
		max_allowed_workers_ = Worker::kMaxWorkers;
	}
	
	/* Gates: (0) Default, (1) Forward */
	static const gate_idx_t kNumOGates = 2;
	static const Commands cmds;

	CommandResponse Init(const bess::pb::GtpuEncapArg &arg);
	void DeInit() override;
	CommandResponse AddSessionRecord(const bess::pb::GtpuEncapAddSessionRecordArg &arg);
	CommandResponse RemoveSessionRecord(const bess::pb::GtpuEncapRemoveSessionRecordArg &arg);
	CommandResponse ShowRecords(const bess::pb::EmptyArg &);
	void ProcessBatch(Context *ctx, bess::PacketBatch *batch) override;
	
 private:
	int dp_session_create(struct session_info *entry);

	struct rte_hash *session_map;

	uint32_t s1u_sgw_ip;	/* S1U IP address */

	/**
	 * Number of possible subscribers
	 */
	int InitNumSubs;
};
/*----------------------------------------------------------------------------------*/
#endif  // BESS_MODULES_GTPUENCAP_H_
