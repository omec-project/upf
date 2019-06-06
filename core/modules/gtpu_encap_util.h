/* for gtpu_encap decls */
#include "gtpu_encap.h"
/* for rte_zmalloc() */
#include <rte_malloc.h>
/* for be32_t */
#include "../utils/endian.h"
/* for ToIpv4Address() */
#include "utils/ip.h"
/*----------------------------------------------------------------------------------*/
int
GtpuEncap::dp_session_create(struct session_info *entry)
{
	using bess::utils::be32_t;
	using bess::utils::ToIpv4Address;

	struct session_info *data;
#if 0
	struct ue_session_info *ue_data;
	uint32_t ue_sess_id, bear_id;

	ue_data = NULL;
	ue_sess_id = UE_SESS_ID(entry->sess_id);
	bear_id = UE_BEAR_ID(entry->sess_id);
#endif

	/* allocate memory for session info */
	data = (struct session_info *)rte_calloc("session_info",
						 sizeof(struct session_info),
						 1,
						 0); 
	if (data == NULL) {
		std::cerr << "Failed to allocate memory for session info!" << std::endl;
		return -1;
	}

	if (session_map.Insert(entry->sess_id, (uint64_t)data) == NULL) {
		std::cerr << "Failed to insert session info with " << " sess_id "
			  << entry->sess_id << std::endl;
	}

	/* copy session info to the entry */
	data->ue_addr = entry->ue_addr;
	data->ul_s1_info = entry->ul_s1_info;
	data->dl_s1_info = entry->dl_s1_info;
	memcpy(&data->ipcan_dp_bearer_cdr,
	       &entry->ipcan_dp_bearer_cdr,
	       sizeof(struct ipcan_dp_bearer_cdr));
	data->sess_id = entry->sess_id;

	uint32_t addr = entry->ue_addr.u.ipv4_addr;
	DLOG(INFO) << "Adding entry for UE ip address: "
		   << ToIpv4Address(be32_t(addr)) << std::endl;
	DLOG(INFO) << "------------------------------------------------" << std::endl;
#if 0
	data->num_ul_pcc_rules = 0;
	data->num_dl_pcc_rules = 0;
#endif
	return 0;
}
/*----------------------------------------------------------------------------------*/
