/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright(c) 2019 Intel Corporation
 */
#ifndef __BESS_CONTROL_H__
#define __BESS_CONTROL_H__
/*--------------------------------------------------------------------------------*/
#include "gtp_common.h"
#include "module_msg.pb.h"
#include "service.grpc.pb.h"
#include <glog/logging.h>
#include <grpc++/channel.h>
#include <grpc++/create_channel.h>
/*--------------------------------------------------------------------------------*/
using namespace grpc;
/*--------------------------------------------------------------------------------*/
/**
 * BESSD macros
 */
#define BESSD_IP "localhost"
#define BESSD_PORT 10514u
enum src_iface_type {Access = 1, Core};

/**
 * Module decls
 */
#define ENCAPMOD "GTPUEncap"
#define ENCAPADDMETHOD "add"
#define ENCAPREMMETHOD "remove"
#define PDRLOOKUPMOD "PDRLookup"
#define PDRADDMETHOD "add"
#define PREQOSCOUNTERMOD "PreQoSCounter"
#define COUNTERADDMETHOD "add"
#define COUNTERDELMETHOD "remove"
#define MODULE_NAME_LEN 128
#define HOSTNAME_LEN 256
/*--------------------------------------------------------------------------------*/
class BessClient {
 private:
  std::unique_ptr<bess::pb::BESSControl::Stub> stub_;
  ClientContext context;
  bess::pb::CommandRequest crt;
  bess::pb::CommandResponse cre;

 public:
  BessClient(std::shared_ptr<Channel> channel)
      : stub_(bess::pb::BESSControl::NewStub(channel)), crt() {}
  void runAddCommand(const uint32_t teid, const uint32_t eteid,
                     const uint32_t ueaddr, const uint32_t enode_ip,
                     const char *modname) {
    bess::pb::GtpuEncapAddSessionRecordArg *geasra =
        new bess::pb::GtpuEncapAddSessionRecordArg();
    geasra->set_teid(teid);
    geasra->set_eteid(eteid);
    geasra->set_ueaddr(ueaddr);
    geasra->set_enodeb_ip(enode_ip);
    ::google::protobuf::Any *any = new ::google::protobuf::Any();
    any->PackFrom(*geasra);
    crt.set_name(modname);
    crt.set_cmd(ENCAPADDMETHOD);
    crt.set_allocated_arg(any);
    Status status = stub_->ModuleCommand(&context, crt, &cre);
    // Act upon its status.
    if (status.ok()) {
      VLOG(1) << "runAddCommand RPC successfully executed." << std::endl;
    } else {
      std::cout << status.error_code() << ": " << status.error_message()
                << std::endl;
      std::cout << "runAddCommand RPC failed." << std::endl;
    }

    delete geasra;

    /* `any' freed up by ModuleCommand() */
  }

  void runRemoveCommand(const uint32_t ueaddr, const char *modname) {
    bess::pb::GtpuEncapRemoveSessionRecordArg *rsra =
        new bess::pb::GtpuEncapRemoveSessionRecordArg();
    rsra->set_ueaddr(ueaddr);
    ::google::protobuf::Any *any = new ::google::protobuf::Any();
    any->PackFrom(*rsra);
    crt.set_name(modname);
    crt.set_cmd(ENCAPREMMETHOD);
    crt.set_allocated_arg(any);
    Status status = stub_->ModuleCommand(&context, crt, &cre);
    // Act upon its status.
    if (status.ok()) {
      VLOG(1) << "runRemoveCommand RPC successfully executed." << std::endl;
    } else {
      std::cout << status.error_code() << ": " << status.error_message()
                << std::endl;
      std::cout << "runRemoveCommand RPC failed." << std::endl;
    }

    delete rsra;

    /* `any' freed up by ModuleCommand() */
  }

  void runAddPDRCommand(const enum src_iface_type sit, const uint32_t enodeip,
			const uint32_t teid, const uint32_t ueaddr,
			const uint32_t inetip, const uint16_t ueport,
			const uint16_t inetport, const uint8_t protoid,
			const char *modname) {
    bess::pb::WildcardMatchCommandAddArg *wmcaa =
        new bess::pb::WildcardMatchCommandAddArg();
    wmcaa->set_gate(1);
    wmcaa->set_priority(1);

    /* SET VALUES */
    /* set src_iface value */
    bess::pb::FieldData *src_iface = wmcaa->add_values();
    /* Access = 1, Core = 2 */
    src_iface->set_value_int(sit);

    /* set tunnel_ipv4_dst */
    bess::pb::FieldData *tunnel_ipv4_dst = wmcaa->add_values();
    tunnel_ipv4_dst->set_value_int(enodeip);

    /* set teid */
    bess::pb::FieldData *_teid = wmcaa->add_values();
    _teid->set_value_int(teid);

    /* set dst ip */
    bess::pb::FieldData *dst_ip = wmcaa->add_values();
    dst_ip->set_value_int(ueaddr);

    /* set src ip */
    bess::pb::FieldData *src_ip = wmcaa->add_values();
    src_ip->set_value_int(inetip);

    /* set dst l4 port */
    bess::pb::FieldData *l4_dstport = wmcaa->add_values();
    l4_dstport->set_value_int(ueport);

    /* set src l4 port */
    bess::pb::FieldData *l4_srcport = wmcaa->add_values();
    l4_srcport->set_value_int(inetport);

    /* set proto id */
    bess::pb::FieldData *_protoid = wmcaa->add_values();
    _protoid->set_value_int(protoid);

    /* SET MASKS */
    /* set src_iface value */
    src_iface = wmcaa->add_masks();
    /* Access = 0xFF, Core = 0xFF */
    src_iface->set_value_int(0xFF);

    /* set tunnel_ipv4_dst - Setting it to 0 for the time being */
    tunnel_ipv4_dst = wmcaa->add_masks();
    tunnel_ipv4_dst->set_value_int(0x00u);

    /* set teid - Setting it to 0 for the time being */
    _teid = wmcaa->add_masks();
    _teid->set_value_int(0x00u);

    /* set dst ip */
    dst_ip = wmcaa->add_masks();
    dst_ip->set_value_int(0xFFFFFFFFu);

    /* set src ip */
    src_ip = wmcaa->add_masks();
    src_ip->set_value_int(0x0000);

    /* set dst l4 port */
    l4_dstport = wmcaa->add_masks();
    l4_dstport->set_value_int(0x0000);

    /* set src l4 port */
    l4_srcport = wmcaa->add_masks();
    l4_srcport->set_value_int(0x0000);

    /* set proto id */
    _protoid = wmcaa->add_masks();
    _protoid->set_value_int(0x00);

    /* SET VALUESV */
    /* set pdr_id, set to 0 for the time being */
    bess::pb::FieldData *_void = wmcaa->add_valuesv();
    _void->set_value_int(0x0u);

    /* set fseid, set to 0 for the time being */
    _void = wmcaa->add_valuesv();
    _void->set_value_int(0x0u);

    /* set ctr_id, set to 0 for the time being */
    _void = wmcaa->add_valuesv();
    _void->set_value_int(ueaddr);

    /* set far_id, set to 0 for the time being */
    _void = wmcaa->add_valuesv();
    _void->set_value_int(0x0u);

    /* set needs_gtpu_decap, set to 0 for the time being */
    _void = wmcaa->add_valuesv();
    _void->set_value_int(0x00);

    ::google::protobuf::Any *any = new ::google::protobuf::Any();
    any->PackFrom(*wmcaa);
    crt.set_name(modname);
    crt.set_cmd(PDRADDMETHOD);
    crt.set_allocated_arg(any);
    Status status = stub_->ModuleCommand(&context, crt, &cre);
    // Act upon its status.
    if (status.ok()) {
      VLOG(1) << "runAddPDRCommand RPC successfully executed." << std::endl;
    } else {
      std::cout << status.error_code() << ": " << status.error_message()
                << std::endl;
      std::cout << "runAddPDRCommand RPC failed." << std::endl;
    }

    delete wmcaa;

    /* `any' freed up by ModuleCommand() */
  }

  void runAddCounterCommand(const uint32_t ctr_id,
			    const char *modname) {
    bess::pb::CounterAddArg *caa =
        new bess::pb::CounterAddArg();
    caa->set_ctr_id(ctr_id);
    ::google::protobuf::Any *any = new ::google::protobuf::Any();
    any->PackFrom(*caa);
    crt.set_name(modname);
    crt.set_cmd(COUNTERADDMETHOD);
    crt.set_allocated_arg(any);
    Status status = stub_->ModuleCommand(&context, crt, &cre);
    // Act upon its status.
    if (status.ok()) {
      VLOG(1) << "runAddCommand RPC successfully executed." << std::endl;
    } else {
      std::cout << status.error_code() << ": " << status.error_message()
                << std::endl;
      std::cout << "runAddCommand RPC failed." << std::endl;
    }

    delete caa;

    /* `any' freed up by ModuleCommand() */
  }

  void runDelCounterCommand(const uint32_t ctr_id,
			    const char *modname) {
    bess::pb::CounterRemoveArg *cra =
        new bess::pb::CounterRemoveArg();
    cra->set_ctr_id(ctr_id);
    ::google::protobuf::Any *any = new ::google::protobuf::Any();
    any->PackFrom(*cra);
    crt.set_name(modname);
    crt.set_cmd(COUNTERDELMETHOD);
    crt.set_allocated_arg(any);
    Status status = stub_->ModuleCommand(&context, crt, &cre);
    // Act upon its status.
    if (status.ok()) {
      VLOG(1) << "runAddCommand RPC successfully executed." << std::endl;
    } else {
      std::cout << status.error_code() << ": " << status.error_message()
                << std::endl;
      std::cout << "runAddCommand RPC failed." << std::endl;
    }

    delete cra;

    /* `any' freed up by ModuleCommand() */
  }
};
/*--------------------------------------------------------------------------------*/
std::ostream &operator<<(std::ostream &os, const struct ip_addr &ip) {
  if (ip.iptype == IPTYPE_IPV4) {
    os << ((ip.u.ipv4_addr >> 24) & 0xFF) << "."
       << ((ip.u.ipv4_addr >> 16) & 0xFF) << "."
       << ((ip.u.ipv4_addr >> 8) & 0xFF) << "." << (ip.u.ipv4_addr & 0xFF);
  } else {
    os << "Unsupported IP type";
  }
  return os;
}
/*--------------------------------------------------------------------------------*/
#endif /* !__BESS_CONTROL_H__ */
