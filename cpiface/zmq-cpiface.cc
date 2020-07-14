/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright(c) 2019 Intel Corporation
 */

/* for bess-based GRPC funcs */
#include "bess_control.h"
/* for parsing */
#include "parser.h"
#include <ctime>
/* for stack iterator */
#include <iterator>
/* for session map*/
#include <map>
/* for stack */
#include <stack>
#include <sys/types.h>
/* for exec wait */
#include <sys/wait.h>
#include <unistd.h>
/* for libzmq */
#include <zmq.h>
/* for gettimeofday */
#include <sys/time.h>
/* for templates */
#include "template.h"
/*--------------------------------------------------------------------------------*/
/**
 * ZMQ stuff
 */
void *receiver;
void *sender;
void *reg;
void *context0;
void *context1;
void *context2;
#define ZMQ_POLL_TIMEOUT 1000 // in msecs
#define KEEPALIVE_TIMEOUT 100 // in secs

struct TeidEntry {
  uint32_t teid;
  uint32_t ctr_id;
};
/*--------------------------------------------------------------------------------*/
void sig_handler(int signo) {
  zmq_close(receiver);
  zmq_close(sender);
  zmq_ctx_destroy(context1);
  zmq_ctx_destroy(context2);

  google::protobuf::ShutdownProtobufLibrary();
}
/*--------------------------------------------------------------------------------*/
void
force_restart(int argc, char **argv)
{
  pid_t pid;
  int status;

  pid = fork();

  if (pid == -1) {
    std::cerr << "Failed to fork: " << strerror(errno) << std::endl;
    exit(EXIT_FAILURE);
  } else if (pid == 0) { // child process
    execv(argv[0], argv);
    exit(EXIT_SUCCESS);
  } else { // parent process
    if (waitpid(pid, &status, 0) > 0) {
      if (WIFEXITED(status) && !WEXITSTATUS(status))
	std::cerr << "Restart successful!" << std::endl;
      else if (WIFEXITED(status) && WEXITSTATUS(status)) {
	if (WEXITSTATUS(status) == 127) {
          // execv failed
	  std::cerr << "execv() failed\n" << std::endl;
	} else
	  std::cerr << "Program terminated normally, "
		    << "but returned a non-zero status"
		    << std::endl;
      } else
	  std::cerr << "Program didn't terminate normally"
		    << std::endl;
    } else {
      // waitpid() failed
      std::cerr << "waitpid() failed" << std::endl;
    }
    exit(EXIT_SUCCESS);
  }
}
/*--------------------------------------------------------------------------------*/
void invokeGRPCall(Args args, void *func_args, const char *modname, uint8_t funcID)
{
  BessClient b(CreateChannel(std::string(args.bessd_ip) + ":" +
			     std::to_string(args.bessd_port),
			     InsecureChannelCredentials()));
  ((b).*(b.grpc_ptr[funcID]))(func_args, modname);
}
/*--------------------------------------------------------------------------------*/
int main(int argc, char **argv) {
  GOOGLE_PROTOBUF_VERIFY_VERSION;

  /* key: SESS_ID(rbuf.sess_entry.ue_addr.u.ipv4_addr, DEFAULT_BEARER), val:
   * enb_teid) */
  std::map<uint64_t, TeidEntry> zmq_sess_map;
  std::stack<uint32_t> counter;
  // set my_dp_id to 0, SPGW-C will give me the id
  uint32_t my_dp_id = 0;
  struct timeval last_ack, current_time;
  // set it to 100 secs for the time being
  const uint32_t dp_cp_timeout_interval = KEEPALIVE_TIMEOUT;
  // 1 second zmq_poll timeout
  const uint32_t zmq_poll_timeout = ZMQ_POLL_TIMEOUT;
  Args args;

  context0 = zmq_ctx_new();
  context1 = zmq_ctx_new();
  context2 = zmq_ctx_new();
  // set args coming from command-line
  args.parse(argc, argv);

  /* initialize stack */
  for (int32_t k = args.counter_count - 1; k >= 0; k--)
    counter.push(k);

  if (context0 == NULL || context1 == NULL || context2 == NULL) {
    std::cerr << "Failed to create context(s)!: " << strerror(errno)
              << std::endl;
    return EXIT_FAILURE;
  }

  // Socket to register to CP
  reg = zmq_socket(context0, ZMQ_REQ);
  if (reg == NULL) {
    std::cerr << "Failed to create reg socket!: " << strerror(errno)
              << std::endl;
    return EXIT_FAILURE;
  }

  // connect to registration port
  if (zmq_connect(reg, ("tcp://" + std::string(args.nb_dst_ip) + ":" +
                        std::to_string(args.zmqd_nb_port))
                           .c_str()) < 0) {
    std::cerr << "Failed to connect to registration port!: " << strerror(errno)
              << std::endl;
    return EXIT_FAILURE;
  }

  VLOG(1) << "Connected to registration handle" << std::endl;

  // Build message
  if (inet_aton(args.nb_src_ip, &args.rmb.upf_comm_ip) == 0) {
    std::cerr << "Invalid address: " << args.nb_src_ip << std::endl;
    return EXIT_FAILURE;
  }
  // set S1U IP address
  if (inet_aton(args.s1u_sgw_ip, &args.rmb.s1u_ip) == 0) {
    std::cerr << "Invalid address: " << args.s1u_sgw_ip << std::endl;
    return EXIT_FAILURE;
  }
  // retrieve hostname
  if (!strcmp(args.rmb.hostname, "") &&
      gethostname(args.rmb.hostname, sizeof(args.rmb.hostname)) == -1) {
    std::cerr << "Unable to retreive hostname of DP!" << std::endl;
    return EXIT_FAILURE;
  }

  VLOG(1) << "DP hostname: " << args.rmb.hostname << std::endl;

  // send registration request
  if (zmq_send(reg, (void *)&args.rmb, sizeof(args.rmb), 0) == -1) {
    std::cerr << "Failed to send registration request to CP!" << std::endl;
    return EXIT_FAILURE;
  }
  // get response
  if (zmq_recv(reg, &args.zmqd_send_port, sizeof(args.zmqd_send_port), 0) ==
      -1) {
    std::cerr << "Failed to recv registration request from CP!" << std::endl;
    return EXIT_FAILURE;
  }

  VLOG(1) << "Received port #: " << args.zmqd_send_port
          << " from registration port." << std::endl;

  // close registration socket
  zmq_close(reg);
  zmq_ctx_destroy(context0);

  receiver = zmq_socket(context1, ZMQ_PULL);

  if (receiver == NULL) {
    std::cerr << "Failed to create receiver socket!: " << strerror(errno)
              << std::endl;
    return EXIT_FAILURE;
  }
  // Socket to recv message from
  if (zmq_bind(receiver, ("tcp://" + std::string(args.nb_src_ip) + ":" +
                          std::to_string(args.zmqd_recv_port))
                             .c_str()) != 0) {
    std::cerr << "Failed to bind to receiver ZMQ port!: " << strerror(errno)
              << std::endl;
    return EXIT_FAILURE;
  }
  //  Socket to send messages to
  sender = zmq_socket(context2, ZMQ_PUSH);
  if (zmq_connect(sender, ("tcp://" + std::string(args.nb_dst_ip) + ":" +
                           std::to_string(args.zmqd_send_port))
                              .c_str()) < 0) {
    std::cerr << "Failed to connect to sender!: " << strerror(errno)
              << std::endl;
    return EXIT_FAILURE;
  }

  VLOG(1) << "Connected to CP." << std::endl;

  // register a signal handler
  if (signal(SIGTERM, sig_handler) == SIG_ERR) {
    std::cerr << "Unable to register signal handler!" << std::endl;
    return EXIT_FAILURE;
  }

  gettimeofday(&last_ack, NULL);

  struct resp_msgbuf keepalive;
  keepalive.mtype = DPN_KEEPALIVE_REQ;
  keepalive.op_id = 1;            // for now always 1...
  keepalive.sess_id = 0;          // node specific message
  keepalive.dp_id.id = my_dp_id;  // DP is not aware about its id...
  strcpy(keepalive.dp_id.name, args.rmb.hostname);

  //  Process messages from either socket
  while (true) {
    zmq_pollitem_t items[] = {
        {receiver, 0, ZMQ_POLLIN, 0},
    };

    if (zmq_poll((zmq_pollitem_t *)items, 1, zmq_poll_timeout) < 0) {
      std::cerr << "ZMQ poll failed!: " << strerror(errno);
      if (errno != EINTR) {
        std::cerr << std::endl;
        return EXIT_FAILURE;
      } else {
        std::cerr << "Retrying..." << std::endl;
        continue;
      }
    }
    if (items[0].revents & ZMQ_POLLIN) {
      // as long as we get packets from control path we are good
      gettimeofday(&last_ack, NULL);
      bool send_resp = true;
      struct msgbuf rbuf;
      struct resp_msgbuf resp;
      int size = zmq_recv(receiver, &rbuf, sizeof(rbuf), 0);
      if (size == -1) {
        std::cerr << "Error in zmq reception: " << strerror(errno) << std::endl;
        break;
      }
      long mtype = rbuf.mtype;
      uint32_t enb_teid = 0;
      uint32_t curr_ctr = 0;
      TeidEntry te;
      memset(&resp, 0, sizeof(struct resp_msgbuf));
      switch (mtype) {
        case MSG_SESS_CRE:
          VLOG(1) << "Got a session create request, ";
          VLOG(1) << "UEADDR: " << rbuf.sess_entry.ue_addr
                  << ", ENODEADDR: " << rbuf.sess_entry.ul_s1_info.enb_addr
                  << ", sgw_teid: " << (rbuf.sess_entry.ul_s1_info.sgw_teid)
                  << ", enb_teid: "
                  << ntohl(rbuf.sess_entry.dl_s1_info.enb_teid) << " ("
                  << ntohl(rbuf.sess_entry.dl_s1_info.enb_teid) << ")"
                  << std::endl;
          resp.op_id = rbuf.sess_entry.op_id;
          // SPGW-C returns the DP ID
          my_dp_id = rbuf.dp_id.id;
          resp.dp_id.id = rbuf.dp_id.id;
          resp.mtype = DPN_RESPONSE;
          te.teid = 0;
          te.ctr_id = counter.top();
          zmq_sess_map[SESS_ID(rbuf.sess_entry.ue_addr.u.ipv4_addr,
                               DEFAULT_BEARER)] = te;
          VLOG(1) << "Assigning sess with IP addr: "
                  << rbuf.sess_entry.ue_addr.u.ipv4_addr
                  << " counter: " << te.ctr_id << std::endl;
          resp.sess_id = rbuf.sess_entry.sess_id;
          /* ctr_id is used up */
          counter.pop();
          break;
        case MSG_SESS_MOD:
          VLOG(1) << "Got a session modify request, ";
          VLOG(1) << "UEADDR: " << rbuf.sess_entry.ue_addr
                  << ", ENODEADDR: " << rbuf.sess_entry.ul_s1_info.enb_addr
                  << ", sgw_teid: " << (rbuf.sess_entry.ul_s1_info.sgw_teid)
                  << ", enb_teid: "
                  << ntohl(rbuf.sess_entry.dl_s1_info.enb_teid) << " ("
                  << ntohl(rbuf.sess_entry.dl_s1_info.enb_teid) << ")"
                  << std::endl;
          resp.op_id = rbuf.sess_entry.op_id;
          resp.dp_id.id = rbuf.dp_id.id;
          resp.mtype = DPN_RESPONSE;
          resp.sess_id = rbuf.sess_entry.sess_id;
          if (zmq_sess_map.find(SESS_ID(rbuf.sess_entry.ue_addr.u.ipv4_addr,
                                        DEFAULT_BEARER)) ==
              zmq_sess_map.end()) {
            std::cerr << "No record found!" << std::endl;
            break;
          } else {
            curr_ctr = zmq_sess_map[SESS_ID(rbuf.sess_entry.ue_addr.u.ipv4_addr,
                                            DEFAULT_BEARER)]
                           .ctr_id;
            te.teid = rbuf.sess_entry.dl_s1_info.enb_teid;
            te.ctr_id = curr_ctr;
            zmq_sess_map[SESS_ID(rbuf.sess_entry.ue_addr.u.ipv4_addr,
                                 DEFAULT_BEARER)] = te;
            VLOG(1) << "Assigning sess with IP addr: "
                    << rbuf.sess_entry.ue_addr.u.ipv4_addr
                    << " and teid: " << te.teid << " counter: " << te.ctr_id
                    << std::endl;
          }
	  pdrD.saddr = rbuf.sess_entry.ue_addr.u.ipv4_addr; /* ueaddr ip */
	  pdrD.fseid = rbuf.sess_entry.dl_s1_info.enb_teid; /* fseid */
	  pdrD.ctr_id = curr_ctr;                           /* ctr_id */
	  // Add PDR (DOWNLINK)
	  invokeGRPCall(args, &pdrD, args.pdrlookup, GRPC_PDR_ADD);

	  pdrU.daddr = rbuf.sess_entry.ue_addr.u.ipv4_addr; /* ueaddr ip */
	  pdrU.fseid = rbuf.sess_entry.dl_s1_info.enb_teid; /* fseid */
	  pdrU.ctr_id = curr_ctr;                           /* ctr_id */
	  // Add PDR (UPLINK)
	  invokeGRPCall(args, &pdrU, args.pdrlookup, GRPC_PDR_ADD);

	  farD.fseid = rbuf.sess_entry.dl_s1_info.enb_teid; /* fseid */
	  farD.tun_src_ip =
		  ntohl((uint32_t)(inet_addr(args.s1u_sgw_ip))); /* n3 addr */
	  farD.tun_dst_ip = rbuf.sess_entry.ul_s1_info.enb_addr.u
		  .ipv4_addr;                /* enb addr */
	  farD.teid = rbuf.sess_entry.dl_s1_info.enb_teid;/* enb_teid */
	  // Add FAR (DOWNLINK)
	  invokeGRPCall(args, &farD, args.farlookup, GRPC_FAR_ADD);

	  farU.fseid = rbuf.sess_entry.dl_s1_info.enb_teid; /* fseid */
	  // Add FAR (UPLINK)
	  invokeGRPCall(args, &farU, args.farlookup, GRPC_FAR_ADD);

	  // Add PreQoS Counter
	  invokeGRPCall(
		args,
		(&curr_ctr),
		(("pre" + std::string(args.qoscounter)).c_str()),
		GRPC_CTR_ADD);

	  // Add PostQoS Counter
	  invokeGRPCall(
		args,
                (&curr_ctr),
                (("postUL" + std::string(args.qoscounter)).c_str()),
		GRPC_CTR_ADD);

	  // Add PostQoS Counter
	  invokeGRPCall(
		args,
                (&curr_ctr),
                (("postDL" + std::string(args.qoscounter)).c_str()),
		GRPC_CTR_ADD);
          break;
        case MSG_SESS_DEL:
          VLOG(1) << "Got a session delete request" << std::endl;
          VLOG(1) << "UEADDR: " << rbuf.sess_entry.ue_addr
                  << ", ENODEADDR: " << rbuf.sess_entry.ul_s1_info.enb_addr
                  << ", sgw_teid: " << (rbuf.sess_entry.ul_s1_info.sgw_teid)
                  << ", enb_teid: "
                  << ntohl(rbuf.sess_entry.dl_s1_info.enb_teid) << " ("
                  << ntohl(rbuf.sess_entry.dl_s1_info.enb_teid) << ")"
                  << std::endl;
          resp.op_id = rbuf.sess_entry.op_id;
          resp.dp_id.id = rbuf.dp_id.id;
          resp.mtype = DPN_RESPONSE;
          resp.sess_id = rbuf.sess_entry.sess_id;
          /* why is the ue ip address stored in reverse endian order just in
           * delete message? */
          if (zmq_sess_map.find(SESS_ID((rbuf.sess_entry.ue_addr.u.ipv4_addr),
                                        DEFAULT_BEARER)) ==
              zmq_sess_map.end()) {
            std::cerr << "No record found!" << std::endl;
            break;
          } else {
            enb_teid =
                zmq_sess_map[SESS_ID((rbuf.sess_entry.ue_addr.u.ipv4_addr),
                                     DEFAULT_BEARER)]
                    .teid;
            curr_ctr =
                zmq_sess_map[SESS_ID((rbuf.sess_entry.ue_addr.u.ipv4_addr),
                                     DEFAULT_BEARER)]
                    .ctr_id;
            VLOG(1) << "Assigning sess with IP addr: "
                    << (rbuf.sess_entry.ue_addr.u.ipv4_addr)
                    << " and teid: " << enb_teid << " counter: " << curr_ctr
                    << std::endl;
          }
          {
            std::map<std::uint64_t, TeidEntry>::iterator it = zmq_sess_map.find(
                SESS_ID((rbuf.sess_entry.ue_addr.u.ipv4_addr), DEFAULT_BEARER));
            zmq_sess_map.erase(it);
          }

	  pdrD.saddr = (rbuf.sess_entry.ue_addr.u.ipv4_addr); /* ueaddr ip */
	  // Delete PDR (DOWNLINK)
	  invokeGRPCall(args, &pdrD, args.pdrlookup, GRPC_PDR_DEL);

	  pdrU.daddr = (rbuf.sess_entry.ue_addr.u.ipv4_addr); /* ueaddr ip */
	  // Delete PDR (UPLINK)
	  invokeGRPCall(args, &pdrU, args.pdrlookup, GRPC_PDR_DEL);

	  // Del FAR (DOWNLINK)
	  FARArgs fa;
	  fa.far_id = 1;
	  fa.fseid = enb_teid;
	  invokeGRPCall(args, &fa, args.farlookup, GRPC_FAR_DEL);

	  // Del FAR (UPLINK)
	  fa.far_id = 0;
	  fa.fseid = enb_teid;
	  invokeGRPCall(args, &fa, args.farlookup, GRPC_FAR_DEL);

	  // Delete PreQoS Counter
	  invokeGRPCall(
			args,
			(&curr_ctr),
			(("pre" + std::string(args.qoscounter)).c_str()),
			GRPC_CTR_DEL);

	  // Delete PostQoS Counter
	  invokeGRPCall(
			args,
			(&curr_ctr),
			(("postUL" + std::string(args.qoscounter)).c_str()),
			GRPC_CTR_DEL);

	  // Delete PostQoS Counter
	  invokeGRPCall(
			args,
			(&curr_ctr),
			(("postDL" + std::string(args.qoscounter)).c_str()),
			GRPC_CTR_DEL);

          /* freed up counter id is returned to the stack */
          VLOG(1) << "Curr Ctr returned: " << curr_ctr << std::endl;
          counter.push(curr_ctr);
          break;
        case MSG_KEEPALIVE_ACK:
          my_dp_id = rbuf.dp_id.id;
          send_resp = false;
          VLOG(1) << "Got a keepalive ack from CP, and it gave me dp_id: "
                  << my_dp_id << std::endl;
          break;
        default:
          send_resp = false;
          VLOG(1) << "Got a request with mtype: " << mtype << std::endl;
          break;
      }

      if (send_resp == true) {
        size = zmq_send(sender, &resp, sizeof(resp), ZMQ_NOBLOCK);
        if (size == -1) {
          std::cerr << "Error in zmq sending: " << strerror(errno) << std::endl;
          break;
        } else {
          VLOG(1) << "Sending back response block" << std::endl;
        }
      }
    } else {
      VLOG(1) << "ZMQ poll timeout DPID " << my_dp_id << std::endl;
      gettimeofday(&current_time, NULL);
      if (current_time.tv_sec - last_ack.tv_sec > dp_cp_timeout_interval) {
	invokeGRPCall(args, NULL, args.pdrlookup, GRPC_PDR_CLR);
	invokeGRPCall(args, NULL, args.farlookup, GRPC_FAR_CLR);
	invokeGRPCall(args,
		      NULL,
		      (("pre" + std::string(args.qoscounter)).c_str()),
		      GRPC_CTR_CLR);
	invokeGRPCall(args,
		      NULL,
		      (("postUL" + std::string(args.qoscounter)).c_str()),
		      GRPC_CTR_CLR);
	invokeGRPCall(args,
		      NULL,
		      (("postDL" + std::string(args.qoscounter)).c_str()),
		      GRPC_CTR_CLR);

	std::cerr << "CP<-->DP communication broken. DPID: " << my_dp_id
		  << ". DP is restarting..." << std::endl;
	force_restart(argc, argv);
      }
      keepalive.dp_id.id = my_dp_id;
      int size = zmq_send(sender, &keepalive, sizeof(keepalive), ZMQ_NOBLOCK);
      if (size == -1) {
        std::cerr << "Error in zmq sending: " << strerror(errno) << std::endl;
        break;
      }
    }
  }

  return EXIT_SUCCESS;
}
/*--------------------------------------------------------------------------------*/
