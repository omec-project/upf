/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright(c) 2019 Intel Corporation
 */
#ifndef BESS_UTILS_GTP_H_
#define BESS_UTILS_GTP_H_
/*----------------------------------------------------------------------------------*/
#include "endian.h"

namespace bess {
namespace utils {

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

  size_t header_length() const {
    const Gtpv1 *gtph = this;
    const uint8_t *pktptr = (const uint8_t *)this;
    size_t len = sizeof(Gtpv1);

    if (gtph->seq || gtph->pdn || gtph->ex)
      len += 4;
    if (gtph->ex) {
      /* Probe till the last extension header */
      /* calculate total len of gtp header (with options) */
      while (pktptr[len - 1])
        len += (pktptr[len] << 2);
    }
    return len;
  }
};
}  // namespace utils
}  // namespace bess

/*----------------------------------------------------------------------------------*/
#endif /* BESS_UTILS_GTP_H_ */
