/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright(c) 2019 Intel Corporation
 */
#ifndef __BESS_CONTROL_H__
#define __BESS_CONTROL_H__
/*--------------------------------------------------------------------------------*/
#include <grpcpp/channel.h>
#include <grpcpp/create_channel.h>
#include <glog/logging.h>
#include "service.grpc.pb.h"
#include "module_msg.pb.h"
#include "gtp_common.h"
/*--------------------------------------------------------------------------------*/
using namespace grpc;
/*--------------------------------------------------------------------------------*/
/**
 * BESSD macros
 */
#define BESSD_IP		"localhost"
#define BESSD_PORT		10514u

/**
 * Module decls
 */
#define ENCAPMOD		"GTPUEncap"
#define ENCAPADDMETHOD		"add"
#define ENCAPREMMETHOD		"remove"
#define MODULE_NAME_LEN		128
#define HOSTNAME_LEN		256
/*--------------------------------------------------------------------------------*/
class BessClient {
private:
	std::unique_ptr<bess::pb::BESSControl::Stub> stub_;
	ClientContext context;
	bess::pb::CommandRequest crt;
	bess::pb::CommandResponse cre;
public:
	BessClient(std::shared_ptr<Channel> channel) : stub_(bess::pb::BESSControl::NewStub(channel)), crt() {}
		void runAddCommand(const uint32_t teid, const uint32_t eteid, const uint32_t ueaddr,
				   const uint32_t enode_ip, const char *modname) {
		bess::pb::GtpuEncapAddSessionRecordArg *geasra = new bess::pb::GtpuEncapAddSessionRecordArg();
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
		bess::pb::GtpuEncapRemoveSessionRecordArg *rsra = new bess::pb::GtpuEncapRemoveSessionRecordArg();
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
};
/*--------------------------------------------------------------------------------*/
std::ostream&
operator<<(std::ostream& os, const struct ip_addr& ip)
{
	if (ip.iptype == IPTYPE_IPV4) {
		os << ((ip.u.ipv4_addr >> 24) & 0xFF)
		   << "." << ((ip.u.ipv4_addr >> 16) & 0xFF)
		   << "." << ((ip.u.ipv4_addr >> 8) & 0xFF)
		   << "." << (ip.u.ipv4_addr & 0xFF);
	} else {
		os << "Unsupported IP type";
	}
	return os;
}
/*--------------------------------------------------------------------------------*/
#endif /* !__BESS_CONTROL_H__ */
