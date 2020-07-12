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
#include <sys/time.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <unistd.h>
#include <zmq.h>
/*--------------------------------------------------------------------------------*/
#define ZMQ_SERVER_IP "172.17.0.2"
#define ZMQ_RECV_PORT 20
#define ZMQ_SEND_PORT 5557
#define ZMQ_NB_IP "172.17.0.1"
#define ZMQ_NB_PORT 21
#define S1U_SGW_IP "11.1.1.1"
#define ZMQ_POLL_TIMEOUT 1000  // in msecs
#define KEEPALIVE_TIMEOUT 100  // in secs
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
/*--------------------------------------------------------------------------------*/
struct Args {
  char bessd_ip[HOSTNAME_LEN] = BESSD_IP;
  char zmqd_ip[HOSTNAME_LEN] = ZMQ_SERVER_IP;
  char zmqd_nb_ip[HOSTNAME_LEN] = ZMQ_NB_IP;
  char s1u_sgw_ip[HOSTNAME_LEN] = S1U_SGW_IP;
  uint16_t bessd_port = BESSD_PORT;
  uint16_t zmqd_send_port = ZMQ_SEND_PORT;
  uint16_t zmqd_recv_port = ZMQ_RECV_PORT;
  uint16_t zmqd_nb_port = ZMQ_NB_PORT;
  char encapmod[MODULE_NAME_LEN] = ENCAPMOD;

  struct RegMsgBundle {
    struct in_addr upf_comm_ip;
    struct in_addr s1u_ip;
    char hostname[HOSTNAME_LEN];
  } rmb = {{.s_addr = 0}, {.s_addr = 0}, ""};
  void parse(const int argc, char **argv) {
    int c;
    // Get args from command line
    static const struct option long_options[] = {
        {"bessd_ip", required_argument, NULL, 'B'},
        {"bessd_port", required_argument, NULL, 'b'},
        {"zmqd_ip", required_argument, NULL, 'Z'},
        {"zmqd_send_port", required_argument, NULL, 's'},
        {"zmqd_recv_port", required_argument, NULL, 'r'},
        {"zmqd_nb_ip", required_argument, NULL, 'N'},
        {"zmqd_nb_port", required_argument, NULL, 'n'},
        {"s1u_sgw_ip", required_argument, NULL, 'u'},
        {"encapmod", required_argument, NULL, 'M'},
        {"hostname", required_argument, NULL, 'h'},
        {0, 0, 0, 0}};
    do {
      int option_index = 0;
      uint32_t val = 0;

      c = getopt_long(argc, argv, "B:b:Z:s:r:M:N:n:u:h:", long_options,
                      &option_index);

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
        case 'N':
          strncpy(zmqd_nb_ip, optarg, MIN(strlen(optarg), HOSTNAME_LEN - 1));
          break;
        case 'n':
          val = strtoul(optarg, NULL, 10);
          if (val == ULONG_MAX && errno == ERANGE) {
            std::cerr << "Failed to parse zmqd_nb_port" << std::endl;
            exit(EXIT_FAILURE);
          }
          zmqd_nb_port = (uint16_t)(val & 0x0000FFFF);
          break;
        case 'u':
          strncpy(s1u_sgw_ip, optarg, MIN(strlen(optarg), HOSTNAME_LEN - 1));
          break;
        case 'h':
          strncpy(rmb.hostname, optarg, MIN(strlen(optarg), HOSTNAME_LEN - 1));
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
void force_restart(int argc, char **argv) {
  pid_t pid;
  int status;

  pid = fork();

  if (pid == -1) {
    std::cerr << "Failed to fork: " << strerror(errno) << std::endl;
    exit(EXIT_FAILURE);
  } else if (pid == 0) {  // child process
    execv(argv[0], argv);
    exit(EXIT_SUCCESS);
  } else {  // parent process
    if (waitpid(pid, &status, 0) > 0) {
      if (WIFEXITED(status) && !WEXITSTATUS(status))
        std::cerr << "Restart successful!" << std::endl;
      else if (WIFEXITED(status) && WEXITSTATUS(status)) {
        if (WEXITSTATUS(status) == 127) {
          // execv failed
          std::cerr << "execv() failed\n" << std::endl;
        } else
          std::cerr << "Program terminated normally, "
                    << "but returned a non-zero status" << std::endl;
      } else
        std::cerr << "Program didn't terminate normally" << std::endl;
    } else {
      // waitpid() failed
      std::cerr << "waitpid() failed" << std::endl;
    }
    exit(EXIT_SUCCESS);
  }
}
/*--------------------------------------------------------------------------------*/
int main(int argc, char **argv) {
  GOOGLE_PROTOBUF_VERIFY_VERSION;

  std::map<uint64_t, bool> zmq_sess_map;
  // set my_dp_id to 0, SPGW-C will give me the id
  uint32_t my_dp_id = 0;
  struct timeval last_ack, current_time;
  // set it to 100 secs for the time being
  const uint32_t dp_cp_timeout_interval = KEEPALIVE_TIMEOUT;
  // 1 second zmq_poll timeout
  const uint32_t zmq_poll_timeout = ZMQ_POLL_TIMEOUT;
  // set default values first
  Args args;

  context0 = zmq_ctx_new();
  context1 = zmq_ctx_new();
  context2 = zmq_ctx_new();
  // set args coming from command-line
  args.parse(argc, argv);

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
  if (zmq_connect(reg, ("tcp://" + std::string(args.zmqd_nb_ip) + ":" +
                        std::to_string(args.zmqd_nb_port))
                           .c_str()) < 0) {
    std::cerr << "Failed to connect to registration port!: " << strerror(errno)
              << std::endl;
    return EXIT_FAILURE;
  }

  VLOG(1) << "Connected to registration handle" << std::endl;

  // Build message
  if (inet_aton(args.zmqd_ip, &args.rmb.upf_comm_ip) == 0) {
    std::cerr << "Invalid address: " << args.zmqd_ip << std::endl;
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
  if (zmq_bind(receiver, ("tcp://" + std::string(args.zmqd_ip) + ":" +
                          std::to_string(args.zmqd_recv_port))
                             .c_str()) != 0) {
    std::cerr << "Failed to bind to receiver ZMQ port!: " << strerror(errno)
              << std::endl;
    return EXIT_FAILURE;
  }
  //  Socket to send messages to
  sender = zmq_socket(context2, ZMQ_PUSH);
  if (zmq_connect(sender, ("tcp://" + std::string(args.zmqd_nb_ip) + ":" +
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
          resp.dp_id.id = rbuf.dp_id.id;
          resp.mtype = DPN_RESPONSE;
          resp.sess_id = rbuf.sess_entry.sess_id;
          if (zmq_sess_map.find(SESS_ID(rbuf.sess_entry.ue_addr.u.ipv4_addr,
                                        DEFAULT_BEARER)) ==
              zmq_sess_map.end()) {
            std::cerr << "No record found!" << std::endl;
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
          }
          {
            // Create BessClient
            BessClient b(CreateChannel(std::string(args.bessd_ip) + ":" +
                                           std::to_string(args.bessd_port),
                                       InsecureChannelCredentials()));
            b.runRemoveCommand((rbuf.sess_entry.ue_addr.u.ipv4_addr),
                               args.encapmod);
            std::map<std::uint64_t, bool>::iterator it = zmq_sess_map.find(
                SESS_ID((rbuf.sess_entry.ue_addr.u.ipv4_addr), DEFAULT_BEARER));
            zmq_sess_map.erase(it);
          }
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
        {
          // Before restarting, delete all session records
          // Create BessClient
          BessClient b(CreateChannel(std::string(args.bessd_ip) + ":" +
                                         std::to_string(args.bessd_port),
                                     InsecureChannelCredentials()));
          b.runRemoveAllCommand(args.encapmod);
        }
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
