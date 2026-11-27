// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"github.com/wmnsk/go-pfcp/ie"
)

// confirmedPush adapts a cause-only push to the confirmation-aware contract, for the tests
// whose subject is the cause rather than the confirmation: whatever they answer as accepted
// counts as confirmed, so an unconfirmed write never arises in them.
func confirmedPush(
	push func(all, updated PacketForwardingRules) uint8,
) func(all, updated PacketForwardingRules) (uint8, bool) {
	return func(all, updated PacketForwardingRules) (uint8, bool) {
		cause := push(all, updated)

		return cause, cause == ie.CauseRequestAccepted
	}
}
