/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright(c) 2019 Intel Corporation
 */
#ifndef __GTP_COMMON_H__
#define __GTP_COMMON_H__
/*----------------------------------------------------------------------------------*/
#define DPN_ID 			12345
/**
 * Maximum buffer/name length
 */
#define MAX_LEN 		128

/**
 * MAX DNS Sponsor ID name lenth
 */
#define MAX_DNS_SPON_ID_LEN 	16

/**
 * IPv6 address length
 */
#define IPV6_ADDR_LEN 		16

/**
 * Maximum PCC rules per session.
 */
#define MAX_PCC_RULES 		12

/**
 * Maximum PCC rules per session.
 */
#define MAX_ADC_RULES 		16

/**
 * Maximum CDR services.
 */
#define MAX_SERVICE		1

enum {
	MSG_SESS_CRE = 		2,
	MSG_SESS_MOD,
	MSG_SESS_DEL
};

enum {
	DPN_RESPONSE	=	4,
	DPN_CREATE_RESP = 	10,
	DPN_MODIFY_RESP,
	DPN_DELETE_RESP
};

/**
 *  Select IPv4 or IPv6.
 */
enum iptype {
	IPTYPE_IPV4 = 		0,     /* IPv4. */
	IPTYPE_IPV6,         	       /* IPv6. */
};

/**
 * check if nth bit is set.
 */
#define ISSET_BIT(mask, n)  (((mask) & (1LLU << (n))) ? 1 : 0)

/**
 * Default bearer session
 */
#define DEFAULT_BEARER			5

/**
 * set session id from the combination of
 * unique UE addr and Bearer id
 */
#define SESS_ID(ue_addr, br_id)		(((uint64_t)(br_id) << 32) | (0xffffffff & (ue_addr)))
				      /* [0] 28 bits | [bearer-id] 4 bits | [ue-addr] 32 bits */
/**
 * get bearer id
 */
#define UE_BEAR_ID(x)			(x>>32)

/**
 * get ue_addr
 */
#define UE_ADDR(x)			(x & 0xffffffff)
/*----------------------------------------------------------------------------------*/
/*
 * Response Message Structure
 */
/* ASR- TODO- Check struct cdr_msg wrt to TMOPL requirement xlsx */
struct cdr_msg {
	uint32_t ratingGroup;
	uint64_t localSequenceNumber;
	uint64_t timeOfFirstUsage;
	uint64_t timeOfLastUsage;
	uint64_t timeUsage;
	uint8_t record_closure_cause;
	uint64_t datavolumeFBCUplink;
	uint64_t datavolumeFBCDownlink;
	uint64_t timeOfReport;			//TODO: CP will report
	void *ue_context;
	uint32_t apn_idx;
};

/**
   * IPv4 or IPv6 address configuration structure.
   */
struct ip_addr {
	enum iptype iptype;                     /* IP type: IPv4 or IPv6. */
	union {
		uint32_t ipv4_addr;             /* IPv4 address*/
		uint8_t  ipv6_addr[IPV6_ADDR_LEN]; /* IPv6 address*/
	} u;
} __attribute__((packed, aligned(64)));

/**
 * UpLink S1u interface config structure.
 */
struct ul_s1_info {
	uint32_t sgw_teid;              /* SGW teid */
	struct ip_addr enb_addr;        /* eNodeB address */
	struct ip_addr sgw_addr;        /* Serving Gateway address */
	struct ip_addr s5s8_pgwu_addr;  /* S5S8_PGWU address */
} __attribute__((packed, aligned(64)));

/**
 * DownLink S1u interface config structure.
 */
struct dl_s1_info {
	uint32_t enb_teid;              /* eNodeB teid */
	struct ip_addr enb_addr;        /* eNodeB address */
	struct ip_addr sgw_addr;        /* Serving Gateway address */
	struct ip_addr s5s8_sgwu_addr;  /* S5S8_SGWU address */
} __attribute__((packed, aligned(64)));

/**
 * Packet filter configuration structure.
 */
struct service_data_list {
	uint32_t        service[MAX_SERVICE];   /* list of service id*/
	/* TODO: add other members*/
};

struct cdr {
	uint64_t bytes;
	uint64_t pkt_count;
};

/**
 * Volume based Charging
 */
struct chrg_data_vol {
	struct cdr ul_cdr;              /* Uplink cdr */
	struct cdr dl_cdr;              /* Downlink cdr */
	struct cdr ul_drop;             /* Uplink dropped cdr */
	struct cdr dl_drop;             /* Downlink dropped cdr */
};

/**
 * IP-CAN Bearer Charging Data Records
 */
struct ipcan_dp_bearer_cdr {
	uint32_t charging_id;				/* Bearer Charging id*/
	uint32_t pdn_conn_charging_id;			/* PDN connection charging id*/
	struct tm record_open_time;			/* Record time*/
	uint64_t duration_time;				/* duration (sec)*/
	uint8_t	record_closure_cause;			/* Record closure cause*/
	uint64_t record_seq_number;			/* Sequence no.*/
	uint8_t charging_behavior_index; 		/* Charging index*/
	uint32_t service_id;				/* to identify the service
						 	 * or the service component
						 	 * the bearer relates to*/
	char sponsor_id[MAX_DNS_SPON_ID_LEN];	        /* to identify the 3rd party organization (the
						 	 * sponsor) willing to pay for the operator's charge*/
	struct service_data_list service_data_list; 	/* List of service*/
	uint32_t rating_group;				/* rating group of this bearer*/
	uint64_t vol_threshold;				/* volume threshold in MBytes*/
	struct chrg_data_vol data_vol;			/* charing per UE by volume*/
	uint32_t charging_rule_id;			/* Charging Rule ID*/
} __attribute__((packed, aligned(64)));

/**
 * Bearer Session information structure
 */
struct session_info {
	struct ip_addr ue_addr;						/* UE ip address*/
	struct ul_s1_info ul_s1_info;					/* UpLink S1u info*/
	struct dl_s1_info dl_s1_info;					/* DownLink S1u info*/
	uint8_t bearer_id;						/* Bearer ID*/

	/* PCC rules related params*/
	uint32_t num_ul_pcc_rules;					/* No. of UL PCC rule*/
	uint32_t ul_pcc_rule_id[MAX_PCC_RULES]; 			/* PCC rule id supported in UL*/
	uint32_t num_dl_pcc_rules;					/* No. of PCC rule*/
	uint32_t dl_pcc_rule_id[MAX_PCC_RULES];				/* PCC rule id*/

	/* ADC rules related params*/
	uint32_t num_adc_rules;						/* No. of ADC rule*/
	uint32_t adc_rule_id[MAX_ADC_RULES]; 				/* List of ADC rule id*/

	/* Charging Data Records*/
	struct ipcan_dp_bearer_cdr ipcan_dp_bearer_cdr;			/* Charging Data Records*/
	uint32_t client_id;

	uint64_t op_id;
	uint64_t sess_id;						/* session id of this bearer
									 * last 4 bits of sess_id
									 * maps to bearer id*/
	uint32_t service_id;						/* Type of service given
									 * given to this session like
									 * Internet, Management, CIPA etc
									 */
	uint32_t ul_apn_mtr_idx;		/* UL APN meter profile index*/
	uint32_t dl_apn_mtr_idx;		/* DL APN meter profile index*/
} __attribute__((packed, aligned(64)));

/**
 * DataPlane identifier information structure.
 */
struct dp_id {
	uint64_t id;			/* table identifier.*/
	char name[MAX_LEN];		/* name string of identifier*/
} __attribute__((packed, aligned(64)));

/*
 * Response Message Structure
 */
struct resp_msgbuf {
	long mtype;
	uint64_t op_id;
	uint64_t sess_id;
	struct dp_id dp_id;
};

/*
 * Message Structure
 */
struct msgbuf {
	long mtype;
	struct dp_id dp_id;
	struct session_info sess_entry;
};
/*----------------------------------------------------------------------------------*/
#endif /* !__GTP_COMMON_H__ */
