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
/* for IPVERSION */
#include <netinet/ip.h>
/* gtpu sim util funcs */
//#include "gtpu_encap_util.h"
/* for be32_t */
#include "../utils/endian.h"
/* for ToIpv4Address() */
#include "utils/ip.h"
/*----------------------------------------------------------------------------------*/
enum {DEFAULT_GATE = 0, FORWARD_GATE};
/*----------------------------------------------------------------------------------*/
const Commands GtpuEncap::cmds = {
	{"add", "GtpuEncapAddSessionRecordArg",
	 MODULE_CMD_FUNC(&GtpuEncap::AddSessionRecord),
	 Command::THREAD_UNSAFE},
	{"remove", "GtpuEncapRemoveSessionRecordArg",
	 MODULE_CMD_FUNC(&GtpuEncap::RemoveSessionRecord),
	 Command::THREAD_UNSAFE},
	{"show_records", "EmptyArg",
	 MODULE_CMD_FUNC(&GtpuEncap::ShowRecords),
	 Command::THREAD_UNSAFE}	
};
/*----------------------------------------------------------------------------------*/
// Template for generating UDP packets without data
struct[[gnu::packed]] PacketTemplate {
	struct ipv4_hdr iph;
	struct udp_hdr udph;
	struct gtpu_hdr gtph;

	PacketTemplate() {
		gtph.version = GTPU_VERSION;
		gtph.pt = GTP_PROTOCOL_TYPE_GTP;
		gtph.spare = 0;
		gtph.ex = 0;
		gtph.seq = 0;
		gtph.pdn = 0;
		gtph.type = GTP_GPDU;
		gtph.length = 0;	// to fill in
		gtph.teid = 0;		// to fill in
		udph.src_port = htons(UDP_PORT_GTPU);
		udph.dst_port = htons(UDP_PORT_GTPU);
		udph.dgram_len = 0;	// to fill in
		/* calculated by L4Checksum module in line */
		udph.dgram_cksum = 0;
		iph.version_ihl = IPVERSION << 4 | sizeof(struct ipv4_hdr) / IPV4_IHL_MULTIPLIER;
		iph.type_of_service = 0;
		iph.total_length = 0;	// to fill in
		iph.packet_id = 0x513;
		iph.fragment_offset = 0;
		iph.time_to_live = 64;
		iph.next_proto_id = IPPROTO_UDP;
		/* calculated by IPChecksum module in line */
		iph.hdr_checksum = 0;
		iph.src_addr = 0;	// to fill in
		iph.dst_addr = 0;	// to fill in
	}
};
static PacketTemplate outer_ip_template;
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
CommandResponse
GtpuEncap::AddSessionRecord(const bess::pb::GtpuEncapAddSessionRecordArg &arg)
{
	using bess::utils::be32_t;
	using bess::utils::ToIpv4Address;

	uint32_t teid = arg.teid();
	uint32_t ueaddr = arg.ueaddr();
	uint32_t enodeb_ip = arg.enodeb_ip();
	struct session_info sess;

	if (teid == 0)
		return CommandFailure(EINVAL, "Invalid TEID value");
	if (ueaddr == 0)
		return CommandFailure(EINVAL, "Invalid UE address");
	if (enodeb_ip == 0)
		return CommandFailure(EINVAL, "Invalid enodeB IP address");
	
	DLOG(INFO) << "Teid: " << std::hex << teid << ", ueaddr: "
		   << ToIpv4Address(be32_t(ueaddr)) << ", enodeaddr: "
		   << ToIpv4Address(be32_t(enodeb_ip)) << std::endl;

	memset(&sess, 0, sizeof(struct session_info));

	sess.ue_addr.u.ipv4_addr = ueaddr;
	sess.ul_s1_info.sgw_teid = teid;
	sess.ul_s1_info.sgw_addr.u.ipv4_addr = s1u_sgw_ip;
	sess.dl_s1_info.sgw_addr.u.ipv4_addr = s1u_sgw_ip;
	sess.ul_s1_info.enb_addr.u.ipv4_addr = enodeb_ip;

	sess.sess_id = SESS_ID(/*htonl*/(sess.ue_addr.u.ipv4_addr), DEFAULT_BEARER);

	if (dp_session_create(&sess) < 0) {
		std::cerr << "Failed to insert entry for ueaddr: "
			  << ToIpv4Address(be32_t(ueaddr)) << std::endl;
		return CommandFailure(ENOMEM, "Failed to insert session record");
	}
	return CommandSuccess();
}
/*----------------------------------------------------------------------------------*/
CommandResponse
GtpuEncap::RemoveSessionRecord(const bess::pb::GtpuEncapRemoveSessionRecordArg &arg)
{
	using bess::utils::be32_t;
	using bess::utils::ToIpv4Address;

	uint32_t ip = arg.ueaddr();
	uint64_t key;

	if (ip == 0)
		return CommandFailure(EINVAL, "Invalid UE address");

	DLOG(INFO) << "IP Address: " << ToIpv4Address(be32_t(ip)) << std::endl;

	key = SESS_ID(ip, DEFAULT_BEARER);
	
	/* retrieve session info */
	std::pair<uint64_t, uint64_t> *value = session_map.Find(/*htonl*/(key));
	struct session_info *data = (value == NULL) ? (struct session_info *)value :
		(struct session_info *)value->second;

	if (data == NULL)
		return CommandFailure(EINVAL, "The given address does not exist");

	/* free session_info */
	rte_free(data);

	/* now remove the record */
	if (session_map.Remove(key) == false)
		return CommandFailure(EINVAL, "Failed to remove UE address");

	return CommandSuccess();
}
/*----------------------------------------------------------------------------------*/
CommandResponse
GtpuEncap::ShowRecords(const bess::pb::EmptyArg &)
{
	using bess::utils::be32_t;
	using bess::utils::ToIpv4Address;

	std::cerr << "Showing records now" << std::endl;
	for (auto it = session_map.begin(); it != session_map.end(); it++) {
		uint64_t key = it->first;
		uint32_t ip = UE_ADDR(key);
		std::cerr << "IP Address: " << ToIpv4Address(be32_t(ip))
			  << ", Data: " << it->second << std::endl;
	}

	return CommandSuccess();
}
/*----------------------------------------------------------------------------------*/
void
GtpuEncap::ProcessBatch(Context *ctx, bess::PacketBatch *batch)
{
	using bess::utils::be32_t;
	using bess::utils::ToIpv4Address;

	int cnt = batch->cnt();
	for (int i = 0; i < cnt; i++) {
		bess::Packet *p = batch->pkts()[i];
		/* assuming that this module comes right after EthernetDecap */
		/* pkt_len can be used as the length of IP datagram */
		uint16_t pkt_len = p->total_len();
		struct ipv4_hdr *iph = p->head_data<struct ipv4_hdr *>();
		uint32_t daddr = iph->dst_addr;
		uint32_t saddr = iph->src_addr;
		DLOG(INFO) << "ip->saddr: " << ToIpv4Address(be32_t(saddr))
			   << ", ip->daddr: " << ToIpv4Address(be32_t(daddr))
			   << std::endl;

		/* retrieve session info */
		uint64_t sess_id = SESS_ID(ntohl(daddr), DEFAULT_BEARER);
		std::pair<uint64_t, uint64_t> *result = session_map.Find(sess_id);
		struct session_info *data = (result == NULL) ? (struct session_info *)result :
			(struct session_info *)result->second;

		if (data == NULL) {
			DLOG(INFO) << "Could not find teid for IP address: "
				   << ToIpv4Address(be32_t(daddr))
				   << std::endl;
			EmitPacket(ctx, p, DEFAULT_GATE);
			continue;
		}

		/* pre-allocate space for encaped header(s) */
		char *new_p = static_cast<char *>(p->prepend(sizeof(struct udp_hdr) +
							     sizeof(struct gtpu_hdr) +
							     sizeof(struct ipv4_hdr)));
		/* setting GTPU pointer */
		struct gtpu_hdr *gtph = (struct gtpu_hdr *)(new_p + sizeof(struct ipv4_hdr)
							    + sizeof(struct udp_hdr));

		/* copying template content */
		bess::utils::Copy(new_p, &outer_ip_template, sizeof(outer_ip_template));

		/* setting gtpu header */
		gtph->length = htons(pkt_len);
		gtph->teid = htonl(data->ul_s1_info.sgw_teid);

		/* setting outer UDP header */
		struct udp_hdr *udph = (struct udp_hdr *)(new_p + sizeof(struct ipv4_hdr));
		udph->dgram_len = htons(pkt_len + sizeof(struct gtpu_hdr) +
					sizeof(struct udp_hdr));

		/* setting outer IP header */
		iph = (struct ipv4_hdr *)(new_p);
		iph->total_length = htons(pkt_len + sizeof(struct gtpu_hdr) +
					  sizeof(struct udp_hdr) + sizeof(struct ipv4_hdr));
		iph->src_addr = htonl(data->ul_s1_info.sgw_addr.u.ipv4_addr);
		iph->dst_addr = htonl(data->ul_s1_info.enb_addr.u.ipv4_addr);
		EmitPacket(ctx, p, FORWARD_GATE);
	}
}
/*----------------------------------------------------------------------------------*/
void
GtpuEncap::DeInit()
{
	using bess::utils::be32_t;
	using bess::utils::ToIpv4Address;

	for (auto it = session_map.begin(); it != session_map.end(); it++) {
		uint64_t key = it->first;
		struct session_info *data = (struct session_info *)it->second;
		if (data != NULL)
			rte_free(data);
		if (session_map.Remove(key) == false) {
			uint32_t ip = UE_ADDR(key);
			std::cerr << "Failed to remove record with UE address: "
				  << ToIpv4Address(be32_t(ip)) << std::endl;
		}
	}
}
/*----------------------------------------------------------------------------------*/
CommandResponse
GtpuEncap::Init(const bess::pb::GtpuEncapArg &arg) {

	s1u_sgw_ip = arg.s1u_sgw_ip();

	if (s1u_sgw_ip == 0)
		return CommandFailure(EINVAL,
				      "Invalid S1U SGW IP address!");

	InitNumSubs = arg.num_subscribers();
	if (InitNumSubs == 0)
		return CommandFailure(EINVAL,
				      "Invalid number of subscribers!");

	session_map = bess::utils::CuckooMap<uint64_t, uint64_t>(InitNumBucket, InitNumSubs);

	return CommandSuccess();
}
/*----------------------------------------------------------------------------------*/
ADD_MODULE(GtpuEncap, "gtpu_encap", "first version of gtpu encap module")
