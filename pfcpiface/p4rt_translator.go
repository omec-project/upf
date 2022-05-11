// SPDX-License-Identifier: Apache-2.0
// Copyright 2021-present Open Networking Foundation

package pfcpiface

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/bits"
	"net"

	"github.com/omec-project/upf-epc/internal/p4constants"
	p4ConfigV1 "github.com/p4lang/p4runtime/go/p4/config/v1"
	p4 "github.com/p4lang/p4runtime/go/p4/v1"
	log "github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"
)

// P4 constants
const (
	DirectionUplink   = 1
	DirectionDownlink = 2

	FieldAppIPProto        = "app_ip_proto"
	FieldAppL4Port         = "app_l4_port"
	FieldAppIPAddress      = "app_ip_addr"
	FieldIPv4DstPrefix     = "ipv4_dst_prefix"
	FieldN3Address         = "n3_address"
	FieldUEAddress         = "ue_address"
	FieldApplicationID     = "app_id"
	FieldTEID              = "teid"
	FieldQFI               = "qfi"
	FieldCounterIndex      = "ctr_idx"
	FieldTrafficClass      = "tc"
	FieldTunnelPeerID      = "tunnel_peer_id"
	FieldTunnelSrcAddress  = "src_addr"
	FieldTunnelDstAddress  = "dst_addr"
	FieldTunnelSrcPort     = "sport"
	FieldSrcIface          = "src_iface"
	FieldDirection         = "direction"
	FieldSliceID           = "slice_id"
	FieldSessionMeterIndex = "session_meter_idx"
	FieldAppMeterIndex     = "app_meter_idx"

	DefaultPriority      = 0
	DefaultApplicationID = 0
)

type tunnelParams struct {
	tunnelIP4Src uint32
	tunnelIP4Dst uint32
	tunnelPort   uint16
}

type P4rtTranslator struct {
	p4Info *p4ConfigV1.P4Info
}

func newP4RtTranslator(p4info *p4ConfigV1.P4Info) *P4rtTranslator {
	return &P4rtTranslator{
		p4Info: p4info,
	}
}

func convertValueToBinary(value interface{}) ([]byte, error) {
	switch t := value.(type) {
	case []byte:
		return value.([]byte), nil
	case bool:
		uintFlag := uint8(0)

		flag := value.(bool)
		if flag {
			uintFlag = 1
		}

		b := make([]byte, 1)
		b[0] = uintFlag

		return b, nil
	case uint8:
		b := make([]byte, 1)
		b[0] = value.(uint8)

		return b, nil
	case uint16:
		b := make([]byte, 2)
		binary.BigEndian.PutUint16(b, value.(uint16))

		return b, nil
	case uint32:
		b := make([]byte, 4)
		binary.BigEndian.PutUint32(b, value.(uint32))

		return b, nil
	case uint64:
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, value.(uint64))

		return b, nil
	case int:
		b := make([]byte, 4)
		binary.BigEndian.PutUint32(b, uint32(value.(int)))

		return b, nil
	default:
		log.Debugf("Type %T", t)
		return nil, ErrOperationFailedWithParam("convert type to byte array", "type", t)
	}
}

func (t *P4rtTranslator) getActionByID(actionID uint32) (*p4ConfigV1.Action, error) {
	for _, action := range t.p4Info.Actions {
		if action.Preamble.Id == actionID {
			return action, nil
		}
	}

	return nil, ErrNotFoundWithParam("action", "ID", actionID)
}

func (t *P4rtTranslator) getMeterSizeByID(meterID uint32) (int64, error) {
	for _, meter := range t.p4Info.Meters {
		if meter.Preamble.Id == meterID {
			return meter.GetSize(), nil
		}
	}

	return 0, ErrNotFoundWithParam("meter", "ID", meterID)
}

func (t *P4rtTranslator) getCounterSizeByID(counterID uint32) (int64, error) {
	for _, counter := range t.p4Info.Counters {
		if counter.Preamble.Id == counterID {
			return counter.GetSize(), nil
		}
	}

	return 0, ErrNotFoundWithParam("counter", "ID", counterID)
}

func (t *P4rtTranslator) getTableByID(tableID uint32) (*p4ConfigV1.Table, error) {
	for _, table := range t.p4Info.Tables {
		if table.Preamble.Id == tableID {
			return table, nil
		}
	}

	return nil, ErrNotFoundWithParam("table", "ID", tableID)
}

//nolint:unused
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

func (t *P4rtTranslator) getActionParamByName(action *p4ConfigV1.Action, paramName string) *p4ConfigV1.Action_Param {
	for _, param := range action.Params {
		if param.Name == paramName {
			return param
		}
	}

	return nil
}

// TODO: find a way to use *p4.TableEntry as receiver
func (t *P4rtTranslator) withExactMatchField(entry *p4.TableEntry, name string, value interface{}) error {
	if entry.TableId == 0 {
		return ErrInvalidArgumentWithReason("entry.TableId", entry.TableId, "no table name for entry defined, set table name before adding match fields")
	}

	p4Table, err := t.getTableByID(entry.TableId)
	if err != nil {
		return err
	}

	p4MatchField := t.getMatchFieldByName(p4Table, name)
	if p4MatchField == nil {
		return ErrOperationFailedWithParam("find match field", "name", name)
	}

	matchField := &p4.FieldMatch{
		FieldId: p4MatchField.Id,
	}

	byteVal, err := convertValueToBinary(value)
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

func (t *P4rtTranslator) withLPMField(entry *p4.TableEntry, name string, value uint32, prefixLen uint8) error {
	lpmFieldLog := log.WithFields(log.Fields{
		"entry":      entry.String(),
		"field name": name,
	})
	lpmFieldLog.Trace("Adding LPM match field to the entry")

	if entry.TableId == 0 {
		return ErrInvalidArgumentWithReason("entry.TableId", entry.TableId, "no table ID for entry defined, set table ID before adding match fields")
	}

	p4Table, err := t.getTableByID(entry.TableId)
	if err != nil {
		return err
	}

	p4MatchField := t.getMatchFieldByName(p4Table, name)
	if p4MatchField == nil {
		return ErrOperationFailedWithParam("find match field", "name", name)
	}

	byteVal, err := convertValueToBinary(value)
	if err != nil {
		return err
	}

	matchField := &p4.FieldMatch{
		FieldId: p4MatchField.Id,
	}

	lpmMatch := &p4.FieldMatch_LPM{
		Value:     byteVal,
		PrefixLen: int32(prefixLen),
	}

	matchField.FieldMatchType = &p4.FieldMatch_Lpm{Lpm: lpmMatch}

	entry.Match = append(entry.Match, matchField)

	return nil
}

func (t *P4rtTranslator) withRangeMatchField(entry *p4.TableEntry, name string, low interface{}, high interface{}) error {
	rangeFieldLog := log.WithFields(log.Fields{
		"entry":      entry.String(),
		"field name": name,
	})
	rangeFieldLog.Trace("Adding range match field to the entry")

	if entry.TableId == 0 {
		return ErrInvalidArgumentWithReason("entry.TableId", entry.TableId, "no table ID for entry defined, set table ID before adding match fields")
	}

	lowByteVal, err := convertValueToBinary(low)
	if err != nil {
		return err
	}

	highByteVal, err := convertValueToBinary(high)
	if err != nil {
		return err
	}

	p4Table, err := t.getTableByID(entry.TableId)
	if err != nil {
		return err
	}

	p4MatchField := t.getMatchFieldByName(p4Table, name)
	if p4MatchField == nil {
		return ErrOperationFailedWithParam("find match field", "name", name)
	}

	matchField := &p4.FieldMatch{
		FieldId: p4MatchField.Id,
	}

	rangeMatch := &p4.FieldMatch_Range{
		Low:  lowByteVal,
		High: highByteVal,
	}

	matchField.FieldMatchType = &p4.FieldMatch_Range_{Range: rangeMatch}

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
		return ErrInvalidArgumentWithReason("entry.TableId", entry.TableId, "no table name for entry defined, set table name before adding match fields")
	}

	byteVal, err := convertValueToBinary(value)
	if err != nil {
		return err
	}

	byteMask, err := convertValueToBinary(mask)
	if err != nil {
		return err
	}

	if len(byteVal) != len(byteMask) {
		ternaryFieldLog.Trace("value and mask length mismatch")
		return ErrOperationFailedWithParam("value and mask length mismatch for ternary field", "field", name)
	}

	p4Table, err := t.getTableByID(entry.TableId)
	if err != nil {
		return err
	}

	p4MatchField := t.getMatchFieldByName(p4Table, name)
	if p4MatchField == nil {
		return ErrOperationFailedWithParam("find match field", "name", name)
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
		return ErrInvalidArgumentWithReason("entry.ActionId", action.ActionId,
			"invalid action ID defined, set action ID before adding action parameters")
	}

	byteVal, err := convertValueToBinary(value)
	if err != nil {
		return err
	}

	p4Action, err := t.getActionByID(action.ActionId)
	if err != nil {
		return err
	}

	p4ActionParam := t.getActionParamByName(p4Action, name)
	if p4ActionParam == nil {
		return ErrOperationFailedWithParam("find action param", "action param name", name)
	}

	param := &p4.Action_Param{
		ParamId: p4ActionParam.Id,
		Value:   byteVal,
	}

	action.Params = append(action.Params, param)

	return nil
}

//nolint:unused
func (t *P4rtTranslator) getActionParamValue(tableEntry *p4.TableEntry, id uint32) ([]byte, error) {
	for _, param := range tableEntry.Action.GetAction().Params {
		if param.ParamId == id {
			return param.Value, nil
		}
	}

	return nil, ErrNotFoundWithParam("action param", "id", id)
}

//nolint:unused
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
				return nil, ErrOperationFailedWithReason("getting LPM match field value",
					"trying to get LPM value for non-LPM match field")
			}

			ipNet := &net.IPNet{
				IP:   make(net.IP, net.IPv4len),
				Mask: net.CIDRMask(int(lpmField.PrefixLen), 8*net.IPv4len),
			}
			copy(ipNet.IP, lpmField.Value[:])

			return ipNet, nil
		}
	}

	return nil, ErrNotFoundWithParam(fmt.Sprintf("match field %s", name), "table", p4Table.Preamble.Name)
}

func (t *P4rtTranslator) BuildInterfaceTableEntryNoAction() *p4.TableEntry {
	entry := &p4.TableEntry{
		TableId:  p4constants.TablePreQosPipeInterfaces,
		Priority: DefaultPriority,
	}

	return entry
}

func (t *P4rtTranslator) BuildInterfaceTableEntry(ipNet *net.IPNet, sliceID uint8, isCore bool) (*p4.TableEntry, error) {
	srcIface := access
	direction := DirectionUplink

	if isCore {
		srcIface = core
		direction = DirectionDownlink
	}

	entry := &p4.TableEntry{
		TableId:  p4constants.TablePreQosPipeInterfaces,
		Priority: DefaultPriority,
	}

	maskLength, _ := ipNet.Mask.Size()
	if err := t.withLPMField(entry, FieldIPv4DstPrefix, ip2int(ipNet.IP.To4()), uint8(maskLength)); err != nil {
		return nil, err
	}

	action := &p4.Action{
		ActionId: p4constants.ActionPreQosPipeSetSourceIface,
	}

	if err := t.withActionParam(action, FieldSrcIface, srcIface); err != nil {
		return nil, err
	}

	if err := t.withActionParam(action, FieldDirection, direction); err != nil {
		return nil, err
	}

	if err := t.withActionParam(action, FieldSliceID, sliceID); err != nil {
		return nil, err
	}

	entry.Action = &p4.TableAction{
		Type: &p4.TableAction_Action{Action: action},
	}

	return entry, nil
}

func (t *P4rtTranslator) BuildApplicationsTableEntry(pdr pdr, sliceID uint8, internalAppID uint8) (*p4.TableEntry, error) {
	applicationsBuilderLog := log.WithFields(log.Fields{
		"pdr": pdr,
	})
	applicationsBuilderLog.Trace("Building P4rt table entry for applications table")

	entry := &p4.TableEntry{
		TableId: p4constants.TablePreQosPipeApplications,
		// priority for UP4 cannot be greater than 65535
		Priority: int32(math.MaxUint16 - pdr.precedence),
	}

	var (
		appIP, appIPMask uint32 = 0, 0
		appPort          portRange
	)

	if pdr.srcIface == access {
		appIP, appIPMask = pdr.appFilter.dstIP, pdr.appFilter.dstIPMask
		appPort = pdr.appFilter.dstPortRange
	} else if pdr.srcIface == core {
		appIP, appIPMask = pdr.appFilter.srcIP, pdr.appFilter.srcIPMask
		appPort = pdr.appFilter.srcPortRange
	}

	appProto, appProtoMask := pdr.appFilter.proto, pdr.appFilter.protoMask

	if err := t.withExactMatchField(entry, FieldSliceID, sliceID); err != nil {
		return nil, err
	}

	appIPPrefixLen := 32 - bits.TrailingZeros32(appIPMask)
	if appIPPrefixLen > 0 {
		if err := t.withLPMField(entry, FieldAppIPAddress, appIP, uint8(appIPPrefixLen)); err != nil {
			return nil, err
		}
	}

	if !appPort.isWildcardMatch() {
		if err := t.withRangeMatchField(entry, FieldAppL4Port, appPort.low, appPort.high); err != nil {
			return nil, err
		}
	}

	if appProto != 0 && appProtoMask != 0 {
		if err := t.withTernaryMatchField(entry, FieldAppIPProto, appProto, appProtoMask); err != nil {
			return nil, err
		}
	}

	action := &p4.Action{
		ActionId: p4constants.ActionPreQosPipeSetAppId,
	}

	if err := t.withActionParam(action, FieldApplicationID, internalAppID); err != nil {
		return nil, err
	}

	entry.Action = &p4.TableAction{
		Type: &p4.TableAction_Action{Action: action},
	}

	applicationsBuilderLog = applicationsBuilderLog.WithField("entry", entry)
	applicationsBuilderLog.Trace("Built P4rt table entry for applications table")

	return entry, nil
}

func (t *P4rtTranslator) buildUplinkSessionsEntry(pdr pdr, sessMeterIdx uint32) (*p4.TableEntry, error) {
	uplinkBuilderLog := log.WithFields(log.Fields{
		"pdr":               pdr,
		"sessionMeterIndex": sessMeterIdx,
	})
	uplinkBuilderLog.Trace("Building P4rt table entry for sessions_uplink table")

	entry := &p4.TableEntry{
		TableId:  p4constants.TablePreQosPipeSessionsUplink,
		Priority: DefaultPriority,
	}

	if err := t.withExactMatchField(entry, FieldN3Address, pdr.tunnelIP4Dst); err != nil {
		return nil, err
	}

	if err := t.withExactMatchField(entry, FieldTEID, pdr.tunnelTEID); err != nil {
		return nil, err
	}

	action := &p4.Action{
		ActionId: p4constants.ActionPreQosPipeSetSessionUplink,
	}

	if err := t.withActionParam(action, FieldSessionMeterIndex, sessMeterIdx); err != nil {
		return nil, err
	}

	entry.Action = &p4.TableAction{
		Type: &p4.TableAction_Action{Action: action},
	}

	uplinkBuilderLog.WithField("entry", entry).Trace("Built P4rt table entry for sessions_uplink table")

	return entry, nil
}

func (t *P4rtTranslator) buildDownlinkSessionsEntry(pdr pdr, sessMeterIdx uint32, tunnelPeerID uint8, needsBuffering bool) (*p4.TableEntry, error) {
	builderLog := log.WithFields(log.Fields{
		"pdr":               pdr,
		"sessionMeterIndex": sessMeterIdx,
		"tunnelPeerID":      tunnelPeerID,
		"needsBuffering":    needsBuffering,
	})
	builderLog.Trace("Building P4rt table entry for sessions_downlink table")

	entry := &p4.TableEntry{
		TableId:  p4constants.TablePreQosPipeSessionsDownlink,
		Priority: DefaultPriority,
	}

	if err := t.withExactMatchField(entry, FieldUEAddress, pdr.ueAddress); err != nil {
		return nil, err
	}

	var action *p4.Action
	if needsBuffering {
		action = &p4.Action{
			ActionId: p4constants.ActionPreQosPipeSetSessionDownlinkBuff,
		}
	} else {
		action = &p4.Action{
			ActionId: p4constants.ActionPreQosPipeSetSessionDownlink,
		}
		if err := t.withActionParam(action, FieldTunnelPeerID, tunnelPeerID); err != nil {
			return nil, err
		}
	}

	if err := t.withActionParam(action, FieldSessionMeterIndex, sessMeterIdx); err != nil {
		return nil, err
	}

	entry.Action = &p4.TableAction{
		Type: &p4.TableAction_Action{Action: action},
	}

	builderLog.WithField("entry", entry).Trace("Built P4rt table entry for sessions_downlink table")

	return entry, nil
}

func (t *P4rtTranslator) BuildSessionsTableEntry(pdr pdr, sessionMeter meter, tunnelPeerID uint8, needsBuffering bool) (*p4.TableEntry, error) {
	switch pdr.srcIface {
	case access:
		return t.buildUplinkSessionsEntry(pdr, sessionMeter.uplinkCellID)
	case core:
		return t.buildDownlinkSessionsEntry(pdr, sessionMeter.downlinkCellID, tunnelPeerID, needsBuffering)
	default:
		return nil, ErrUnsupported("source interface type of PDR", pdr.srcIface)
	}
}

func (t *P4rtTranslator) buildUplinkTerminationsEntry(pdr pdr, appMeterIdx uint32, shouldDrop bool, internalAppID uint8, tc uint8, relatedQER qer) (*p4.TableEntry, error) {
	builderLog := log.WithFields(log.Fields{
		"pdr":           pdr,
		"appMeterIndex": appMeterIdx,
		"tc":            tc,
	})
	builderLog.Debug("Building P4rt table entry for UP4 terminations_uplink table")

	entry := &p4.TableEntry{
		TableId:  p4constants.TablePreQosPipeTerminationsUplink,
		Priority: DefaultPriority,
	}

	// QER gating
	if relatedQER.ulStatus == ie.GateStatusClosed {
		shouldDrop = true
	}

	if err := t.withExactMatchField(entry, FieldUEAddress, pdr.ueAddress); err != nil {
		return nil, err
	}

	if err := t.withExactMatchField(entry, FieldApplicationID, internalAppID); err != nil {
		return nil, err
	}

	var action *p4.Action
	if shouldDrop {
		action = &p4.Action{
			ActionId: p4constants.ActionPreQosPipeUplinkTermDrop,
		}
	} else {
		action = &p4.Action{
			ActionId: p4constants.ActionPreQosPipeUplinkTermFwd,
		}

		if err := t.withActionParam(action, FieldTrafficClass, tc); err != nil {
			return nil, err
		}
		if err := t.withActionParam(action, FieldAppMeterIndex, appMeterIdx); err != nil {
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

func (t *P4rtTranslator) buildDownlinkTerminationsEntry(pdr pdr, appMeterIdx uint32, relatedFAR far,
	internalAppID uint8, qfi uint8, tc uint8, relatedQER qer) (*p4.TableEntry, error) {
	builderLog := log.WithFields(log.Fields{
		"pdr":           pdr,
		"appMeterIndex": appMeterIdx,
		"tc":            tc,
		"related-far":   relatedFAR,
	})
	builderLog.Debug("Building P4rt table entry for UP4 terminations_downlink table")

	entry := &p4.TableEntry{
		TableId:  p4constants.TablePreQosPipeTerminationsDownlink,
		Priority: DefaultPriority,
	}

	shouldDrop := false
	if relatedFAR.Drops() || relatedQER.dlStatus == ie.GateStatusClosed {
		shouldDrop = true
	}

	if err := t.withExactMatchField(entry, FieldUEAddress, pdr.ueAddress); err != nil {
		return nil, err
	}

	if err := t.withExactMatchField(entry, FieldApplicationID, internalAppID); err != nil {
		return nil, err
	}

	var action *p4.Action
	if shouldDrop {
		action = &p4.Action{
			ActionId: p4constants.ActionPreQosPipeDownlinkTermDrop,
		}
	} else {
		action = &p4.Action{
			ActionId: p4constants.ActionPreQosPipeDownlinkTermFwd,
		}

		if err := t.withActionParam(action, FieldTEID, relatedFAR.tunnelTEID); err != nil {
			return nil, err
		}

		if err := t.withActionParam(action, FieldQFI, qfi); err != nil {
			return nil, err
		}

		if err := t.withActionParam(action, FieldTrafficClass, tc); err != nil {
			return nil, err
		}

		if err := t.withActionParam(action, FieldAppMeterIndex, appMeterIdx); err != nil {
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

func (t *P4rtTranslator) BuildTerminationsTableEntry(pdr pdr, appMeter meter, relatedFAR far, internalAppID uint8, qfi uint8, tc uint8, relatedQER qer) (*p4.TableEntry, error) {
	switch pdr.srcIface {
	case access:
		return t.buildUplinkTerminationsEntry(pdr, appMeter.uplinkCellID, relatedFAR.Drops(), internalAppID, tc, relatedQER)
	case core:
		return t.buildDownlinkTerminationsEntry(pdr, appMeter.downlinkCellID, relatedFAR, internalAppID, qfi, tc, relatedQER)
	default:
		return nil, ErrUnsupported("source interface type of PDR", pdr.srcIface)
	}
}

func (t *P4rtTranslator) BuildGTPTunnelPeerTableEntry(tunnelPeerID uint8, tunnelParams tunnelParams) (*p4.TableEntry, error) {
	builderLog := log.WithFields(log.Fields{
		"tunnelPeerID":  tunnelPeerID,
		"tunnel-params": tunnelParams,
	})
	builderLog.Trace("Building P4rt table entry for GTP Tunnel Peers table")

	entry := &p4.TableEntry{
		TableId:  p4constants.TablePreQosPipeTunnelPeers,
		Priority: DefaultPriority,
		Action: &p4.TableAction{
			Type: &p4.TableAction_Action{
				Action: &p4.Action{
					ActionId: p4constants.ActionPreQosPipeLoadTunnelParam,
				},
			},
		},
	}

	if err := t.withExactMatchField(entry, FieldTunnelPeerID, tunnelPeerID); err != nil {
		return nil, err
	}

	if err := t.withActionParam(entry.GetAction().GetAction(), FieldTunnelSrcAddress, tunnelParams.tunnelIP4Src); err != nil {
		return nil, err
	}

	if err := t.withActionParam(entry.GetAction().GetAction(), FieldTunnelDstAddress, tunnelParams.tunnelIP4Dst); err != nil {
		return nil, err
	}

	if err := t.withActionParam(entry.GetAction().GetAction(), FieldTunnelSrcPort, tunnelParams.tunnelPort); err != nil {
		return nil, err
	}

	builderLog.WithField("entry", entry).Debug("Built P4rt table entry for GTP Tunnel Peers table")

	return entry, nil
}

func (t *P4rtTranslator) BuildMeterEntry(meterID uint32, cellID uint32, config *p4.MeterConfig) *p4.MeterEntry {
	meterName := p4constants.GetMeterIDToNameMap()[meterID]

	builderLog := log.WithFields(log.Fields{
		"Meter":   meterName,
		"Cell ID": cellID,
	})
	builderLog.Trace("Building Meter entry")

	entry := &p4.MeterEntry{
		MeterId: meterID,
		Index:   &p4.Index{Index: int64(cellID)},
		Config:  config,
	}

	builderLog = builderLog.WithField("meter-entry", entry)
	builderLog.Debug("Meter entry built successfully")

	return entry
}
