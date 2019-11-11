/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright(c) 2019 Intel Corporation
 */
/* for gtpu_decap decls */
#include "gtpu_decap.h"
/* for rte_zmalloc() */
#include <rte_malloc.h>
/* for IPVERSION */
#include <netinet/ip.h>
/* for be32_t */
#include "utils/endian.h"
/* for ToIpv4Address() */
#include "utils/ip.h"
/* for udp header */
#include "utils/udp.h"
/* for gtp header */
#include "utils/gtp.h"
#include <rte_jhash.h>
/*----------------------------------------------------------------------------------*/
using bess::utils::Ipv4;
using bess::utils::Udp;
using bess::utils::Gtpv1;
using bess::utils::be32_t;
using bess::utils::ToIpv4Address;

enum {DEFAULT_GATE = 0, FORWARD_GATE};
/*----------------------------------------------------------------------------------*/
//#define USE_IN_FILE_VERSION		1
#if USE_IN_FILE_VERSION
size_t
GetGtpuHeaderSize(uint8_t *pktptr)
{
	Gtpv1 *gtph= (Gtpv1 *)pktptr;
	size_t len = sizeof(Gtpv1);

	if (gtph->seq)
		len += 2;
	if (gtph->pdn)
		len += 1;
	if (gtph->ex) {
		len += 1;
		/* Probe till the last extension header */
		/* calculate total len of gtp header (with options) */
		while (pktptr[len - 1])
			len += (pktptr[len] << 2);
	}

	return len;
}
#endif
/*----------------------------------------------------------------------------------*/
void
GtpuDecap::ProcessBatch(Context *ctx, bess::PacketBatch *batch)
{
	int cnt = batch->cnt();
	int hits = 0;
	uint64_t key[bess::PacketBatch::kMaxBurst];
	void *key_ptr[bess::PacketBatch::kMaxBurst];
	struct session_info *data[bess::PacketBatch::kMaxBurst];
	uint64_t hit_mask = 0ULL;

	for (int i = 0; i < cnt; i++) {
		bess::Packet *p = batch->pkts()[i];
		/* assuming that this module comes right after EthernetTrim */
		/* pkt_len can be used as the length of IP datagram */
		/* Trim iph->ihl<<2 + sizeof(Udp) + size of Gtpv1 header */
		Ipv4 *iph = p->head_data<Ipv4 *>();
#if USE_IN_FILE_VERSION
		batch->pkts()[i]->adj((iph->header_length<<2) + sizeof(Udp) +
				      GetGtpuHeaderSize((uint8_t *)iph +
							(iph->header_length<<2) + sizeof(Udp)));
#else
		Gtpv1 *gtph = (Gtpv1 *)((uint8_t *)iph + (iph->header_length<<2) + sizeof(Udp));
		batch->pkts()[i]->adj((iph->header_length<<2) +
				      sizeof(Udp) +
				      gtph->header_length());
#endif

		iph = p->head_data<Ipv4 *>();
		be32_t daddr = iph->dst;
		be32_t saddr = iph->src;
		DLOG(INFO) << "ip->saddr: " << ToIpv4Address(saddr)
			   << ", ip->daddr: " << ToIpv4Address(daddr)
			   << std::endl;
		key[i] = SESS_ID(saddr.raw_value(), DEFAULT_BEARER);
		key_ptr[i] = &key[i];
	}

	if ((hits = rte_hash_lookup_bulk_data(session_map,
					      (const void **)&key_ptr,
					      cnt,
					      &hit_mask,
					      (void **)data)) <= 0) {
		DLOG(INFO) << "Failed to look-up" << std::endl;
		/* Since default module is sink, the packets go right in the dump */
		/* RunNextModule() sends batch to DEFAULT GATE */
		RunNextModule(ctx, batch);
		return;
	}

	for (int i = 0; i < cnt; i++) {
		bess::Packet *p = batch->pkts()[i];

		if (!ISSET_BIT(hit_mask, i)) {
			EmitPacket(ctx, p, DEFAULT_GATE);
			std::cerr << "Fetch failed for ip->daddr: "
				  << ToIpv4Address(be32_t(UE_ADDR(key[i])))
				  << std::endl;
			continue;
		}
		EmitPacket(ctx, p, FORWARD_GATE);
	}

	DLOG(INFO) << "rte_hash_lookup_bulk_data output: (cnts: "
		   << cnt << ", hits: " << hits << ", hit_mask: " << hit_mask
		   << ")" << std::endl;
}
/*----------------------------------------------------------------------------------*/
CommandResponse
GtpuDecap::Init(const bess::pb::GtpuDecapArg &arg) {

	std::string ename = arg.ename();

	if (ename.c_str()[0] == '\0')
		return CommandFailure(EINVAL,
				      "Invalid input name!");

	std::string hashtable_name = "session_map" + ename;
	std::cerr << "Fetching rte_hash: " << hashtable_name << std::endl;

	session_map = rte_hash_find_existing(hashtable_name.c_str());
	if (session_map == NULL)
		return CommandFailure(ENOMEM,
				      "Unable to find rte_hash table: %s\n",
				      "session_map");
	return CommandSuccess();
}
/*----------------------------------------------------------------------------------*/
ADD_MODULE(GtpuDecap, "gtpu_decap", "first version of gtpu decap module")
