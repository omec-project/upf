// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"time"

	"github.com/wmnsk/go-pfcp/message"
)

// deletionResponseRetention is how long the connection to a peer keeps its response to a
// Session Deletion Request. A control plane retransmits a request until its own retry
// budget runs out (TS 29.244 clause 6.4 leaves T1 and N1 to the implementation): the
// omec-project SMF sends a request up to 3 times in all, 3 s apart, and stops waiting 9 s
// after the first send; it keeps its own responses for 15 s.
const deletionResponseRetention = 15 * time.Second

type deletionKey struct {
	localSEID uint64
	seq       uint32
}

type cachedDeletion struct {
	rsp message.Message
	at  time.Time
}

// deletionResponses keeps a connection's recent responses to Session Deletion Requests. "Not counting retransmissions, a Request message shall be answered with a
// single Response message" (TS 29.244 clause 6.4): a control plane that lost the response
// retransmits the request, and by then the session is gone. Answered "Session context not
// found", the control plane is told the deletion it asked for failed; it is answered with
// the response instead. Under sessionsMu.
type deletionResponses struct {
	byKey map[deletionKey]cachedDeletion
	order []deletionKey // oldest first
}

func (c *deletionResponses) get(k deletionKey, now time.Time) (message.Message, bool) {
	d, ok := c.byKey[k]
	if !ok || now.Sub(d.at) >= deletionResponseRetention {
		return nil, false
	}

	return d.rsp, true
}

func (c *deletionResponses) put(k deletionKey, rsp message.Message, now time.Time) {
	c.expire(now)

	if c.byKey == nil {
		c.byKey = make(map[deletionKey]cachedDeletion)
	}

	if _, held := c.byKey[k]; !held {
		c.order = append(c.order, k)
	}

	c.byKey[k] = cachedDeletion{rsp, now}
}

// expire drops the responses kept for the retention period.
func (c *deletionResponses) expire(now time.Time) {
	n := 0
	for _, k := range c.order {
		if now.Sub(c.byKey[k].at) < deletionResponseRetention {
			break
		}

		delete(c.byKey, k)
		n++
	}

	c.order = c.order[n:]
}
