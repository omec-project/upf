#ifndef BESS_UTILS_GTP_H_
#define BESS_UTILS_GTP_H_
/*----------------------------------------------------------------------------------*/
#include "endian.h"

namespace bess {
namespace utils {

	struct[[gnu::packed]] Gtpv1 {
		uint8_t pdn:1,					/* N-PDU number */
			seq:1,					/* Sequence number */
			ex:1,					/* Extension header */
			spare:1,				/* Reserved field */
			pt:1,					/* Protocol type */
			version:3;				/* Version */

		uint8_t type;					/* Message type */
		be16_t  length;					/* Message length */
		be32_t  teid;					/* Tunnel endpoint identifier */
		/* The options start here. */
	};
}  // namespace utils
}  // namespace bess
		
/*----------------------------------------------------------------------------------*/
#endif /* BESS_UTILS_GTP_H_ */
