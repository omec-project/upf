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
#define ZMQ_SERVER_IP "127.0.0.1"
#define ZMQ_RECV_PORT 20
#define ZMQ_SEND_PORT 5557
#define ZMQ_NB_IP "127.0.0.1"
#define ZMQ_NB_PORT 1111
#define S1U_SGW_IP "127.0.0.1"
#define UDP_PORT_GTPU 2152
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
  char pdrlookup[MODULE_NAME_LEN] = PDRLOOKUPMOD;
  char farlookup[MODULE_NAME_LEN] = FARLOOKUPMOD;
  char qoscounter[MODULE_NAME_LEN] = QOSCOUNTERMOD;

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
        {"pdrlookup", required_argument, NULL, 'P'},
        {"farlookup", required_argument, NULL, 'F'},
        {"qoscounter", required_argument, NULL, 'c'},
        {"hostname", required_argument, NULL, 'h'},
        {0, 0, 0, 0}};
    do {
      int option_index = 0;
      uint32_t val = 0;

      c = getopt_long(argc, argv, "B:b:Z:s:r:c:P:F:N:n:u:h:", long_options,
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
        case 'c':
          strncpy(qoscounter, optarg, MIN(strlen(optarg), MODULE_NAME_LEN - 1));
          break;
        case 'F':
          strncpy(farlookup, optarg, MIN(strlen(optarg), MODULE_NAME_LEN - 1));
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
        case 'P':
          strncpy(pdrlookup, optarg, MIN(strlen(optarg), MODULE_NAME_LEN - 1));
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
int main(int argc, char **argv) {
  GOOGLE_PROTOBUF_VERIFY_VERSION;

  /* key: SESS_ID(rbuf.sess_entry.ue_addr.u.ipv4_addr, DEFAULT_BEARER), val:
   * enb_teid) */
  std::map<uint64_t, uint32_t> zmq_sess_map;

  context0 = zmq_ctx_new();
  context1 = zmq_ctx_new();
  context2 = zmq_ctx_new();

  // set default values first
  Args args;
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

  //  Process messages from either socket
  while (true) {
    static const zmq_pollitem_t items[] = {
        {receiver, 0, ZMQ_POLLIN, 0},
    };
    if (zmq_poll((zmq_pollitem_t *)items, 1, -1) < 0) {
      std::cerr << "ZMQ poll failed!: " << strerror(errno) << std::endl;
      return EXIT_FAILURE;
    }
    if (items[0].revents & ZMQ_POLLIN) {
      struct msgbuf rbuf;
      struct resp_msgbuf resp;
      int size = zmq_recv(receiver, &rbuf, sizeof(rbuf), 0);
      if (size == -1) {
        std::cerr << "Error in zmq reception: " << strerror(errno) << std::endl;
        break;
      }
      long mtype = rbuf.mtype;
      uint32_t enb_teid = 0;
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
          resp.dp_id.id = rbuf.dp_id.id;
          resp.mtype = DPN_RESPONSE;
          zmq_sess_map[SESS_ID(rbuf.sess_entry.ue_addr.u.ipv4_addr,
                               DEFAULT_BEARER)] = 0;
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
          } else {
            zmq_sess_map[SESS_ID(rbuf.sess_entry.ue_addr.u.ipv4_addr,
                                 DEFAULT_BEARER)] =
                rbuf.sess_entry.dl_s1_info.enb_teid;
          }
          {
            // Add PDR (DOWNLINK)
            BessClient b(CreateChannel(std::string(args.bessd_ip) + ":" +
                                           std::to_string(args.bessd_port),
                                       InsecureChannelCredentials()));
            PDRArgs pa = {
                .sit = Core,        /* source iface */
                .tipd = 0,          /* tunnel_ipv4_dst */
                .tipd_mask = 0,     /* tunnel_ipv4_dst mask */
                .enb_teid = 0,      /* enb teid */
                .enb_teid_mask = 0, /* enb teid mask */
                .saddr = rbuf.sess_entry.ue_addr.u.ipv4_addr, /* ueaddr ip */
                .saddr_mask = 0xFFFFFFFFu, /* ueaddr ip mask */
                .daddr = 0,                /* inet ip */
                .daddr_mask = 0,           /* inet ip mask */
                .sport = 0,                /* ueport */
                .sport_mask = 0,           /* ueport mask */
                .dport = 0,                /* inet port */
                .dport_mask = 0,           /* inet port mask */
                .protoid = 0,              /* proto-id */
                .protoid_mask = 0,         /* proto-id + mask */
                .pdr_id = 0,               /* pdr id */
                .fseid = rbuf.sess_entry.dl_s1_info.enb_teid,  /* fseid */
                .ctr_id = rbuf.sess_entry.dl_s1_info.enb_teid, /* ctr_id */
                .far_id = 1,                                   /* far id */
                .need_decap = 0,                               /* need decap */
            };
            b.runAddPDRCommand(&pa, args.pdrlookup);
          }
          {
            // Add PDR (UPLINK)
            BessClient b(CreateChannel(std::string(args.bessd_ip) + ":" +
                                           std::to_string(args.bessd_port),
                                       InsecureChannelCredentials()));
            PDRArgs pa = {
                .sit = Access,      /* source iface */
                .tipd = 0,          /* tunnel_ipv4_dst */
                .tipd_mask = 0,     /* tunnel_ipv4_dst mask */
                .enb_teid = 0,      /* enb teid */
                .enb_teid_mask = 0, /* enb teid mask */
                .saddr = 0,         /* inet ip */
                .saddr_mask = 0,    /* inet ip mask */
                .daddr = rbuf.sess_entry.ue_addr.u.ipv4_addr, /* ueaddr ip */
                .daddr_mask = 0xFFFFFFFFu, /* ueaddr ip mask */
                .sport = 0,                /* ueport */
                .sport_mask = 0,           /* ueport mask */
                .dport = 0,                /* inet port */
                .dport_mask = 0,           /* inet port mask */
                .protoid = 0,              /* proto-id */
                .protoid_mask = 0,         /* proto-id + mask */
                .pdr_id = 0,               /* pdr id */
                .fseid = rbuf.sess_entry.dl_s1_info.enb_teid,  /* fseid */
                .ctr_id = rbuf.sess_entry.dl_s1_info.enb_teid, /* ctr_id */
                .far_id = 0,                                   /* far id */
                .need_decap = 1,                               /* need decap */
            };
            b.runAddPDRCommand(&pa, args.pdrlookup);
          }
          {
            // Add FAR (DOWNLINK)
            BessClient b(CreateChannel(std::string(args.bessd_ip) + ":" +
                                           std::to_string(args.bessd_port),
                                       InsecureChannelCredentials()));
            FARArgs fa = {
                .far_id = 1,                                  /* far id*/
                .fseid = rbuf.sess_entry.dl_s1_info.enb_teid, /* fseid */
                .tunnel = 1,    /* needs tunnelling */
                .drop = 0,      /* needs dropping */
                .notify_cp = 0, /* notify cp */
                .tuntype = 1,   /* tunnel out type */
                .tun_src_ip =
                    ntohl((uint32_t)(inet_addr(S1U_SGW_IP))), /* n3 addr */
                .tun_dst_ip = rbuf.sess_entry.ul_s1_info.enb_addr.u
                                  .ipv4_addr,                /* enb addr */
                .teid = rbuf.sess_entry.dl_s1_info.enb_teid, /* enb_teid */
                .tun_port = UDP_PORT_GTPU,                   /* 2152 */
            };
            b.runAddFARCommand(&fa, args.farlookup);
          }
          {
            // Add FAR (UPLINK)
            BessClient b(CreateChannel(std::string(args.bessd_ip) + ":" +
                                           std::to_string(args.bessd_port),
                                       InsecureChannelCredentials()));
            FARArgs fa = {
                .far_id = 0,                                  /* far id*/
                .fseid = rbuf.sess_entry.dl_s1_info.enb_teid, /* fseid */
                .tunnel = 0,     /* needs tunnelling */
                .drop = 0,       /* needs dropping */
                .notify_cp = 0,  /* notify cp */
                .tuntype = 0,    /* tunnel out type */
                .tun_src_ip = 0, /* not needed */
                .tun_dst_ip = 0, /* not needed */
                .teid = 0,       /* not needed */
                .tun_port = 0,   /* not needed */
            };
            b.runAddFARCommand(&fa, args.farlookup);
          }
          {
            // Add PreQoS Counter
            BessClient b(CreateChannel(std::string(args.bessd_ip) + ":" +
                                           std::to_string(args.bessd_port),
                                       InsecureChannelCredentials()));
            b.runAddCounterCommand(
                (rbuf.sess_entry.dl_s1_info.enb_teid),
                (("Pre" + std::string(args.qoscounter)).c_str()));
          }
          {
            // Add PostQoS Counter
            BessClient b(CreateChannel(std::string(args.bessd_ip) + ":" +
                                           std::to_string(args.bessd_port),
                                       InsecureChannelCredentials()));
            b.runAddCounterCommand(
                (rbuf.sess_entry.dl_s1_info.enb_teid),
                (("Post" + std::string(args.qoscounter)).c_str()));
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
          if (zmq_sess_map.find(
                  SESS_ID(ntohl(rbuf.sess_entry.ue_addr.u.ipv4_addr),
                          DEFAULT_BEARER)) == zmq_sess_map.end()) {
            std::cerr << "No record found!" << std::endl;
            break;
          } else
            enb_teid = zmq_sess_map[SESS_ID(
                ntohl(rbuf.sess_entry.ue_addr.u.ipv4_addr), DEFAULT_BEARER)];
          {
            std::map<std::uint64_t, uint32_t>::iterator it = zmq_sess_map.find(
                SESS_ID(ntohl(rbuf.sess_entry.ue_addr.u.ipv4_addr),
                        DEFAULT_BEARER));
            zmq_sess_map.erase(it);
          }
          {
            // Delete PDR (DOWNLINK)
            BessClient b(CreateChannel(std::string(args.bessd_ip) + ":" +
                                           std::to_string(args.bessd_port),
                                       InsecureChannelCredentials()));
            PDRArgs pa = {
                .sit = Core,        /* source iface */
                .tipd = 0,          /* tunnel_ipv4_dst */
                .tipd_mask = 0,     /* tunnel_ipv4_dst mask */
                .enb_teid = 0,      /* enb teid */
                .enb_teid_mask = 0, /* enb teid mask */
                .saddr =
                    ntohl(rbuf.sess_entry.ue_addr.u.ipv4_addr), /* ueaddr ip */
                .saddr_mask = 0xFFFFFFFFu, /* ueaddr ip mask */
                .daddr = 0,                /* inet ip */
                .daddr_mask = 0,           /* inet ip mask */
                .sport = 0,                /* ueport */
                .sport_mask = 0,           /* ueport mask */
                .dport = 0,                /* inet port */
                .dport_mask = 0,           /* inet port mask */
                .protoid = 0,              /* proto-id */
                .protoid_mask = 0,         /* proto-id + mask */
                .pdr_id = 0,               /* pdr id (not needed) */
                .fseid = 0,                /* fseid (not needed) */
                .ctr_id = 0,               /* ctr_id (not needed) */
                .far_id = 0,               /* far id (not needed) */
                .need_decap = 0,           /* need decap (not needed) */
            };
            b.runDelPDRCommand(&pa, args.pdrlookup);
          }
          {
            // Delete PDR (UPLINK)
            BessClient b(CreateChannel(std::string(args.bessd_ip) + ":" +
                                           std::to_string(args.bessd_port),
                                       InsecureChannelCredentials()));
            PDRArgs pa = {
                .sit = Access,      /* source iface */
                .tipd = 0,          /* tunnel_ipv4_dst */
                .tipd_mask = 0,     /* tunnel_ipv4_dst mask */
                .enb_teid = 0,      /* enb teid */
                .enb_teid_mask = 0, /* enb teid mask */
                .saddr = 0,         /* inet ip */
                .saddr_mask = 0,    /* inet ip mask */
                .daddr =
                    ntohl(rbuf.sess_entry.ue_addr.u.ipv4_addr), /* ueaddr ip */
                .daddr_mask = 0xFFFFFFFFu, /* ueaddr ip mask */
                .sport = 0,                /* ueport */
                .sport_mask = 0,           /* ueport mask */
                .dport = 0,                /* inet port */
                .dport_mask = 0,           /* inet port mask */
                .protoid = 0,              /* proto-id */
                .protoid_mask = 0,         /* proto-id + mask */
                .pdr_id = 0,               /* pdr id (not needed) */
                .fseid = 0,                /* fseid (not needed) */
                .ctr_id = 0,               /* ctr_id (not needed) */
                .far_id = 0,               /* far id (not needed) */
                .need_decap = 0,           /* need decap (not needed) */
            };
            b.runDelPDRCommand(&pa, args.pdrlookup);
          }
          {
            // Del FAR (DOWNLINK)
            BessClient b(CreateChannel(std::string(args.bessd_ip) + ":" +
                                           std::to_string(args.bessd_port),
                                       InsecureChannelCredentials()));
            FARArgs fa;
            fa.far_id = 1;
            fa.fseid = enb_teid;
            b.runDelFARCommand(&fa, args.farlookup);
          }
          {
            // Del FAR (UPLINK)
            BessClient b(CreateChannel(std::string(args.bessd_ip) + ":" +
                                           std::to_string(args.bessd_port),
                                       InsecureChannelCredentials()));
            FARArgs fa;
            fa.far_id = 0;
            fa.fseid = enb_teid;
            b.runDelFARCommand(&fa, args.farlookup);
          }
          {
            // Delete PreQoS Counter
            BessClient b(CreateChannel(std::string(args.bessd_ip) + ":" +
                                           std::to_string(args.bessd_port),
                                       InsecureChannelCredentials()));
            b.runDelCounterCommand(
                (enb_teid), (("Pre" + std::string(args.qoscounter)).c_str()));
          }
          {
            // Delete PostQoS Counter
            BessClient b(CreateChannel(std::string(args.bessd_ip) + ":" +
                                           std::to_string(args.bessd_port),
                                       InsecureChannelCredentials()));
            b.runDelCounterCommand(
                (enb_teid), (("Post" + std::string(args.qoscounter)).c_str()));
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
