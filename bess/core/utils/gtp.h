/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright 2019 Intel Corporation
 */
#ifndef BESS_UTILS_GTP_H_
#define BESS_UTILS_GTP_H_
/*----------------------------------------------------------------------------------*/
#include "endian.h"

namespace bess {
namespace utils {

#define EXT_TYPE_PDU_SESSION_CONTAINER 0x85

struct [[gnu::packed]] Gtpv1 {
  uint8_t pdn : 1, /* N-PDU number */
      seq : 1,     /* Sequence number */
      ex : 1,      /* Extension header */
      spare : 1,   /* Reserved field */
      pt : 1,      /* Protocol type */
      version : 3; /* Version */

  uint8_t type;  /* Message type */
  be16_t length; /* Message length */
  be32_t teid;   /* Tunnel endpoint identifier */
  /* The options start here. */

  /* Length of the header, its optional fields and every extension header,
   * provided all of it lies within the first `avail` bytes from the start of
   * this header; 0 otherwise. Nothing beyond `avail` is read.
   *
   * TS 29.281 5.1: the optional fields are present if any of E, S or PN is
   * set. 5.2.1: an extension header is n*4 octets with n positive, its first
   * octet holds n, and its last octet holds the next extension header type,
   * 0 ending the chain. A zero n is rejected rather than walked, since the
   * walk would never advance. */
  size_t header_length(size_t avail) const {
    const uint8_t *pktptr = reinterpret_cast<const uint8_t *>(this);
    size_t len = sizeof(Gtpv1);

    if (avail < len)
      return 0;
    if (!(seq || pdn || ex))
      return len;

    len += 4;
    if (avail < len)
      return 0;
    if (!ex)
      return len;

    /* pktptr[len - 1] is the next extension header type; pktptr[len] is the
     * length of the extension header that starts at len. */
    while (pktptr[len - 1]) {
      if (len >= avail)
        return 0;
      size_t ext_len = static_cast<size_t>(pktptr[len]) << 2;
      if (ext_len == 0 || ext_len > avail - len)
        return 0;
      len += ext_len;
    }
    return len;
  }
};

struct [[gnu::packed]] Gtpv1SeqPDUExt {
  be16_t seqnum; /* Sequence Number*/
  uint8_t npdu;  /* N-PDU number*/
  uint8_t ext;   /* Extension type */
};

struct [[gnu::packed]] Gtpv1PDUSessExt {
  uint8_t hlen;       /* Extension header length */
  uint8_t spare1 : 4, /* Spare */
      pdu_type : 4;   /* PDU type 0=DL/1=UL*/
  uint8_t qfi : 6,    /* QoS Flow Identifier*/
      spare2 : 2;     /* Spare */
  uint8_t next_type;  /* Next extension header type */

  size_t header_length() const { return sizeof(Gtpv1PDUSessExt) >> 2; }

  uint8_t type() const { return EXT_TYPE_PDU_SESSION_CONTAINER; }
};

}  // namespace utils
}  // namespace bess

/*----------------------------------------------------------------------------------*/
/**
 * GTPU header
 */
#define GTPU_VERSION 0x01
#define GTP_PROTOCOL_TYPE_GTP 0x01
#define GTP_GPDU 0xff
#define GTPU_ECHO_RECOVERY 14
#define GTPU_ECHO_REQUEST 0x01
#define GTPU_ECHO_RESPONSE 0x02

/// UDP header
#define UDP_PORT_GTPU 2152

/*----------------------------------------------------------------------------------*/
#endif /* BESS_UTILS_GTP_H_ */
