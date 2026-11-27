// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// strictLiBlock decodes the `li` object on its own, refusing a key this element does not model
// and a value of the wrong type. It returns nil when the document has no `li` object, or a null
// one: interception is off. Otherwise it returns the object as far as it decoded, with the
// refusal, if any, beside it.
//
// **A misspelled LI key is silently dropped by encoding/json, and the defaults it lands on are the
// dangerous ones.** A typo'd `keepalive_retain_tasking` or `keepalive_timeout` leaves the X1
// keepalive procedure on a purge policy or timers the operator did not choose, so this CC-POI may
// keep duplicating content after the triggering function responsible for it is gone for longer
// than intended. A typo'd `admf_url` leaves the fault channel a no-op, so
// every condition this element is required to report goes nowhere, including the report that would
// have said the configuration was wrong. In both cases the operator wrote the setting, the element
// ignored it, and nothing anywhere says so.
//
// `validateLiConfig` already refuses an *absent* mandatory key. It cannot see a misspelled one:
// from its side the two are identical, and the settings above are the ones where the difference
// matters most, because neither is mandatory and both fail unsafely.
//
// **Scoped to the `li` object, deliberately.** Passing DisallowUnknownFields over the whole
// configuration would refuse keys this version does not model, which is a different and much
// larger change: configuration keys are added over time, and a deployment carrying a newer one
// would stop starting. The property to hold is narrow — a mistyped LI key must not reach a default — and
// so is the check.
//
// **A refused object is still returned, never nil.** encoding/json records the first unknown
// key or type mismatch and decodes the rest, so the fields that did decode — admf_url among
// them — let startLIShipper report the refusal. An `li` that is not an object at all decodes
// nothing, and is refused all the same: returning nil for it would read as "interception is
// off" and the refusal would never be acted on.
func strictLiBlock(jsonData []byte) (*LiConfig, error) {
	// Only the top level, so that no key outside `li` is ever presented to the strict decode.
	var top map[string]json.RawMessage
	if err := json.Unmarshal(jsonData, &top); err != nil {
		// A document this cannot read is the real decode's to report, with its own message.
		return nil, nil
	}

	raw, ok, err := liKey(top)
	if err != nil {
		return &LiConfig{}, err
	}

	if !ok || string(raw) == "null" {
		// No `li` object: interception is off, and there is nothing to be strict about.
		return nil, nil
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	li := &LiConfig{}
	if err := dec.Decode(li); err != nil {
		return li, fmt.Errorf("li: %w — a key this element does not recognise, or a value it cannot "+
			"read, is a setting that never reaches it, and the defaults it leaves in place are the "+
			"unsafe ones: an unread keepalive setting leaves this CC-POI holding tasking on terms "+
			"the operator did not choose, and an unread admf_url leaves it with no channel to "+
			"report that on", err)
	}

	return li, nil
}

// liKey finds the `li` object among the document's top-level keys, ignoring case as encoding/json
// does when it matches a key to a field: `"LI"` configures interception, as it would through a
// struct tag, rather than leaving it off with nothing saying so.
//
// More than one spelling is refused, an exact `li` among them included: encoding/json would
// decode each into the one field in document order, but the map has lost that order, and taking
// one silently, or reading an exact `"li": null` and leaving interception off beside an `"LI"`
// object, is the fault this decode exists to stop.
func liKey(top map[string]json.RawMessage) (json.RawMessage, bool, error) {
	var (
		raw   json.RawMessage
		found []string
	)

	for k, v := range top {
		if strings.EqualFold(k, "li") {
			raw = v
			found = append(found, k)
		}
	}

	if len(found) > 1 {
		return nil, false, fmt.Errorf("li: the document has %d keys that differ only in case from \"li\", "+
			"so which one configures this element is ambiguous", len(found))
	}

	return raw, len(found) == 1, nil
}
