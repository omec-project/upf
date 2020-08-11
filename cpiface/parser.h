#ifndef __PARSER_H__
#define __PARSER_H__
/*--------------------------------------------------------------------------------*/
/* for json file parsing */
#include <jsoncpp/json/reader.h>
#include <jsoncpp/json/value.h>
/* for struct in_addr related functions */
#include <arpa/inet.h>
#include <net/if.h>
#include <sys/socket.h>
/* for getopt() */
#include <getopt.h>
/* for gethostbyname() */
#include <netdb.h>
/* for ioctl() */
#include <sys/ioctl.h>
/* fstream handling during json parsing */
#include <fstream>
/*--------------------------------------------------------------------------------*/
#define ZMQ_SERVER_IP "127.0.0.1"
#define ZMQ_RECV_PORT 20
#define ZMQ_SEND_PORT 5557
#define ZMQ_NB_IP "127.0.0.1"
#define ZMQ_NB_PORT 21
#define S1U_SGW_IP "127.0.0.1"
#define UDP_PORT_GTPU 2152
#define SCRIPT_NAME "/tmp/conf/upf.json"
#define COUNTER_LIMIT 50000
#define FILENAME_LEN 1024
/*--------------------------------------------------------------------------------*/
struct Args {
  char bessd_ip[HOSTNAME_LEN] = BESSD_IP;
  char nb_src_ip[HOSTNAME_LEN] = ZMQ_SERVER_IP;
  char nb_dst_ip[HOSTNAME_LEN] = ZMQ_NB_IP;
  char s1u_sgw_ip[HOSTNAME_LEN] = S1U_SGW_IP;
  uint16_t bessd_port = BESSD_PORT;
  uint16_t zmqd_send_port = ZMQ_SEND_PORT;
  uint16_t zmqd_recv_port = ZMQ_RECV_PORT;
  uint16_t zmqd_nb_port = ZMQ_NB_PORT;
  uint32_t counter_count = COUNTER_LIMIT;
  char pdrlookup[MODULE_NAME_LEN] = PDRLOOKUPMOD;
  char farlookup[MODULE_NAME_LEN] = FARLOOKUPMOD;
  char qoscounter[MODULE_NAME_LEN] = QOSCOUNTERMOD;
  char json_conf[FILENAME_LEN] = SCRIPT_NAME;

  struct RegMsgBundle {
    struct in_addr upf_comm_ip;
    struct in_addr s1u_ip;
    char hostname[HOSTNAME_LEN];
  } rmb = {{.s_addr = 0}, {.s_addr = 0}, ""};
  /*--------------------------------------------------------------------------------*/
  void parse(const int argc, char **argv) {
    int c;
    // Get args from command line
    static const struct option long_options[] = {
        {"bessd_ip", required_argument, NULL, 'B'},
        {"bessd_port", required_argument, NULL, 'b'},
        {"nb_src_ip", required_argument, NULL, 'Z'},
        {"zmqd_send_port", required_argument, NULL, 's'},
        {"zmqd_recv_port", required_argument, NULL, 'r'},
        {"nb_dst_ip", required_argument, NULL, 'N'},
        {"zmqd_nb_port", required_argument, NULL, 'n'},
        {"s1u_sgw_ip", required_argument, NULL, 'u'},
        {"pdrlookup", required_argument, NULL, 'P'},
        {"farlookup", required_argument, NULL, 'F'},
        {"qoscounter", required_argument, NULL, 'c'},
        {"hostname", required_argument, NULL, 'h'},
        {"json_config", required_argument, NULL, 'f'},
        {0, 0, 0, 0}};
    do {
      int option_index = 0;
      uint32_t val = 0;

      c = getopt_long(argc, argv, "B:b:Z:s:r:c:P:F:N:n:u:h:f:", long_options,
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
        case 'f':
          strncpy(json_conf, optarg, MIN(strlen(optarg), MODULE_NAME_LEN - 1));
          break;
        case 'Z':
          strncpy(nb_src_ip, optarg, MIN(strlen(optarg), HOSTNAME_LEN - 1));
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
          strncpy(nb_dst_ip, optarg, MIN(strlen(optarg), HOSTNAME_LEN - 1));
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

    // values from command line arguments always take precedence
    Json::Value root;
    Json::Reader reader;
    std::ifstream script(json_conf);
    script >> root;
    if (reader.parse(script, root, true)) {
      std::cerr << "Failed to parse configuration\n"
                << reader.getFormattedErrorMessages();
    }

    if (!strcmp(nb_dst_ip, ZMQ_NB_IP))
      getNBDstIPViaJson(nb_dst_ip,
                        root["cpiface"]["nb_dst_ip"].asString().c_str());
    strcpy(rmb.hostname, root["cpiface"]["hostname"].asString().c_str());
    if (!strcmp(nb_src_ip, ZMQ_SERVER_IP))
      getNBSrcIPViaJson(nb_src_ip, nb_dst_ip);
    if (!strcmp(s1u_sgw_ip, S1U_SGW_IP))
      getS1uAddrViaJson(s1u_sgw_ip,
                        root["access"]["ifname"].asString().c_str());
    counter_count = root["max_sessions"].asInt();
    script.close();
  }

  void getNBSrcIPViaJson(char *nb_src_ip, const char *nb_dst) {
#define DUMMY_PORT 9
    sockaddr_storage ss_addr = {0};
    unsigned long addr = inet_addr(nb_dst);

    ((struct sockaddr_in *)&ss_addr)->sin_addr.s_addr = addr;
    ((struct sockaddr_in *)&ss_addr)->sin_family = AF_INET;
    ((struct sockaddr_in *)&ss_addr)->sin_port = htons(DUMMY_PORT);

    int handle = socket(AF_INET, SOCK_DGRAM, IPPROTO_UDP);
    if (handle == -1) {
      std::cerr << "Unable to create socket for nb_src_ip probing."
                << " Sticking to original: " << nb_src_ip << std::endl;
      return;
    }
    socklen_t ss_addrlen = sizeof(ss_addr);
    if (connect(handle, (sockaddr *)&ss_addr, ss_addrlen) == -1 &&
        errno != ECONNREFUSED) {
      std::cerr << "Unable to determine nb_src_ip. "
                << " Sticking to original: " << nb_src_ip << std::endl;
      close(handle);
      return;
    }
    if (getsockname(handle, (sockaddr *)&ss_addr, &ss_addrlen) == -1) {
      std::cerr << "Unable to determine nb_src_ip. "
                << " Sticking to original: " << nb_src_ip << std::endl;
      close(handle);
      return;
    }

    char *source_address =
        inet_ntoa(((struct sockaddr_in *)&ss_addr)->sin_addr);
    std::cerr << "NB source address: " << source_address << std::endl;
    strcpy(nb_src_ip, source_address);
    close(handle);
  }
  /*--------------------------------------------------------------------------------*/
  void getNBDstIPViaJson(char *nb_dst_ip, const char *nb_dst) {
    struct hostent *he = gethostbyname(nb_dst);
    if (he == NULL) {
      std::cerr << "Failed to fetch IP address from host: " << nb_dst
                << ". Sticking to original: " << nb_dst_ip << std::endl;
      return;
    } else {
      struct in_addr raddr;
      memcpy(&raddr, he->h_addr, sizeof(uint32_t));
      strcpy(nb_dst_ip, inet_ntoa(raddr));
    }
  }
  /*--------------------------------------------------------------------------------*/
  void getS1uAddrViaJson(char *s1u_sgw_ip, const char *ifname) {
    int fd;
    struct ifreq ifr;

    fd = socket(AF_INET, SOCK_DGRAM, 0);

    if (fd != -1) {
      /* IPv4 address */
      ifr.ifr_addr.sa_family = AF_INET;
      /* IP address attached to "s1u" */
      strncpy(ifr.ifr_name, ifname, IFNAMSIZ - 1);
      if (ioctl(fd, SIOCGIFADDR, &ifr) == 0) {
        strcpy(s1u_sgw_ip,
               inet_ntoa(((struct sockaddr_in *)&ifr.ifr_addr)->sin_addr));
      }
      close(fd);
    }
  }
  /*--------------------------------------------------------------------------------*/
  void fetchHostname(char *hostname, size_t len) {
    char domain_str[HOSTNAME_LEN];

    if (gethostname(hostname, len) == -1) {
      std::cerr << "Error retrieving hostname: " << strerror(errno)
                << std::endl;
      return;
    }
    if (getdomainname(domain_str, len) == -1 || !strcmp(domain_str, "(none)")) {
      std::cerr << "Failed to read domain name!" << std::endl;
      return;
    }

    sprintf(hostname, "%s.%s", hostname, domain_str);
    VLOG(1) << "FQDN is: " << hostname << std::endl;
  }
};
/*--------------------------------------------------------------------------------*/
#endif /*__PARSER_H__*/
