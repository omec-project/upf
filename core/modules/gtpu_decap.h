#ifndef BESS_MODULES_GTPUDECAP_H_
#define BESS_MODULES_GTPUDECAP_H_
/*----------------------------------------------------------------------------------*/
#include <rte_hash.h>
#include "../module.h"
#include "../pb/module_msg.pb.h"
/*----------------------------------------------------------------------------------*/
/**
 * check if nth bit is set.
 */
#define ISSET_BIT(mask, n)  (((mask) & (1LLU << (n))) ? 1 : 0)

/**
 * Default bearer session
 */
#define DEFAULT_BEARER			5
/**
 * set session id from the combination of
 * unique UE addr and Bearer id
 */
#define SESS_ID(ue_addr, br_id)		(((uint64_t)(br_id) << 32) | (0xffffffff & (ue_addr)))
				      /* [0] 28 bits | [bearer-id] 4 bits | [ue-addr] 32 bits */
/**
 * get bearer id
 */
#define UE_BEAR_ID(x)			(x>>32)
/**
 * get ue_addr
 */
#define UE_ADDR(x)			(x & 0xffffffff)
/*----------------------------------------------------------------------------------*/
class GtpuDecap final : public Module {
 public:
	GtpuDecap() {
		max_allowed_workers_ = Worker::kMaxWorkers;
	}
	
	/* Gates: (0) Default, (1) Forward */
	static const gate_idx_t kNumOGates = 2;

	CommandResponse Init(const bess::pb::GtpuDecapArg &arg);
	void ProcessBatch(Context *ctx, bess::PacketBatch *batch) override;

 private:
	struct rte_hash *session_map;
};
/*----------------------------------------------------------------------------------*/
#endif  // BESS_MODULES_GTPUDECAP_H_
