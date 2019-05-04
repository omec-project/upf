/* for inet_aton() */
#include <sys/socket.h>
#include <netinet/in.h>
#include <arpa/inet.h>
/* for rte macros */
#include <rte_config.h>
#include <rte_common.h>
/* for gtpu_encap decls */
#include "gtpu_encap.h"
/* for rte_eth */
#include <rte_ether.h>
/* for rte ipv4 */
#include <rte_ip.h>
/* for rte udp */
#include <rte_udp.h>
/* for rte_zmalloc() */
#include <rte_malloc.h>
/* for ip checksum */
#include "utils/checksum.h"
/* for IPVERSION */
#include <netinet/ip.h>
/*----------------------------------------------------------------------------------*/
/**
 * XXX - TODO: Use Bess-based pkt classes instead of rte-based structs
 */
void
GtpuEncap::ProcessBatch(Context *ctx, bess::PacketBatch *batch)
{
	int cnt = batch->cnt();
	for (int i = 0; i < cnt; i++) {
		bess::Packet *p = batch->pkts()[i];
		uint16_t pkt_len = p->total_len();
		struct ipv4_hdr *iph = p->head_data<struct ipv4_hdr *>();
		uint32_t daddr = iph->dst_addr;
		uint32_t saddr = iph->src_addr;
		DLOG(INFO) << "ip->saddr: " << (saddr & 0xFF) << "." << ((saddr >> 8) & 0xFF)
			   << "." << ((saddr >> 16) & 0xFF) << "." << ((saddr >> 24) & 0xFF)

			   << ", ip->daddr: " << (daddr & 0xFF) << "." << ((daddr >> 8) & 0xFF)
			   << "." << ((daddr >> 16) & 0xFF) << "." << ((daddr >> 24) & 0xFF)
			   << std::endl;

		/* retrieve session info */
		std::pair<uint32_t, uint64_t> *result = session_map.Find(ntohl(daddr));
		struct session_info *data = (result == NULL) ? (struct session_info *)result :
			(struct session_info *)result->second;

		if (data == NULL) {
			DLOG(INFO) << "Could not find teid for IP address: " << (daddr & 0xFF)
				   << "." << ((daddr >> 8) & 0xFF)
				   << "." << ((daddr >> 16) & 0xFF) << "."
				   << ((daddr >> 24) & 0xFF) << std::endl;
			/* XXX - TODO: Open up a new gate and redirect bad traffic to Sink */
			bess::Packet::Free(p);
			continue;
		}

		/* pre-allocate space for encaped header(s) */
		char *new_p = static_cast<char *>(p->prepend(sizeof(struct udp_hdr) +
							     sizeof(struct gtpu_hdr) +
							     sizeof(struct ipv4_hdr)));
		/* setting GTPU header */
		struct gtpu_hdr *gtph = (struct gtpu_hdr *)(new_p + sizeof(struct ipv4_hdr)
							    + sizeof(struct udp_hdr));
		gtph->version = GTPU_VERSION;
		gtph->pt = GTP_PROTOCOL_TYPE_GTP;
		gtph->spare = 0;
		gtph->ex = 0;
		gtph->seq = 0;
		gtph->pdn = 0;
		gtph->type = GTP_GPDU;
		gtph->length = htons(pkt_len);
		gtph->teid = htonl(data->ul_s1_info.sgw_teid);

		/* setting outer UDP header */
		struct udp_hdr *udph = (struct udp_hdr *)(new_p + sizeof(struct ipv4_hdr));
		udph->src_port = htons(UDP_PORT_GTPU);
		udph->dst_port = htons(UDP_PORT_GTPU);
		udph->dgram_len = htons(pkt_len + sizeof(struct gtpu_hdr) +
					sizeof(struct udp_hdr));
		udph->dgram_cksum = 0;

		/* setting outer IP header */
		iph = (struct ipv4_hdr *)(new_p);
		iph->version_ihl = IPVERSION << 4 | sizeof(struct ipv4_hdr) / IPV4_IHL_MULTIPLIER;
		iph->type_of_service = 0;
		iph->total_length = htons(pkt_len + sizeof(struct gtpu_hdr) +
					  sizeof(struct udp_hdr) + sizeof(struct ipv4_hdr));
		iph->packet_id = 0x513;
		iph->fragment_offset = 0;
		iph->time_to_live = 64;
		iph->next_proto_id = IPPROTO_UDP;
		iph->hdr_checksum = 0;
		iph->src_addr = htonl(data->ul_s1_info.sgw_addr.u.ipv4_addr);
		iph->dst_addr = htonl(data->ul_s1_info.enb_addr.u.ipv4_addr);

		/* calculate outer IP checksum with bess util func() */
		bess::utils::Ipv4 *ip = reinterpret_cast<bess::utils::Ipv4 *>(iph);
		iph->hdr_checksum = CalculateIpv4Checksum(*ip);

#ifdef DEBUG
		std::map<uint64_t, void *>::iterator it;
		for (it = umap.begin(); it != umap.end(); it++) {
			DLOG(INFO) << it->first << std::endl;
		}
#endif
	}

	/* run next module in line */
	RunNextModule(ctx, batch);
}
/*----------------------------------------------------------------------------------*/
/* Generate unique eNB teid. */
uint32_t
GtpuEncap::SimuCPEnbv4Teid(int ue_idx, int max_ue_ran, int max_enb_ran,
			   uint32_t *teid, uint32_t *enb_idx)
{
        int ran;
        int enb;
        int enb_of_ran;
        int ue_of_ran;
        uint32_t ue_teid;
        uint32_t session_idx = 0;
	
        if (max_ue_ran == 0 || max_enb_ran == 0)
                return -1; /* need to have at least one of each */
	
        ue_of_ran = ue_idx % max_ue_ran;
        ran = ue_idx / max_ue_ran;
        enb_of_ran = ue_of_ran % max_enb_ran;
        enb = ran * max_enb_ran + enb_of_ran;
	
        ue_teid = ue_of_ran + max_ue_ran * session_idx + 1;
	
        *teid = ue_teid;
        *enb_idx = enb;
	
        return 0;
}
/*----------------------------------------------------------------------------------*/
/* Generate unique teid for each create session. */
inline void
GtpuEncap::GenerateTEID(uint32_t *teid)
{
        *teid = base_s1u_spgw_gtpu_teid + s1u_spgw_gtpu_teid_offset;
        ++s1u_spgw_gtpu_teid_offset;
}
/*----------------------------------------------------------------------------------*/
int
GtpuEncap::dp_session_create(struct session_info *entry, int index)
{
	struct session_info *data/*, _new*/;
#if 0
	struct ue_session_info *ue_data;
	uint32_t ue_sess_id, bear_id;

	ue_data = NULL;
	ue_sess_id = UE_SESS_ID(entry->sess_id);
	bear_id = UE_BEAR_ID(entry->sess_id);
#endif

#ifndef DEBUG
	(void)index;
#endif
	/* allocate memory for session info */
	data = (struct session_info *)calloc(sizeof(struct session_info), 1); 
	if (data == NULL) {
		std::cerr << "Failed to allocate memory for session info!" << std::endl;
		return -1;
	}

	while (session_map.Insert(entry->ue_addr.u.ipv4_addr, (uint64_t)data) == NULL) {
		std::cerr << "Failed to insert session info with " << index << " sess_id "
			  << entry->sess_id << std::endl;
	}

	/* copy session info to the entry */
	data->ue_addr = entry->ue_addr;
	data->ul_s1_info = entry->ul_s1_info;
	data->dl_s1_info = entry->dl_s1_info;
	data->ipcan_dp_bearer_cdr = entry->ipcan_dp_bearer_cdr;
	data->sess_id = entry->sess_id;

	uint32_t addr = entry->ue_addr.u.ipv4_addr;
	DLOG(INFO) << index << ": Adding entry for UE ip address: "
		   << (addr & 0xFF) << "." << ((addr >> 8) & 0xFF)
		   << "." << ((addr >> 16) & 0xFF) << "."
		   << ((addr >> 24) & 0xFF) << std::endl;
	
	DLOG(INFO) << (((entry->sess_id >> 4) >> 24) & 0xFF) << "."
		   << (((entry->sess_id >> 4) >> 16) & 0xFF) << "."
		   << (((entry->sess_id >> 4) >> 8) & 0xFF) << "."
		   << ((entry->sess_id >> 4) & 0xFF) << std::endl;
	DLOG(INFO) << "------------------------------------------------" << std::endl;
#if 0
	data->num_ul_pcc_rules = 0;
	data->num_dl_pcc_rules = 0;
#endif
	return 0;
}
/*----------------------------------------------------------------------------------*/
/**
 * @brief create hash table.
 *
 */
int
GtpuEncap::HashCreate()
{
	uint32_t i;
	uint32_t teid, enb_teid, enb_ip_idx;
	struct in_addr addr;
	uint32_t ue_ip_start, s1u_sgw_ip, enb_ip;

	teid = enb_teid = enb_ip_idx = 0;
	
	if (inet_aton(UE_IP_START, &addr) == 0) {
		std::cerr << "Invalid UE IP start address" << std::endl;
		return -1;
	}
	ue_ip_start = ntohl(addr.s_addr);

	if (inet_aton(S1U_SGW_IP, &addr) == 0) {
		std::cerr << "Invalid S1U_SGW address" << std::endl;
		return -1;
	}
	s1u_sgw_ip = ntohl(addr.s_addr);

	if (inet_aton(ENODEB_IP_START, &addr) == 0) {
		std::cerr << "Invalid S1U_SGW address" << std::endl;
		return -1;
	}
	enb_ip = ntohl(addr.s_addr);
	
	for (i = 0; i < SUBSCRIBERS; i++) {
		struct session_info sess;
		/* reset it all to 0 */
		memset(&sess, 0, sizeof(struct session_info));
		/* generate teid for each create session */
		GenerateTEID(&teid);
		/* enodeb teid */
		SimuCPEnbv4Teid(i, NG4T_MAX_UE_RAN, NG4T_MAX_ENB_RAN, &enb_teid, &enb_ip_idx);

		sess.ue_addr.iptype = IPTYPE_IPV4;
		sess.ue_addr.u.ipv4_addr = ue_ip_start + i;
		sess.ul_s1_info.sgw_teid = teid;
		sess.ul_s1_info.sgw_addr.iptype = IPTYPE_IPV4;
		sess.ul_s1_info.sgw_addr.u.ipv4_addr = s1u_sgw_ip;
		sess.dl_s1_info.sgw_addr.iptype = IPTYPE_IPV4;
		sess.dl_s1_info.sgw_addr.u.ipv4_addr = s1u_sgw_ip;
		sess.ipcan_dp_bearer_cdr.charging_id = 10;
		sess.ipcan_dp_bearer_cdr.pdn_conn_charging_id = 10;
		sess.ul_s1_info.enb_addr.iptype = IPTYPE_IPV4;
		sess.ul_s1_info.enb_addr.u.ipv4_addr = enb_ip + enb_ip_idx;

		sess.sess_id = SESS_ID(sess.ue_addr.u.ipv4_addr, DEFAULT_BEARER);

		/* add entry to the hash table */
		if (dp_session_create(&sess, i) < 0) {
			std::cerr << "Failed to insert entry for " << i << std::endl;
			return -1;
		}
	}

        return 1;
}
/*----------------------------------------------------------------------------------*/
/**
 * XXX - TODO: Write a deinit function that cleans up all dynamically created units
 * XXX - TODO: Export fields so that attributes such as teid, ip addresses can
 *	       exported.
 */
/*----------------------------------------------------------------------------------*/
CommandResponse
GtpuEncap::Init(const bess::pb::EmptyArg &) {
	int ret;
	
	s1u_spgw_gtpu_teid_offset = 0;
	ret = HashCreate();
	if (ret < 0)
		return CommandFailure(ret,
				      "Could not create session table!");	
	return CommandSuccess();
}
/*----------------------------------------------------------------------------------*/
ADD_MODULE(GtpuEncap, "gtpu_encap", "first version of gtpu encap module")
