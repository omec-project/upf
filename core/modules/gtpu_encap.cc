/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright(c) 2019 Intel Corporation
 */
/* for gtpu_encap decls */
#include "gtpu_encap.h"
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
/* for GetDesc() */
#include "utils/format.h"
#include <rte_jhash.h>
/*----------------------------------------------------------------------------------*/
using bess::utils::be16_t;
using bess::utils::be32_t;
using bess::utils::Gtpv1;
using bess::utils::Ipv4;
using bess::utils::ToIpv4Address;
using bess::utils::Udp;

enum { DEFAULT_GATE = 0, FORWARD_GATE };
/*----------------------------------------------------------------------------------*/
const Commands GtpuEncap::cmds = {
    {"add", "GtpuEncapAddSessionRecordArg",
     MODULE_CMD_FUNC(&GtpuEncap::AddSessionRecord), Command::THREAD_SAFE},
    {"remove", "GtpuEncapRemoveSessionRecordArg",
     MODULE_CMD_FUNC(&GtpuEncap::RemoveSessionRecord), Command::THREAD_SAFE},
    {"show_records", "EmptyArg", MODULE_CMD_FUNC(&GtpuEncap::ShowRecords),
     Command::THREAD_SAFE},
    {"show_count", "EmptyArg", MODULE_CMD_FUNC(&GtpuEncap::ShowCount),
     Command::THREAD_SAFE}};
/*----------------------------------------------------------------------------------*/
// Template for generating UDP packets without data
struct [[gnu::packed]] PacketTemplate {
  Ipv4 iph;
  Udp udph;
  Gtpv1 gtph;

  PacketTemplate() {
    gtph.version = GTPU_VERSION;
    gtph.pt = GTP_PROTOCOL_TYPE_GTP;
    gtph.spare = 0;
    gtph.ex = 0;
    gtph.seq = 0;
    gtph.pdn = 0;
    gtph.type = GTP_GPDU;
    gtph.length = (be16_t)0;  // to fill in
    gtph.teid = (be32_t)0;    // to fill in
    udph.src_port = (be16_t)UDP_PORT_GTPU;
    udph.dst_port = (be16_t)UDP_PORT_GTPU;
    udph.length = (be16_t)0;  // to fill in
    /* calculated by L4Checksum module in line */
    udph.checksum = 0;
    iph.version = IPVERSION;
    iph.header_length = (sizeof(Ipv4) >> 2);
    iph.type_of_service = 0;
    iph.length = (be16_t)0;  // to fill in
    iph.id = (be16_t)0x513;
    iph.fragment_offset = (be16_t)0;
    iph.ttl = 64;
    iph.protocol = IPPROTO_UDP;
    /* calculated by IPChecksum module in line */
    iph.checksum = 0;
    iph.src = (be32_t)0;  // to fill in
    iph.dst = (be32_t)0;  // to fill in
  }
};
static PacketTemplate outer_ip_template;
/*----------------------------------------------------------------------------------*/
int GtpuEncap::dp_session_create(struct session_info *entry) {
  struct session_info *data;

  /* allocate memory for session info */
  data = (struct session_info *)rte_calloc("session_info",
                                           sizeof(struct session_info), 1, 0);
  if (data == NULL) {
    std::cerr << "Failed to allocate memory for session info!" << std::endl;
    return -1;
  }

  if (rte_hash_add_key_data(session_map, &entry->sess_id, data) < 0) {
    std::cerr << "Failed to insert session info with "
              << " sess_id " << entry->sess_id << std::endl;
  }

  /* copy session info to the entry */
  data->ue_addr = entry->ue_addr;
  data->ul_s1_info = entry->ul_s1_info;
  data->dl_s1_info = entry->dl_s1_info;
  memcpy(&data->ipcan_dp_bearer_cdr, &entry->ipcan_dp_bearer_cdr,
         sizeof(struct ipcan_dp_bearer_cdr));
  data->sess_id = entry->sess_id;

  uint32_t addr = entry->ue_addr.u.ipv4_addr;
  DLOG(INFO) << "Adding entry for UE ip address: "
             << ToIpv4Address(be32_t(addr)) << std::endl;
  DLOG(INFO) << "------------------------------------------------" << std::endl;

  return 0;
}
/*----------------------------------------------------------------------------------*/
CommandResponse GtpuEncap::AddSessionRecord(
    const bess::pb::GtpuEncapAddSessionRecordArg &arg) {
  uint32_t teid = arg.teid();
  uint32_t eteid = arg.eteid();
  uint32_t ueaddr = arg.ueaddr();
  uint32_t enodeb_ip = arg.enodeb_ip();
  struct session_info sess;

  if (teid == 0)
    return CommandFailure(EINVAL, "Invalid TEID value");
  if (eteid == 0)
    return CommandFailure(EINVAL, "Invalid enodeb TEID value");
  if (ueaddr == 0)
    return CommandFailure(EINVAL, "Invalid UE address");
  if (enodeb_ip == 0)
    return CommandFailure(EINVAL, "Invalid enodeB IP address");

  DLOG(INFO) << "Teid: " << std::hex << teid
             << ", ueaddr: " << ToIpv4Address(be32_t(ueaddr))
             << ", enodeaddr: " << ToIpv4Address(be32_t(enodeb_ip))
             << std::endl;

  memset(&sess, 0, sizeof(struct session_info));

  sess.ue_addr.u.ipv4_addr = ueaddr;
  sess.ul_s1_info.sgw_teid = teid;
  sess.ul_s1_info.sgw_addr.u.ipv4_addr = s1u_sgw_ip;
  sess.dl_s1_info.enb_teid = eteid;
  sess.dl_s1_info.sgw_addr.u.ipv4_addr = s1u_sgw_ip;
  sess.ul_s1_info.enb_addr.u.ipv4_addr = enodeb_ip;
  sess.sess_id = SESS_ID(htonl(sess.ue_addr.u.ipv4_addr), DEFAULT_BEARER);

  if (dp_session_create(&sess) < 0) {
    std::cerr << "Failed to insert entry for ueaddr: "
              << ToIpv4Address(be32_t(ueaddr)) << std::endl;
    return CommandFailure(ENOMEM, "Failed to insert session record");
  }

  return CommandSuccess();
}
/*----------------------------------------------------------------------------------*/
CommandResponse GtpuEncap::RemoveSessionRecord(
    const bess::pb::GtpuEncapRemoveSessionRecordArg &arg) {
  uint32_t ip = arg.ueaddr();
  uint64_t key;
  struct session_info *data;

  if (ip == 0)
    return CommandFailure(EINVAL, "Invalid UE address");

  DLOG(INFO) << "IP Address: " << ToIpv4Address(be32_t(ip)) << std::endl;

  key = SESS_ID(htonl(ip), DEFAULT_BEARER);

  if (rte_hash_lookup_data(session_map, &key, (void **)&data) < 0)
    return CommandFailure(EINVAL, "The given address does not exist");

  /* free session_info */
  rte_free(data);

  /* now remove the record */
  if (rte_hash_del_key(session_map, &key) < 0)
    return CommandFailure(EINVAL, "Failed to remove UE address");

  return CommandSuccess();
}
/*----------------------------------------------------------------------------------*/
CommandResponse GtpuEncap::ShowRecords(const bess::pb::EmptyArg &) {
  std::cerr << "Showing records now" << std::endl;

  uint32_t next = 0;
  ;
  uint64_t *key;
  void *_data;
  int rc;
  do {
    rc = rte_hash_iterate(session_map, (const void **)&key, &_data, &next);
    if (rc >= 0) {
      uint32_t ip = UE_ADDR(*key);
      struct session_info *data = (struct session_info *)_data;
      std::cerr << "IP Address: " << ToIpv4Address(be32_t(ip))
                << ", Data: " << data << std::endl;
    }
  } while (rc >= 0);

  return CommandSuccess();
}
/*----------------------------------------------------------------------------------*/
CommandResponse GtpuEncap::ShowCount(const bess::pb::EmptyArg &) {
  bess::pb::GtpuEncapArg arg;
  arg.set_s1u_sgw_ip(0);
  arg.set_num_subscribers(rte_hash_count(session_map));
  DLOG(INFO) << "# of records: " << rte_hash_count(session_map) << std::endl;
  return CommandSuccess(arg);
}
/*----------------------------------------------------------------------------------*/
void GtpuEncap::ProcessBatch(Context *ctx, bess::PacketBatch *batch) {
  int cnt = batch->cnt();

  for (int i = 0; i < cnt; i++) {
    bess::Packet *p = batch->pkts()[i];

    /* check attributes' values now */
    uint32_t at_tout_sip;
    bess::metadata::mt_offset_t off = attr_offset(tout_sip_attr);
    at_tout_sip = get_attr_with_offset<uint32_t>(off, p);

    uint32_t at_tout_dip;
    off = attr_offset(tout_dip_attr);
    at_tout_dip = get_attr_with_offset<uint32_t>(off, p);

    uint32_t at_tout_teid;
    off = attr_offset(tout_teid);
    at_tout_teid = get_attr_with_offset<uint32_t>(off, p);

    uint16_t at_tout_uport;
    off = attr_offset(tout_uport);
    at_tout_uport = get_attr_with_offset<uint16_t>(off, p);

#if DEBUG
    /* checking values now */
    std::cerr << "Tunnel out sip: " << at_tout_sip << ", real: " << data[i]->ul_s1_info.sgw_addr.u.ipv4_addr << std::endl;
    std::cerr << "Tunnel out dip: " << (at_tout_dip) << ", real: " << data[i]->ul_s1_info.enb_addr.u.ipv4_addr << std::endl;
    std::cerr << "Tunnel out teid: " << (at_tout_teid) << ", real: " << data[i]->dl_s1_info.enb_teid << std::endl;
    std::cerr << "Tunnel out udp port: " << at_tout_uport << ", real: " << UDP_PORT_GTPU << std::endl;
#endif

    /* assuming that this module comes right after EthernetDecap */
    /* pkt_len can be used as the length of IP datagram */
    uint16_t pkt_len = p->total_len();
    Ipv4 *iph = p->head_data<Ipv4 *>();

    /* pre-allocate space for encaped header(s) */
    char *new_p = static_cast<char *>(
        p->prepend(sizeof(Udp) + sizeof(Gtpv1) + sizeof(Ipv4)));
    if (new_p == NULL) {
      /* failed to prepend header space for encaped packet */
      EmitPacket(ctx, p, DEFAULT_GATE);
      DLOG(INFO) << "prepend() failed!" << std::endl;
      continue;
    }

    /* setting GTPU pointer */
    Gtpv1 *gtph = (Gtpv1 *)(new_p + sizeof(Ipv4) + sizeof(Udp));

    /* copying template content */
    bess::utils::Copy(new_p, &outer_ip_template, sizeof(outer_ip_template));

    /* setting gtpu header */
    gtph->length = (be16_t)(pkt_len);
    gtph->teid = (be32_t)(at_tout_teid);

    /* setting outer UDP header */
    Udp *udph = (Udp *)(new_p + sizeof(Ipv4));
    udph->length = (be16_t)(pkt_len + sizeof(Gtpv1) + sizeof(Udp));
    udph->src_port = udph->dst_port = (be16_t)(at_tout_uport);

    /* setting outer IP header */
    iph = (Ipv4 *)(new_p);
    iph->length =
        (be16_t)(pkt_len + sizeof(Gtpv1) + sizeof(Udp) + sizeof(Ipv4));
    iph->src = (be32_t)(at_tout_sip);
    iph->dst = (be32_t)(at_tout_dip);
    EmitPacket(ctx, p, FORWARD_GATE);
  }
}
/*----------------------------------------------------------------------------------*/
void GtpuEncap::DeInit() {
  uint32_t next = 0;
  uint64_t *key;
  void *_data;
  int rc;
  do {
    rc = rte_hash_iterate(session_map, (const void **)&key, &_data, &next);
    if (rc >= 0) {
      struct session_info *data = (struct session_info *)_data;
      /* now remove the record */
      if (rte_hash_del_key(session_map, key) < 0) {
        uint32_t ip = UE_ADDR(*key);
        std::cerr << "Failed to remove record with UE address: "
                  << ToIpv4Address(be32_t(ip)) << std::endl;
      }
      rte_free(data);
      /* resetting back to NULL */
      next = 0;
    }
  } while (rc >= 0);

  /* finally free the hash table */
  rte_hash_free(session_map);
  session_map = NULL;
}
/*----------------------------------------------------------------------------------*/
CommandResponse GtpuEncap::Init(const bess::pb::GtpuEncapArg &arg) {
  s1u_sgw_ip = arg.s1u_sgw_ip();

  if (s1u_sgw_ip == 0)
    return CommandFailure(EINVAL, "Invalid S1U SGW IP address!");

  InitNumSubs = arg.num_subscribers();
  if (InitNumSubs == 0)
    return CommandFailure(EINVAL, "Invalid number of subscribers!");

  std::string hashtable_name = "session_map" + this->name();
  std::cerr << "Creating rte_hash: " << hashtable_name << std::endl;

  struct rte_hash_parameters session_map_params = {
      .name = hashtable_name.c_str(),
      .entries = (unsigned int)InitNumSubs,
      .reserved = 0,
      .key_len = sizeof(uint64_t),
      .hash_func = rte_jhash,
      .hash_func_init_val = 0,
      .socket_id = (int)rte_socket_id(),
      .extra_flag = RTE_HASH_EXTRA_FLAGS_RW_CONCURRENCY};

  session_map = rte_hash_create(&session_map_params);
  if (session_map == NULL)
    return CommandFailure(ENOMEM, "Unable to create rte_hash table: %s\n",
                          "session_map");

  using AccessMode = bess::metadata::Attribute::AccessMode;
  tout_sip_attr = AddMetadataAttr("tunnel_out_src_ip4addr", sizeof(uint32_t), AccessMode::kRead);
  std::cerr << "tout_sip_attr: " << tout_sip_attr << std::endl;
  tout_dip_attr = AddMetadataAttr("tunnel_out_dst_ip4addr", sizeof(uint32_t), AccessMode::kRead);
  std::cerr << "tout_dip_attr: " << tout_dip_attr << std::endl;
  tout_teid = AddMetadataAttr("tunnel_out_teid", sizeof(uint32_t), AccessMode::kRead);
  std::cerr << "tout_teid: " << tout_teid << std::endl;
  tout_uport = AddMetadataAttr("tunnel_out_udp_port", sizeof(uint16_t), AccessMode::kRead);
  std::cerr << "tout_uport: " << tout_uport << std::endl;

  return CommandSuccess();
}
/*----------------------------------------------------------------------------------*/
std::string GtpuEncap::GetDesc() const {
  return bess::utils::Format("%zu sessions",
                             (size_t)rte_hash_count(session_map));
}
/*----------------------------------------------------------------------------------*/
ADD_MODULE(GtpuEncap, "gtpu_encap", "first version of gtpu encap module")
