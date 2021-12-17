// SPDX-License-Identifier: Apache-2.0
// Copyright 2021-present Open Networking Foundation

package main

import (
	"fmt"
	p4ConfigV1 "github.com/p4lang/p4runtime/go/p4/config/v1"
	p4 "github.com/p4lang/p4runtime/go/p4/v1"
	log "github.com/sirupsen/logrus"
	"net"
)

// P4 constants
const (
	FieldN3Address        = "n3_address"
	FieldUEAddress        = "ue_address"
	FieldTEID             = "teid"
	FieldQFI              = "qfi"
	FieldCounterIndex     = "ctr_idx"
	FieldTrafficClass     = "tc"
	FieldTunnelPeerID     = "tunnel_peer_id"
	FieldTunnelSrcAddress = "src_addr"
	FieldTunnelDstAddress = "dst_addr"
	FieldTunnelSrcPort    = "sport"

	TableTunnelPeers          = "PreQosPipe.tunnel_peers"
	TableDownlinkTerminations = "PreQosPipe.terminations_downlink"
	TableUplinkTerminations   = "PreQosPipe.terminations_uplink"
	TableDownlinkSessions     = "PreQosPipe.sessions_downlink"
	TableUplinkSessions       = "PreQosPipe.sessions_uplink"

	ActSetUplinkSession       = "PreQosPipe.set_session_uplink"
	ActSetDownlinkSession     = "PreQosPipe.set_session_downlink"
	ActSetDownlinkSessionBuff = "PreQosPipe.set_session_downlink_buff"
	ActUplinkTermDrop         = "PreQosPipe.uplink_term_drop"
	ActUplinkTermFwd          = "PreQosPipe.uplink_term_fwd"
	ActDownlinkTermDrop       = "PreQosPipe.downlink_term_drop"
	ActDownlinkTermFwd        = "PreQosPipe.downlink_term_fwd"
	ActLoadTunnelParams       = "PreQosPipe.load_tunnel_param"

	DefaultPriority = 0
)

type P4rtTranslator struct {
	p4Info p4ConfigV1.P4Info
}

func newP4RtTranslator(p4info p4ConfigV1.P4Info) *P4rtTranslator {
	return &P4rtTranslator{
		p4Info: p4info,
	}
}

func (t *P4rtTranslator) tableID(name string) uint32 {
	for _, table := range t.p4Info.Tables {
		if table.Preamble.Name == name {
			return table.Preamble.Id
		}
	}

	return invalidID
}

func (t *P4rtTranslator) counterID(name string) uint32 {
	for _, counter := range t.p4Info.GetCounters() {
		if counter.Preamble.Name == name {
			return counter.Preamble.Id
		}
	}
	return invalidID
}

func (t *P4rtTranslator) actionID(name string) uint32 {
	for _, action := range t.p4Info.Actions {
		if action.Preamble.Name == name {
			return action.Preamble.Id
		}
	}

	return invalidID
}

func (t *P4rtTranslator) getActionByID(actionID uint32) (*p4ConfigV1.Action, error) {
	for _, action := range t.p4Info.Actions {
		if action.Preamble.Id == actionID {
			return action, nil
		}
	}

	return nil, fmt.Errorf("action with given ID not found")
}

func (t *P4rtTranslator) getTableByID(tableID uint32) (*p4ConfigV1.Table, error) {
	for _, table := range t.p4Info.Tables {
		if table.Preamble.Id == tableID {
			return table, nil
		}
	}

	return nil, fmt.Errorf("table with ID %v not found", tableID)
}

func (t *P4rtTranslator) getTableIDByName(name string) (uint32, error) {
	for _, table := range t.p4Info.Tables {
		if table.Preamble.Name == name {
			return table.Preamble.Id, nil
		}
	}

	return 0, fmt.Errorf("table with name %v not found", name)
}

func (t *P4rtTranslator) getCounterByName(name string) (*p4ConfigV1.Counter, error) {
	for _, ctr := range t.p4Info.Counters {
		if ctr.Preamble.Name == name {
			return ctr, nil
		}
	}

	return nil, fmt.Errorf("counter with name %v not found", name)
}

func (t *P4rtTranslator) getMatchFieldIDByName(table *p4ConfigV1.Table, fieldName string) uint32 {
	for _, field := range table.MatchFields {
		if field.Name == fieldName {
			return field.Id
		}
	}

	return invalidID
}

func (t *P4rtTranslator) getMatchFieldByName(table *p4ConfigV1.Table, fieldName string) *p4ConfigV1.MatchField {
	for _, field := range table.MatchFields {
		if field.Name == fieldName {
			return field
		}
	}

	return nil
}

func (t *P4rtTranslator) getMatchFieldSizeByName(table *p4ConfigV1.Table, fieldName string) int32 {
	for _, field := range table.MatchFields {
		if field.Name == fieldName {
			return field.Bitwidth / 8
		}
	}

	return invalidID
}

func (t *P4rtTranslator) getActionParamByName(action *p4ConfigV1.Action, paramName string) *p4ConfigV1.Action_Param {
	for _, param := range action.Params {
		if param.Name == paramName {
			return param
		}
	}

	return nil
}

func (t *P4rtTranslator) getEnumVal(enumName string, valName string) ([]byte, error) {
	enumVal, ok := t.p4Info.TypeInfo.SerializableEnums[enumName]
	if !ok {
		err := fmt.Errorf("enum not found with name %s", enumName)
		return nil, err
	}

	for _, enums := range enumVal.Members {
		if enums.Name == valName {
			return enums.Value, nil
		}
	}

	return nil, fmt.Errorf("EnumVal not found")
}

// TODO: find a way to use *p4.TableEntry as receiver
func (t *P4rtTranslator) withExactMatchField(entry *p4.TableEntry, name string, value interface{}) error {
	if entry.TableId == 0 {
		return fmt.Errorf("no table name for entry defined, set table name before adding match fields")
	}

	p4Table, err := t.getTableByID(entry.TableId)
	if err != nil {
		return err
	}

	p4MatchField := t.getMatchFieldByName(p4Table, name)
	if p4MatchField == nil {
		return fmt.Errorf("failed to find match field name: %s", name)
	}

	matchField := &p4.FieldMatch{
		FieldId: p4MatchField.Id,
	}

	byteVal, err := ConvertValueToBinary(value)
	if err != nil {
		return err
	}

	exactMatch := &p4.FieldMatch_Exact{
		Value: byteVal,
	}
	matchField.FieldMatchType = &p4.FieldMatch_Exact_{Exact: exactMatch}

	entry.Match = append(entry.Match, matchField)
	return nil
}

func (t *P4rtTranslator) withTernaryMatchField(entry *p4.TableEntry, name string, value interface{}, mask interface{}) error {
	ternaryFieldLog := log.WithFields(log.Fields{
		"entry":      entry.String(),
		"field name": name,
	})
	ternaryFieldLog.Trace("Adding ternary match field to the entry")

	if entry.TableId == 0 {
		return fmt.Errorf("no table name for entry defined, set table name before adding match fields")
	}

	byteVal, err := ConvertValueToBinary(value)
	if err != nil {
		return err
	}

	byteMask, err := ConvertValueToBinary(mask)
	if err != nil {
		return err
	}

	if len(byteVal) != len(byteMask) {
		ternaryFieldLog.Trace("value and mask length mismatch")
		return fmt.Errorf("value and mask length mismatch for ternary field: %s", name)
	}

	p4Table, err := t.getTableByID(entry.TableId)
	if err != nil {
		return err
	}

	p4MatchField := t.getMatchFieldByName(p4Table, name)
	if p4MatchField == nil {
		return fmt.Errorf("failed to find match field name: %s", name)
	}

	matchField := &p4.FieldMatch{
		FieldId: p4MatchField.Id,
	}

	ternaryMatch := &p4.FieldMatch_Ternary{
		Value: byteVal,
		Mask:  byteMask,
	}

	matchField.FieldMatchType = &p4.FieldMatch_Ternary_{Ternary: ternaryMatch}

	entry.Match = append(entry.Match, matchField)
	return nil
}

func (t *P4rtTranslator) withActionParam(action *p4.Action, name string, value interface{}) error {
	if action.ActionId == 0 {
		return fmt.Errorf("no action ID defined, set action ID before adding action parameters")
	}

	byteVal, err := ConvertValueToBinary(value)
	if err != nil {
		return err
	}

	p4Action, err := t.getActionByID(action.ActionId)
	if err != nil {
		return err
	}

	p4ActionParam := t.getActionParamByName(p4Action, name)
	if p4ActionParam == nil {
		return fmt.Errorf("failed to find action param name %s", name)
	}

	param := &p4.Action_Param{
		ParamId: p4ActionParam.Id,
		Value:   byteVal,
	}

	action.Params = append(action.Params, param)

	return nil
}

func (t *P4rtTranslator) getLPMMatchFieldValue(tableEntry *p4.TableEntry, name string) (*net.IPNet, error) {
	tableID := tableEntry.TableId

	p4Table, err := t.getTableByID(tableID)
	if err != nil {
		return nil, err
	}

	p4MatchFieldID := t.getMatchFieldIDByName(p4Table, name)

	for _, mf := range tableEntry.Match {
		if mf.FieldId == p4MatchFieldID {
			lpmField := mf.GetLpm()
			if lpmField == nil {
				return nil, fmt.Errorf("trying to get LPM value for non-LPM match field")
			}
			ipNet := &net.IPNet{}
			ipNet.IP = make([]byte, 4)
			copy(ipNet.IP, lpmField.Value)
			ipNet.Mask = net.CIDRMask(int(lpmField.PrefixLen), 32-int(lpmField.PrefixLen))

			return ipNet, nil
		}
	}

	return nil, fmt.Errorf("match field %s not found for table %s", name, p4Table.Preamble.Name)
}

func (t *P4rtTranslator) BuildInterfaceTableEntry(srcIntf string, direction string) (*p4.TableEntry, error) {
	tableID := t.tableID("PreQosPipe.source_iface_lookup")

	entry := &p4.TableEntry{
		TableId: tableID,
		// FIXME: we might want to configure priority
		Priority: DefaultPriority,
	}

	// FIXME: complete
	return entry, nil
}

func (t *P4rtTranslator) ParseAccessIPFromReadInterfaceTableResponse(resp *p4.ReadResponse) (*net.IPNet, error) {
	for _, entity := range resp.GetEntities() {
		field, err := t.getLPMMatchFieldValue(entity.GetTableEntry(), "ipv4_dst_prefix")
		if err != nil {
			log.WithFields(log.Fields{
				"entity": entity,
			}).Warn("failed to get LPM field from response entity")
		}

		return field, nil
	}

	return nil, fmt.Errorf("failed to parse Access IP from P4Runtime response")
}

func (t *P4rtTranslator) buildUplinkSessionsEntry(pdr pdr) (*p4.TableEntry, error) {
	uplinkBuilderLog := log.WithFields(log.Fields{
		"pdr": pdr,
	})
	uplinkBuilderLog.Trace("Building P4rt table entry for sessions_uplink table")

	tableID := t.tableID(TableUplinkSessions)

	entry := &p4.TableEntry{
		TableId:  tableID,
		Priority: DefaultPriority,
	}

	if err := t.withExactMatchField(entry, FieldN3Address, pdr.tunnelIP4Dst); err != nil {
		return nil, err
	}

	if err := t.withTernaryMatchField(entry, FieldTEID, pdr.tunnelTEID, pdr.tunnelTEIDMask); err != nil {
		return nil, err
	}

	action := &p4.Action{
		ActionId: t.actionID(ActSetUplinkSession),
	}

	entry.Action = &p4.TableAction{
		Type: &p4.TableAction_Action{Action: action},
	}

	uplinkBuilderLog.WithField("entry", entry).Trace("Built P4rt table entry for sessions_uplink table")
	return entry, nil
}

func (t *P4rtTranslator) buildDownlinkSessionsEntry(pdr pdr, tunnelPeerID uint8, needsBuffering bool) (*p4.TableEntry, error) {
	builderLog := log.WithFields(log.Fields{
		"pdr":            pdr,
		"tunnelPeerID":   tunnelPeerID,
		"needsBuffering": needsBuffering,
	})
	builderLog.Trace("Building P4rt table entry for sessions_downlink table")

	tableID := t.tableID(TableDownlinkSessions)
	entry := &p4.TableEntry{
		TableId:  tableID,
		Priority: DefaultPriority,
	}

	if err := t.withExactMatchField(entry, FieldUEAddress, pdr.dstIP); err != nil {
		return nil, err
	}

	var action *p4.Action
	if needsBuffering {
		action = &p4.Action{
			ActionId: t.actionID(ActSetDownlinkSessionBuff),
		}
	} else {
		action = &p4.Action{
			ActionId: t.actionID(ActSetDownlinkSession),
		}
		if err := t.withActionParam(action, FieldTunnelPeerID, tunnelPeerID); err != nil {
			return nil, err
		}
	}

	entry.Action = &p4.TableAction{
		Type: &p4.TableAction_Action{Action: action},
	}

	builderLog.WithField("entry", entry).Trace("Built P4rt table entry for sessions_downlink table")
	return entry, nil
}

func (t *P4rtTranslator) BuildSessionsTableEntry(pdr pdr, tunnelPeerID uint8, needsBuffering bool) (*p4.TableEntry, error) {
	switch pdr.srcIface {
	case access:
		return t.buildUplinkSessionsEntry(pdr)
	case core:
		return t.buildDownlinkSessionsEntry(pdr, tunnelPeerID, needsBuffering)
	default:
		return nil, fmt.Errorf("unknown source interface type of PDR: %v", pdr.srcIface)
	}
}

func (t *P4rtTranslator) buildUplinkTerminationsEntry(pdr pdr, shouldDrop bool, tc uint8) (*p4.TableEntry, error) {
	builderLog := log.WithFields(log.Fields{
		"pdr": pdr,
		"tc":  tc,
	})
	builderLog.Debug("Building P4rt table entry for UP4 terminations_uplink table")

	tableID := t.tableID(TableUplinkTerminations)
	entry := &p4.TableEntry{
		TableId:  tableID,
		Priority: DefaultPriority,
	}

	if err := t.withExactMatchField(entry, FieldUEAddress, pdr.srcIP); err != nil {
		return nil, err
	}

	var action *p4.Action
	if shouldDrop {
		action = &p4.Action{
			ActionId: t.actionID(ActUplinkTermDrop),
		}
	} else {
		action = &p4.Action{
			ActionId: t.actionID(ActUplinkTermFwd),
		}

		if err := t.withActionParam(action, FieldTrafficClass, tc); err != nil {
			return nil, err
		}
	}

	if err := t.withActionParam(action, FieldCounterIndex, pdr.ctrID); err != nil {
		return nil, err
	}

	entry.Action = &p4.TableAction{
		Type: &p4.TableAction_Action{Action: action},
	}

	builderLog.WithField("entry", entry).Debug("Built P4rt table entry for terminations_uplink table")
	return entry, nil
}

func (t *P4rtTranslator) buildDownlinkTerminationsEntry(pdr pdr, relatedFAR far, tc uint8) (*p4.TableEntry, error) {
	builderLog := log.WithFields(log.Fields{
		"pdr":         pdr,
		"tc":          tc,
		"related-far": relatedFAR,
	})
	builderLog.Debug("Building P4rt table entry for UP4 terminations_downlink table")

	tableID := t.tableID(TableDownlinkTerminations)
	entry := &p4.TableEntry{
		TableId:  tableID,
		Priority: DefaultPriority,
	}

	if err := t.withExactMatchField(entry, FieldUEAddress, pdr.dstIP); err != nil {
		return nil, err
	}

	var action *p4.Action
	if relatedFAR.Drops() {
		action = &p4.Action{
			ActionId: t.actionID(ActDownlinkTermDrop),
		}
	} else {
		action = &p4.Action{
			ActionId: t.actionID(ActDownlinkTermFwd),
		}

		if err := t.withActionParam(action, FieldTEID, pdr.tunnelTEID); err != nil {
			return nil, err
		}

		// TODO: add support for QFI, which should be stored as the part of PDR
		if err := t.withActionParam(action, FieldQFI, uint8(0)); err != nil {
			return nil, err
		}

		if err := t.withActionParam(action, FieldTrafficClass, tc); err != nil {
			return nil, err
		}
	}

	if err := t.withActionParam(action, FieldCounterIndex, pdr.ctrID); err != nil {
		return nil, err
	}

	entry.Action = &p4.TableAction{
		Type: &p4.TableAction_Action{Action: action},
	}

	builderLog.WithField("entry", entry).Debug("Built P4rt table entry for terminations_downlink table")
	return entry, nil
}

func (t *P4rtTranslator) BuildTerminationsTableEntry(pdr pdr, relatedFAR far, tc uint8) (*p4.TableEntry, error) {
	switch pdr.srcIface {
	case access:
		return t.buildUplinkTerminationsEntry(pdr, relatedFAR.Drops(), tc)
	case core:
		return t.buildDownlinkTerminationsEntry(pdr, relatedFAR, tc)
	default:
		return nil, fmt.Errorf("unknown source interface type of PDR: %v", pdr.srcIface)
	}
}

func (t *P4rtTranslator) BuildGTPTunnelPeerTableEntry(tunnelPeerID uint8, far far) (*p4.TableEntry, error) {
	builderLog := log.WithFields(log.Fields{
		"tunnelPeerID": tunnelPeerID,
		"far":          far,
	})
	builderLog.Trace("Building P4rt table entry for GTP Tunnel Peers table")

	tableID := t.tableID(TableTunnelPeers)
	entry := &p4.TableEntry{
		TableId:  tableID,
		Priority: DefaultPriority,
		Action: &p4.TableAction{
			Type: &p4.TableAction_Action{
				Action: &p4.Action{
					ActionId: t.actionID(ActLoadTunnelParams),
				},
			},
		},
	}

	if err := t.withExactMatchField(entry, FieldTunnelPeerID, tunnelPeerID); err != nil {
		return nil, err
	}

	if err := t.withActionParam(entry.GetAction().GetAction(), FieldTunnelSrcAddress, far.tunnelIP4Src); err != nil {
		return nil, err
	}

	if err := t.withActionParam(entry.GetAction().GetAction(), FieldTunnelDstAddress, far.tunnelIP4Dst); err != nil {
		return nil, err
	}

	if err := t.withActionParam(entry.GetAction().GetAction(), FieldTunnelSrcPort, far.tunnelPort); err != nil {
		return nil, err
	}

	builderLog.WithField("entry", entry).Debug("Built P4rt table entry for GTP Tunnel Peers table")
	return entry, nil
}
