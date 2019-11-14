/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright(c) 2019 Intel Corporation
 */

#include "bess_control.h"
#include <arpa/inet.h>
#include <ctime>
#include <getopt.h>
#include <iterator>
#include <map>
#include <netdb.h>
#include <sys/socket.h>
#include <sys/types.h>
#include <unistd.h>
#include <zmq.h>
/*--------------------------------------------------------------------------------*/
#define ZMQ_SERVER_IP "172.17.0.1"
#define ZMQ_RECV_PORT 5560
#define ZMQ_SEND_PORT 5557
/*--------------------------------------------------------------------------------*/
/**
 * ZMQ stuff
 */
void *receiver;
void *sender;
void *context1;
void *context2;
/*--------------------------------------------------------------------------------*/
struct Args {
  char bessd_ip[HOSTNAME_LEN] = BESSD_IP;
  char zmqd_ip[HOSTNAME_LEN] = ZMQ_SERVER_IP;
  uint16_t bessd_port = BESSD_PORT;
  uint16_t zmqd_send_port = ZMQ_SEND_PORT;
  uint16_t zmqd_recv_port = ZMQ_RECV_PORT;
  char encapmod[MODULE_NAME_LEN] = ENCAPMOD;

  void parse(const int argc, char **argv) {
    int c;
    // Get args from command line
    static const struct option long_options[] = {
        {"bessd_ip", required_argument, NULL, 'B'},
        {"bessd_port", required_argument, NULL, 'b'},
        {"zmqd_ip", required_argument, NULL, 'Z'},
        {"zmqd_send_port", required_argument, NULL, 's'},
        {"zmqd_recv_port", required_argument, NULL, 'r'},
        {"encapmod", required_argument, NULL, 'M'},
        {0, 0, 0, 0}};
    do {
      int option_index = 0;
      uint32_t val = 0;

      c = getopt_long(argc, argv, "B:b:Z:s:r:M:", long_options, &option_index);

      if (c == -1)
        break;

      switch (c) {
        case 'B':
          strncpy(bessd_ip, optarg, MIN(strlen(optarg), HOSTNAME_LEN - 1));
          break;
        case 'b':
          val = strtoul(optarg, NULL, 10);
          if (val == ULONG_MAX && errno == ERANGE) {
            std::cerr << "Failed to parse bessd_port" << std::endl;
            exit(EXIT_FAILURE);
          }
          bessd_port = (uint16_t)(val & 0x0000FFFF);
          break;
        case 'Z':
          strncpy(zmqd_ip, optarg, MIN(strlen(optarg), HOSTNAME_LEN - 1));
          break;
        case 's':
          val = strtoul(optarg, NULL, 10);
          if (val == ULONG_MAX && errno == ERANGE) {
            std::cerr << "Failed to parse zmqd_send_port" << std::endl;
            exit(EXIT_FAILURE);
          }
          zmqd_send_port = (uint16_t)(val & 0x0000FFFF);
          break;
        case 'r':
          val = strtoul(optarg, NULL, 10);
          if (val == ULONG_MAX && errno == ERANGE) {
            std::cerr << "Failed to parse zmqd_recv_port" << std::endl;
            exit(EXIT_FAILURE);
          }
          zmqd_recv_port = (uint16_t)(val & 0x0000FFFF);
          break;
        case 'M':
          strncpy(encapmod, optarg, MIN(strlen(optarg), MODULE_NAME_LEN - 1));
          break;
        default:
          std::cerr << "Unknown argument - " << argv[optind] << std::endl;
          exit(EXIT_FAILURE);
          break;
      }
    } while (c != -1);
  }
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
int main(int argc, char **argv) {
  GOOGLE_PROTOBUF_VERIFY_VERSION;

  std::map<uint64_t, bool> zmq_sess_map;

  context1 = zmq_ctx_new();
  context2 = zmq_ctx_new();
  receiver = zmq_socket(context1, ZMQ_PULL);

  // set default values first
  Args args;
  // set args coming from command-line
  args.parse(argc, argv);

  if (context1 == NULL || context2 == NULL || receiver == NULL) {
    std::cerr << "Failed to create context(s) or receiver socket!: "
              << strerror(errno) << std::endl;
    return EXIT_FAILURE;
  }

  // Socket to recv message from
  if (zmq_connect(receiver, ("tcp://" + std::string(args.zmqd_ip) + ":" +
                             std::to_string(args.zmqd_recv_port))
                                .c_str()) < 0) {
    std::cerr << "Failed to connect to receiver!: " << strerror(errno)
              << std::endl;
    return EXIT_FAILURE;
  }

  //  Socket to send messages to
  sender = zmq_socket(context2, ZMQ_PUSH);
  if (zmq_connect(sender, ("tcp://" + std::string(args.zmqd_ip) + ":" +
                           std::to_string(args.zmqd_send_port))
                              .c_str()) < 0) {
    std::cerr << "Failed to connect to sender!: " << strerror(errno)
              << std::endl;
    return EXIT_FAILURE;
  }

  // register a signal handler
  if (signal(SIGTERM, sig_handler) == SIG_ERR) {
    std::cerr << "Unable to register signal handler!" << std::endl;
    return EXIT_FAILURE;
  }

  //  Process messages from either socket
  while (true) {
    static const zmq_pollitem_t items[] = {
        {receiver, 0, ZMQ_POLLIN, 0},
    };
    zmq_poll((zmq_pollitem_t *)items, 1, -1);
    if (items[0].revents & ZMQ_POLLIN) {
      struct msgbuf rbuf;
      struct resp_msgbuf resp;
      int size = zmq_recv(receiver, &rbuf, sizeof(rbuf), 0);
      if (size == -1) {
        std::cerr << "Error in zmq reception: " << strerror(errno) << std::endl;
        break;
      }
      long mtype = rbuf.mtype;
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
          resp.dp_id.id = DPN_ID;
          resp.mtype = DPN_RESPONSE;
          zmq_sess_map[SESS_ID(rbuf.sess_entry.ue_addr.u.ipv4_addr,
                               DEFAULT_BEARER)] = true;
          resp.sess_id = rbuf.sess_entry.sess_id;
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
          resp.dp_id.id = DPN_ID;
          resp.mtype = DPN_RESPONSE;
          resp.sess_id = rbuf.sess_entry.sess_id;
          if (zmq_sess_map.find(SESS_ID(rbuf.sess_entry.ue_addr.u.ipv4_addr,
                                        DEFAULT_BEARER)) ==
              zmq_sess_map.end()) {
            VLOG(1) << "No record found!" << std::endl;
            break;
          }
          {
            // Create BessClient
            BessClient b(CreateChannel(std::string(args.bessd_ip) + ":" +
                                           std::to_string(args.bessd_port),
                                       InsecureChannelCredentials()));
            b.runAddCommand(rbuf.sess_entry.ul_s1_info.sgw_teid,
                            rbuf.sess_entry.dl_s1_info.enb_teid,
                            rbuf.sess_entry.ue_addr.u.ipv4_addr,
                            rbuf.sess_entry.ul_s1_info.enb_addr.u.ipv4_addr,
                            args.encapmod);
          }
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
          resp.dp_id.id = DPN_ID;
          resp.mtype = DPN_RESPONSE;
          resp.sess_id = rbuf.sess_entry.sess_id;
          /* why is the ue ip address stored in reverse endian order just in
           * delete message? */
          if (zmq_sess_map.find(
                  SESS_ID(ntohl(rbuf.sess_entry.ue_addr.u.ipv4_addr),
                          DEFAULT_BEARER)) == zmq_sess_map.end()) {
            VLOG(1) << "No record found!" << std::endl;
            break;
          }
          {
            // Create BessClient
            BessClient b(CreateChannel(std::string(args.bessd_ip) + ":" +
                                           std::to_string(args.bessd_port),
                                       InsecureChannelCredentials()));
            b.runRemoveCommand(rbuf.sess_entry.ue_addr.u.ipv4_addr,
                               args.encapmod);
            std::map<std::uint64_t, bool>::iterator it = zmq_sess_map.find(
                SESS_ID(ntohl(rbuf.sess_entry.ue_addr.u.ipv4_addr),
                        DEFAULT_BEARER));
            zmq_sess_map.erase(it);
          }
          break;
        default:
          VLOG(1) << "Got a request with mtype: " << mtype << std::endl;
          break;
      }
      size = zmq_send(sender, &resp, sizeof(resp), ZMQ_NOBLOCK);
      if (size == -1) {
        std::cerr << "Error in zmq sending: " << strerror(errno) << std::endl;
        break;
      } else {
        VLOG(1) << "Sending back response block" << std::endl;
      }
    }
  }

  return EXIT_SUCCESS;
}
/*--------------------------------------------------------------------------------*/
