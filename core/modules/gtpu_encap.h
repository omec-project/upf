#ifndef BESS_MODULES_GTPUENCAP_H_
#define BESS_MODULES_GTPUENCAP_H_
/*----------------------------------------------------------------------------------*/
#include <map>
#include "../module.h"
#include "../pb/module_msg.pb.h"
#include "utils/cuckoo_map.h"
/*----------------------------------------------------------------------------------*/
/* TODO - XXX: Cleanup macros. Get them from bess python file */
#define IPV6_ADDR_LEN			16
#define MAX_DNS_SPON_ID_LEN		16
#define UE_IP_START			"16.0.0.1"
#define UE_IP_START_RANGE		"16.0.0.0"
#define AS_IP_START			"13.1.1.110"
#define ENODEB_IP_START			"11.1.1.101"
#define S1U_SGW_IP			"11.1.1.93"
/**
 * GTPU header
 */
#define GTPU_VERSION			0x01
#define GTP_PROTOCOL_TYPE_GTP		0x01
#define GTP_GPDU			0xff

/**
 * UDP header
 */
#define UDP_PORT_GTPU			2152
/**
 * Default bearer session
 */
#define DEFAULT_BEARER			5
/**
 * set session id from the combination of
 * unique UE id and Bearer id
 */
#define SESS_ID(ue_id, br_id)		(((uint64_t)(ue_id) << 4) | (0xf & (br_id)))
#define UE_SESS_ID(x)			(x>>4)
/**
 * get bearer id
 */
#define UE_BEAR_ID(x)			(x & 0xf)
/*----------------------------------------------------------------------------------*/
/**
 * XXX - TODO: Clean up struct declarations. Remove redundant fields
 */
/**
 * Select IPv4 or IPv6
 */
enum iptype {
	IPTYPE_IPV4	=	0,	/* IPv4 */
	IPTYPE_IPV6,			/* IPv6 */
};
/*----------------------------------------------------------------------------------*/
/**
 * IPv4 or IPv6 address configuration structure.
 */
struct ip_addr {
	enum iptype iptype;					/* IP type: IPv4 or IPv6 */
	union {
		uint32_t ipv4_addr;				/* IPv4 address */
		uint8_t ipv6_addr[IPV6_ADDR_LEN];		/* IPv6 address */
	} u;
} __attribute__((packed, aligned(RTE_CACHE_LINE_SIZE)));
/*----------------------------------------------------------------------------------*/
/**
 * UpLink S1u interface config structure.
 */
struct ul_s1_info {
        uint32_t sgw_teid;              /* SGW teid*/
        struct ip_addr enb_addr;        /* eNodeB address*/
        struct ip_addr sgw_addr;        /* Serving Gateway address*/
        struct ip_addr s5s8_pgwu_addr;  /* S5S8_PGWU address*/
} __attribute__((packed, aligned(RTE_CACHE_LINE_SIZE)));

/*----------------------------------------------------------------------------------*/
/**
 * DownLink S1u interface config structure.
 */
struct dl_s1_info {
        uint32_t enb_teid;              /* eNodeB teid*/
        struct ip_addr enb_addr;        /* eNodeB address*/
        struct ip_addr sgw_addr;        /* Serving Gateway address*/
        struct ip_addr s5s8_sgwu_addr;  /* S5S8_SGWU address*/
} __attribute__((packed, aligned(RTE_CACHE_LINE_SIZE)));
/*----------------------------------------------------------------------------------*/
/**
 * IP-CAN Bearer Charging Data Records
 */
struct ipcan_dp_bearer_cdr {
        uint32_t charging_id;                   /* Bearer Charging id*/
        uint32_t pdn_conn_charging_id;          /* PDN connection charging id*/
        //struct tm record_open_time;           /* Record time*/
        uint64_t duration_time;                 /* duration (sec)*/
        uint8_t record_closure_cause;           /* Record closure cause*/
        uint64_t record_seq_number;             /* Sequence no.*/
        uint8_t charging_behavior_index;        /* Charging index*/
        uint32_t service_id;                    /* to identify the service
                                                 * or the service component
                                                 * the bearer relates to*/
	char sponsor_id[MAX_DNS_SPON_ID_LEN];   /* to identify the 3rd party organization (the
                                                 * sponsor) willing to pay for the operator's charge*/
        //struct service_data_list service_data_list; /* List of service*/
        uint32_t rating_group;                  /* rating group of this bearer*/
        uint64_t vol_threshold;                 /* volume threshold in MBytes*/
        //struct chrg_data_vol data_vol;        /* charing per UE by volume*/
        uint32_t charging_rule_id;              /* Charging Rule ID*/
} __attribute__((packed, aligned(RTE_CACHE_LINE_SIZE)));
/*----------------------------------------------------------------------------------*/
/**
 * Bearer Session information structure
 */
typedef struct session_info {
	struct ip_addr ue_addr;					/* UE IP address */
	struct ul_s1_info ul_s1_info;				/* Uplink S1U info */
	struct dl_s1_info dl_s1_info;				/* Downlink SGI info */
	struct ipcan_dp_bearer_cdr ipcan_dp_bearer_cdr;		/* Charging data records */
	uint64_t sess_id;					/* 
								 * session id of this bearer
								 * last 4 bits of sess_id maps
								 * to bearer id
								 */
} session_info;
/*----------------------------------------------------------------------------------*/
/**
 * GTPU header without seq
 */
typedef struct gtpu_hdr {
	uint8_t pdn:1,				/* N-PDU number */
		seq:1,				/* Sequence number */
		ex:1,				/* Extension header */
		spare:1,			/* Reserved field */
		pt:1,				/* Protocol type */
		version:3;			/* Version */
	
	uint8_t type;				/* Message type */
	uint16_t length;			/* Message length */
	uint32_t teid;				/* Tunnel endpoint identifier */
} gtpu_hdr;
/*----------------------------------------------------------------------------------*/
/**
 * @brief Defines number of entries in local database.
 *
 * Recommended local table size to remain within L2 cache: 64000 entries.
 * See README for detailed calculations.
 */
const uint32_t SUBSCRIBERS			= 50000;
const uint32_t NG4T_MAX_UE_RAN			= 500000;
const uint32_t NG4T_MAX_ENB_RAN			= 80;
const uint32_t base_s1u_spgw_gtpu_teid		= 0xf0000000;
/*----------------------------------------------------------------------------------*/
class GtpuEncap final : public Module {
 public:
	GtpuEncap() {
		max_allowed_workers_ = Worker::kMaxWorkers;
		s1u_spgw_gtpu_teid_offset = 0;
	}
	
	//static const gate_idx_t kNumOGates = 0;

	CommandResponse Init(const bess::pb::EmptyArg &arg);
	void ProcessBatch(Context *ctx, bess::PacketBatch *batch) override;
	
 private:
	int HashCreate();
	uint32_t SimuCPEnbv4Teid(int ue_idx, int max_ue_ran, int max_enb_ran,
				 uint32_t *teid, uint32_t *enb_idx);
	int dp_session_create(struct session_info *entry, int index);
	inline void GenerateTEID(uint32_t *teid);
	uint32_t s1u_spgw_gtpu_teid_offset;
	bess::utils::CuckooMap<uint32_t, uint64_t> session_map;
};
/*----------------------------------------------------------------------------------*/
#endif  // BESS_MODULES_GTPUENCAP_H_
