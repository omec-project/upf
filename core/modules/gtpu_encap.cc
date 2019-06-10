/* for gtpu_encap decls */
#include "gtpu_encap.h"
/* for rte_eth */
#include <rte_ether.h>
/* for rte ipv4 */
#include <rte_ip.h>
/* for rte udp */
#include <rte_udp.h>
/* for IPVERSION */
#include <netinet/ip.h>
/* gtpu sim util funcs */
#include "gtpu_encap_util.h"
/* for be32_t */
#include "../utils/endian.h"
/* for ToIpv4Address() */
#include "utils/ip.h"
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

	/* htonl conversion over here will be cheaper than ntohl in the DP pipeline */
	sess.sess_id = SESS_ID(htonl(sess.ue_addr.u.ipv4_addr), DEFAULT_BEARER);

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

	uint32_t key = arg.ueaddr();

	if (key == 0)
		return CommandFailure(EINVAL, "Invalid UE address");

	DLOG(INFO) << "IP Address: " << ToIpv4Address(be32_t(key)) << std::endl;

	/* retrieve session info */
	std::pair<uint64_t, uint64_t> *value = session_map.Find(key);
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
		uint32_t key = it->first;
		std::cerr << "IP Address: " << ToIpv4Address(be32_t(key))
			  << ", Data: " << it->second << std::endl;
	}

	return CommandSuccess();
}
/*----------------------------------------------------------------------------------*/
/**
 * XXX - TODO: Use Bess-based pkt classes instead of rte-based structs
 */
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
		uint64_t sess_id = SESS_ID(daddr, DEFAULT_BEARER);
		std::pair<uint64_t, uint64_t> *result = session_map.Find(sess_id);
		struct session_info *data = (result == NULL) ? (struct session_info *)result :
			(struct session_info *)result->second;

		if (data == NULL) {
			DLOG(INFO) << "Could not find teid for IP address: "
				   << ToIpv4Address(be32_t(daddr))
				   << std::endl;
			/* XXX - TODO: Open up a new gate and redirect bad traffic to Sink */
			DropPacket(ctx, p);
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
		/* calculated by L4Checksum module in line */
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
	}

	/* run next module in line */
	RunNextModule(ctx, batch);
}
/*----------------------------------------------------------------------------------*/
/**
 * XXX - TODO: Write a deinit function that cleans up all dynamically created units
 */
/*----------------------------------------------------------------------------------*/
CommandResponse
GtpuEncap::Init(const bess::pb::GtpuEncapArg &arg) {

	s1u_sgw_ip = arg.s1u_sgw_ip();

	if (s1u_sgw_ip == 0)
		return CommandFailure(EINVAL,
				      "Invalid S1U SGW IP address!");
	      
	return CommandSuccess();
}
/*----------------------------------------------------------------------------------*/
ADD_MODULE(GtpuEncap, "gtpu_encap", "first version of gtpu encap module")
