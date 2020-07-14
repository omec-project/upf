#ifndef __TEMPLATE_H__
#define __TEMPLATE_H__
/*--------------------------------------------------------------------------------*/
/**
 * PDR template(s)
 */
// Downlink
static PDRArgs pdrD {
  .sit = Core,        /* source iface */
  .tipd = 0,          /* tunnel_ipv4_dst */
  .tipd_mask = 0,     /* tunnel_ipv4_dst mask */
  .enb_teid = 0,      /* enb teid */
  .enb_teid_mask = 0, /* enb teid mask */
  .saddr = 0, /* ueaddr ip. set it later */
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
  .fseid = 0, /* fseid. set it later */
  .ctr_id = 0,                           /* ctr_id. set it later */
  .far_id = 1,                                  /* far id */
  .need_decap = 0,                              /* need decap */
};

// Uplink
static PDRArgs pdrU = {
  .sit = Access,      /* source iface */
  .tipd = 0,          /* tunnel_ipv4_dst */
  .tipd_mask = 0,     /* tunnel_ipv4_dst mask */
  .enb_teid = 0,      /* enb teid */
  .enb_teid_mask = 0, /* enb teid mask */
  .saddr = 0,         /* inet ip */
  .saddr_mask = 0,    /* inet ip mask */
  .daddr = 0, /* ueaddr ip. set it later */
  .daddr_mask = 0xFFFFFFFFu, /* ueaddr ip mask */
  .sport = 0,                /* ueport */
  .sport_mask = 0,           /* ueport mask */
  .dport = 0,                /* inet port */
  .dport_mask = 0,           /* inet port mask */
  .protoid = 0,              /* proto-id */
  .protoid_mask = 0,         /* proto-id + mask */
  .pdr_id = 0,               /* pdr id */
  .fseid = 0, /* fseid. set it later */
  .ctr_id = 0,                           /* ctr_id. set it later */
  .far_id = 0,                                  /* far id */
  .need_decap = 1,                              /* need decap */
};

/**
 * FAR template(s)
 */
// Downlink
static FARArgs farD = {
  .far_id = 1,                                  /* far id*/
  .fseid = 0, /* fseid. set it later */
  .tunnel = 1,    /* needs tunnelling */
  .drop = 0,      /* needs dropping */
  .notify_cp = 0, /* notify cp */
  .tuntype = 1,   /* tunnel out type */
  .tun_src_ip = 0, /* n3 addr. set it later */
  .tun_dst_ip = 0, /* enb addr. set it later */
  .teid = 0, /* enb_teid. set it later  */
  .tun_port = UDP_PORT_GTPU,                   /* 2152 */
};

// Uplink
static FARArgs farU = {
  .far_id = 0,                                  /* far id*/
  .fseid = 0, /* fseid. set it later */
  .tunnel = 0,     /* needs tunnelling */
  .drop = 0,       /* needs dropping */
  .notify_cp = 0,  /* notify cp */
  .tuntype = 0,    /* tunnel out type */
  .tun_src_ip = 0, /* not needed */
  .tun_dst_ip = 0, /* not needed */
  .teid = 0,       /* not needed */
  .tun_port = 0,   /* not needed */
};
/*--------------------------------------------------------------------------------*/
#endif
