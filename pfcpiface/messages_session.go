// SPDX-License-Identifier: Apache-2.0
// Copyright 2021 Intel Corporation

package pfcpiface

import (
	"errors"
	"net"
	"strings"

	"github.com/omec-project/upf-epc/logger"
	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

// errors
var (
	ErrWriteToDatapath = errors.New("write to datapath failed")
	ErrAssocNotFound   = errors.New("no association found for NodeID")
	ErrAllocateSession = errors.New("unable to allocate new PFCP session")
	ErrNodeIDMissing   = errors.New("mandatory Node ID IE missing")
	ErrCPFSEIDMissing  = errors.New("mandatory CPF-SEID IE missing")
	ErrCauseMissing    = errors.New("mandatory Cause IE missing")
)

func (pConn *PFCPConn) handleSessionEstablishmentRequest(msg message.Message) (message.Message, error) {
	upf := pConn.upf

	sereq, ok := msg.(*message.SessionEstablishmentRequest)
	if !ok {
		return nil, errUnmarshal(errMsgUnexpectedType)
	}

	errUnmarshalReply := func(err error, offendingIE *ie.IE) (message.Message, error) {
		// Build response message
		pfdres := message.NewSessionEstablishmentResponse(0,
			0,
			0,
			sereq.SequenceNumber,
			0,
			ie.NewCause(ie.CauseRequestRejected),
			offendingIE,
		)

		return pfdres, errUnmarshal(err)
	}

	if sereq.NodeID == nil {
		logger.PfcpLog.Warnln("Session Establishment Request missing mandatory NodeID IE")
		pfdres := message.NewSessionEstablishmentResponse(0,
			0,
			0,
			sereq.SequenceNumber,
			0,
			ie.NewCause(ie.CauseMandatoryIEMissing),
			ie.NewOffendingIE(ie.NodeID),
		)
		return pfdres, errUnmarshal(ErrNodeIDMissing)
	}

	nodeID, err := sereq.NodeID.NodeID()
	if err != nil {
		return errUnmarshalReply(err, sereq.NodeID)
	}

	/* Read fseid from the IE */
	if sereq.CPFSEID == nil {
		logger.PfcpLog.Warnln("Session Establishment Request without CPF-SEID from nodeID:", nodeID)
		pfdres := message.NewSessionEstablishmentResponse(0,
			0,
			0,
			sereq.SequenceNumber,
			0,
			ie.NewCause(ie.CauseMandatoryIEMissing),
			ie.NewOffendingIE(ie.FSEID),
		)
		return pfdres, errUnmarshal(ErrCPFSEIDMissing)
	}

	fseid, err := sereq.CPFSEID.FSEID()
	if err != nil {
		return errUnmarshalReply(err, sereq.CPFSEID)
	}

	remoteSEID := fseid.SEID
	fseidIP := ip2int(fseid.IPv4Address)

	errProcessReply := func(err error, cause uint8) (message.Message, error) {
		// Build response message
		seres := message.NewSessionEstablishmentResponse(0, /* MO?? <-- what's this */
			0,                    /* FO <-- what's this? */
			remoteSEID,           /* seid */
			sereq.SequenceNumber, /* seq # */
			0,                    /* priority */
			pConn.nodeID.localIE,
			ie.NewCause(cause),
		)

		return seres, errProcess(err)
	}

	if strings.Compare(nodeID, pConn.nodeID.remote) != 0 {
		logger.PfcpLog.Warnln("association not found for Establishment request",
			"with nodeID:", nodeID, ", association NodeID:", pConn.nodeID.remote)
		return errProcessReply(ErrAssocNotFound, ie.CauseNoEstablishedPFCPAssociation)
	}

	session, ok := pConn.NewPFCPSession(remoteSEID)
	if !ok {
		return errProcessReply(ErrAllocateSession,
			ie.CauseNoResourcesAvailable)
	}

	// From here the session holds resources before it is known whether it will exist:
	// NewPFCPSession has counted it, and parsing a CHOOSE PDR allocates the UE address
	// (parse_pdr.go) as a side effect of reading it. A rejected establishment is not
	// stored and the control plane is handed no F-SEID for it, so nothing else will
	// give either back -- not a deletion request, not the association teardown. The one
	// rejection that is stored, a rollback that did not finish, is below. Every way out
	// of this function short of storing the session is a return, and several of
	// them are inside the parse loops below, so the cleanup is deferred rather than
	// repeated.
	stored := false

	defer func() {
		if stored {
			return
		}

		upf.ippool.Release(session.localSEID)
		pConn.RemoveSession(session)
	}()

	addPDRs := make([]pdr, 0, MaxItems)
	addFARs := make([]far, 0, MaxItems)
	addQERs := make([]qer, 0, MaxItems)

	for _, cPDR := range sereq.CreatePDR {
		var p pdr
		if err = p.parsePDR(cPDR, session.localSEID, pConn.appPFDs, upf.ippool); err != nil {
			return errProcessReply(err, ie.CauseRequestRejected)
		}

		if p.UPAllocateFteid {
			var fteid uint32
			fteid, err = pConn.upf.fteidGenerator.Allocate()
			if err != nil {
				return errProcessReply(err, ie.CauseNoResourcesAvailable)
			}
			p.tunnelTEID = fteid
			p.tunnelTEIDMask = 0xFFFFFFFF
			p.tunnelIP4Dst = ip2int(upf.accessIP)
			p.tunnelIP4DstMask = 0xFFFFFFFF
		}

		p.fseidIP = fseidIP
		session.CreatePDR(p)
		addPDRs = append(addPDRs, p)
	}

	for _, cFAR := range sereq.CreateFAR {
		var f far
		if err = f.parseFAR(cFAR, session.localSEID, upf, create); err != nil {
			return errProcessReply(err, ie.CauseRequestRejected)
		}

		f.fseidIP = fseidIP
		session.CreateFAR(f)
		addFARs = append(addFARs, f)
	}

	for _, cQER := range sereq.CreateQER {
		var q qer
		if err = q.parseQER(cQER, session.localSEID); err != nil {
			return errProcessReply(err, ie.CauseRequestRejected)
		}

		q.fseidIP = fseidIP
		session.CreateQER(q)
		addQERs = append(addQERs, q)
	}

	session.MarkSessionQer(session.qers)
	// TODO: since PacketForwardingRules doesn't store pointers,
	//  we must also mark session QERs in addQERs.
	//  We need a kind of refactoring to clean it up.
	session.MarkSessionQer(addQERs)

	// session.PacketForwardingRules stores all PFCP rules that has been installed so far,
	// while 'updated' stores only the PFCP rules that have been provided in this particular message.
	updated := PacketForwardingRules{
		pdrs: addPDRs,
		fars: addFARs,
		qers: addQERs,
	}

	cause := upf.SendMsgToUPF(upfMsgTypeAdd, session.PacketForwardingRules, updated)
	if cause == ie.CauseRequestRejected {
		// The batch reported a failure, which means it completed and some of its rules
		// may be programmed. Take them out before forgetting the session: nothing else
		// will, because the session is not stored on this path unless this removal fails
		// to finish, and the SMF has no F-SEID to release. Best effort -- a datapath that
		// just refused a write may refuse this one too, and there is nothing further to
		// report it to.
		delCause, finished := upf.SendMsgToUPFWithCompletion(
			upfMsgTypeDel, session.PacketForwardingRules, PacketForwardingRules{})
		if !finished {
			// A removal that ran out of time may have left any of the rules the refused
			// batch programmed, and once the session is forgotten nothing names them: the
			// control plane has no F-SEID to delete. So the session is stored after all,
			// holding its rules and -- where this UPF allocates it -- its UE address, and
			// the association teardown removes it like any other. The establishment is
			// still refused.
			logger.PfcpLog.Warnln("the rollback of a rejected session did not finish; " +
				"keeping the session until the association is torn down")

			// Set whether or not the store takes the session. If it refuses -- which it
			// does only for a local SEID of zero -- nothing names the rules either way,
			// and letting the cleanup run would hand the address to another UE while a
			// rule the rollback did not remove may still match it. Holding it is the
			// cheaper failure.
			stored = true

			err = pConn.store.PutSession(session)
			if err != nil {
				logger.PfcpLog.Errorf("failed to put PFCP session to store: %v", err)
			}

			return errProcessReply(ErrWriteToDatapath, ie.CauseRequestRejected)
		}

		if delCause == ie.CauseRequestRejected {
			// A rule that could not be translated (`CreatePortRangeCartesianProduct`)
			// or marshalled was never programmed either, so there is nothing to strand,
			// and that is exactly what happens when the add was refused for the same
			// reason. Still the only thing that reaches here after this change: every
			// non-ENOENT answer the three CommandDelete implementations can give is an
			// EINVAL about argument shape, which is static and would have failed the
			// matching add long before, and none of them can refuse to remove a rule
			// they do hold -- WildcardMatch's DelEntry cannot report failure at all and
			// Qos discards its table's result.
			//
			// If one ever can, the deferred cleanup above stops being safe to run
			// unconditionally on this return: the address would go back to the pool
			// while a PDR still matched it, and no stored session would be left to
			// delete that PDR. The returns before the datapath is reached are not
			// exposed to it -- nothing was programmed for them to strand.
			// Fixing either module to report a refused delete has to come with a way to
			// hold the allocation back here.
			logger.PfcpLog.Warnln("the rollback of a rejected session reported a failure; " +
				"no rule the datapath holds is known to be stranded")
		}

		return errProcessReply(ErrWriteToDatapath,
			ie.CauseRequestRejected)
	}

	stored = true

	err = pConn.store.PutSession(session)
	if err != nil {
		logger.PfcpLog.Errorf("failed to put PFCP session to store: %v", err)
	}

	var localFSEID *ie.IE

	localIP := pConn.LocalAddr().(*net.UDPAddr).IP
	if localIP.To4() != nil {
		localFSEID = ie.NewFSEID(session.localSEID, localIP, nil)
	} else {
		localFSEID = ie.NewFSEID(session.localSEID, nil, localIP)
	}

	// Build response message
	seres := message.NewSessionEstablishmentResponse(0, /* MO?? <-- what's this */
		0,                                    /* FO <-- what's this? */
		session.remoteSEID,                   /* seid */
		sereq.SequenceNumber,                 /* seq # */
		0,                                    /* priority */
		pConn.nodeID.localIE,                 /* node id */
		ie.NewCause(ie.CauseRequestAccepted), /* accept it blindly for the time being */
		localFSEID,
	)
	addPdrInfo(seres, addPDRs)

	return seres, nil
}

func (pConn *PFCPConn) handleSessionModificationRequest(msg message.Message) (message.Message, error) {
	upf := pConn.upf

	smreq, ok := msg.(*message.SessionModificationRequest)
	if !ok {
		return nil, errUnmarshal(errMsgUnexpectedType)
	}

	var remoteSEID uint64

	// Set just before the modification is written to the datapath, and consulted by
	// every refusal after it. Nil until then, because a message refused while it is
	// still being parsed has changed nothing to put back.
	var rollBack func()

	sendError := func(err error) (message.Message, error) {
		if rollBack != nil {
			rollBack()
		}

		logger.PfcpLog.Errorln(err)

		smres := message.NewSessionModificationResponse(0, /* MO?? <-- what's this */
			0,                                    /* FO <-- what's this? */
			remoteSEID,                           /* seid */
			smreq.SequenceNumber,                 /* seq # */
			0,                                    /* priority */
			ie.NewCause(ie.CauseRequestRejected), /* accept it blindly for the time being */
		)

		return smres, err
	}

	localSEID := smreq.SEID()

	session, ok := pConn.store.GetSession(localSEID)
	if !ok {
		// 29.244 clause 7.2.2.4.2: a message for a session this node has no context for
		// is answered "Session context not found", under header SEID 0.
		err := ErrNotFoundWithParam("PFCP session", "localSEID", localSEID)
		logger.PfcpLog.Errorln(err)

		return message.NewSessionModificationResponse(0, 0, 0, smreq.SequenceNumber, 0,
			ie.NewCause(ie.CauseSessionContextNotFound)), err
	}

	// Parse into rules of our own. Every loop below can still refuse the message, and a
	// refusal tells the control plane the session is unchanged -- which it is not if
	// the rules that did parse have already been written through the slices this copy
	// shares with the store. The PutSession at the end is what publishes them, and
	// before keeps what the session had for the rollback.
	before := session.PacketForwardingRules
	session.PacketForwardingRules = before.Clone()

	var fseidIP uint32

	if smreq.CPFSEID != nil {
		fseid, err := smreq.CPFSEID.FSEID()
		if err == nil {
			session.remoteSEID = fseid.SEID
			fseidIP = ip2int(fseid.IPv4Address)

			logger.PfcpLog.Debugln("updated FSEID from session modification request")
		}
	}

	remoteSEID = session.remoteSEID

	addPDRs := make([]pdr, 0, MaxItems)
	addFARs := make([]far, 0, MaxItems)
	addQERs := make([]qer, 0, MaxItems)
	endMarkerList := make([][]byte, 0, MaxItems)

	for _, cPDR := range smreq.CreatePDR {
		var p pdr
		if err := p.parsePDR(cPDR, localSEID, pConn.appPFDs, upf.ippool); err != nil {
			return sendError(err)
		}

		p.fseidIP = fseidIP

		session.CreatePDR(p)
		addPDRs = append(addPDRs, p)
	}
	logger.PfcpLog.Debugln("PDRs added:", addPDRs)

	for _, cFAR := range smreq.CreateFAR {
		var f far
		if err := f.parseFAR(cFAR, localSEID, upf, create); err != nil {
			return sendError(err)
		}

		f.fseidIP = fseidIP

		session.CreateFAR(f)
		addFARs = append(addFARs, f)
	}

	for _, cQER := range smreq.CreateQER {
		var q qer
		if err := q.parseQER(cQER, localSEID); err != nil {
			return sendError(err)
		}

		q.fseidIP = fseidIP

		session.CreateQER(q)
		addQERs = append(addQERs, q)
	}

	// Everything appended so far was created by this message and did not exist before
	// it; everything the update loops append replaces a rule that did.
	createdPDRs, createdFARs, createdQERs := len(addPDRs), len(addFARs), len(addQERs)

	for _, uPDR := range smreq.UpdatePDR {
		var (
			p   pdr
			err error
		)

		if err = p.parsePDR(uPDR, localSEID, pConn.appPFDs, upf.ippool); err != nil {
			return sendError(err)
		}

		p.fseidIP = fseidIP

		err = session.UpdatePDR(p)
		if err != nil {
			logger.PfcpLog.Errorln("session PDR update failed", err)
			continue
		}

		addPDRs = append(addPDRs, p)
	}

	for _, uFAR := range smreq.UpdateFAR {
		var (
			f   far
			err error
		)

		if err = f.parseFAR(uFAR, localSEID, upf, update); err != nil {
			return sendError(err)
		}

		f.fseidIP = fseidIP

		err = session.UpdateFAR(&f, &endMarkerList)
		if err != nil {
			logger.PfcpLog.Errorln("session PDR update failed", err)
			continue
		}

		addFARs = append(addFARs, f)
	}

	for _, uQER := range smreq.UpdateQER {
		var (
			q   qer
			err error
		)

		if err = q.parseQER(uQER, localSEID); err != nil {
			return sendError(err)
		}

		q.fseidIP = fseidIP

		err = session.UpdateQER(q)
		if err != nil {
			logger.PfcpLog.Errorln("session QER update failed", err)
			continue
		}

		addQERs = append(addQERs, q)
	}

	session.MarkSessionQer(session.qers)

	// Write what the session now holds of the rules this message touched. That is not
	// the same as what the loops above parsed: an Update following a Create of the same
	// ID replaces the rule the Create just added, so both versions are among the parsed
	// rules -- and written together they race, because one batch programs its rules
	// concurrently, while the session keeps only the second. And a QER's level is the
	// one the session's marking gave it: marked on the message's own QERs instead, a
	// message updating one QER alone, or carrying two versions of one, is written to a
	// table the session does not record it in.
	addPDRs, createdPDRs = heldVersions(session.pdrs, len(before.pdrs), addPDRs[createdPDRs:],
		func(p pdr) uint32 { return p.pdrID })
	addFARs, createdFARs = heldVersions(session.fars, len(before.fars), addFARs[createdFARs:],
		func(f far) uint32 { return f.farID })
	addQERs, createdQERs = heldVersions(session.qers, len(before.qers), addQERs[createdQERs:],
		func(q qer) uint32 { return q.qerID })

	updated := PacketForwardingRules{
		pdrs: addPDRs,
		fars: addFARs,
		qers: addQERs,
	}

	// From here the datapath has been asked to change, so every refusal below has to put
	// it back. A batch that was rejected programmed an unknown subset of its rules
	// before it was, and the deletion batch further down an unknown subset of the
	// removals.
	//
	// Remove what this message created, then write the rules the session had. The order
	// is what makes this safe against a Create naming a rule the session already holds:
	// nothing rejects one, so the created rule can share a datapath key with a rule in
	// before, and removing it after the restore would take the restored rule with it.
	// Restoring second cannot go wrong that way, because it re-writes every rule the
	// session had -- an overwrite for the ones this message never touched, and the way
	// back for anything the removal above or a rejected deletion batch took out.
	//
	// The cost is a window rather than a wrong state: an updated rule that points at a
	// rule this message created is left pointing at nothing until the restore lands one
	// batch later, and a packet reaching it is dropped for that moment.
	rollBack = func() {
		// What the rollback removes: the rules this message created, and the rules it
		// updated onto a datapath entry other than the one the session's own version
		// occupies -- or that have no version of their own before the message at all.
		// UpdatePDR and UpdateQER find their rule in the session as the message has built
		// it so far, so an Update following a Create of the same ID replaces the rule that
		// Create just added, and only that final version is written. The restore writes
		// nothing for such a rule, so removing it cannot take out anything the restore
		// puts back.
		//
		// The second set is the one nothing else would ever reach. The restore below
		// re-adds the old key; it does not remove the new one, and the session's later
		// deletion names the rules the store describes, which are the old ones -- so a
		// moved entry would sit there for the life of the process, and with a FAR this
		// message created it would point at a rule the removal is about to take out.
		//
		// Only rules that actually moved may go. One whose key did not move occupies the
		// very entry the restore is about to write, so removing it would blackhole a flow
		// this message never changed, for the length of two datapath batches.
		//
		// The created rules are copied out rather than appended to in place. They are the
		// front of addPDRs and addQERs, whose tails are the updates the loops below walk,
		// so appending would write into the range being read. As the loops stand that is
		// harmless -- an append never reaches past the element just read -- but the copy
		// is what makes it so a reader does not have to establish that.
		removePDRs := append([]pdr(nil), addPDRs[:createdPDRs]...)
		removeQERs := append([]qer(nil), addQERs[:createdQERs]...)

		for _, p := range addPDRs[createdPDRs:] {
			if was, ok := before.findPDR(p.pdrID); !ok || !p.occupiesSameEntryAs(was) {
				removePDRs = append(removePDRs, p)
			}
		}

		for _, q := range addQERs[createdQERs:] {
			if was, ok := before.findQER(q.qerID); !ok || !q.occupiesSameEntryAs(was) {
				removeQERs = append(removeQERs, q)
			}
		}

		// A FAR is keyed by its own ID and the session's, so an update cannot move one.
		remove := PacketForwardingRules{
			pdrs: removePDRs,
			fars: addFARs[:createdFARs],
			qers: removeQERs,
		}

		// A refused removal of a created rule does not mean the datapath kept it.
		// commandOutcome would pass a module's refusal of a delete straight through, and
		// TestSendMsgToUPFRejectsADeleteTheModuleRefused shows that it does -- but no
		// module produces one for a rule it holds: the codes the three delete paths
		// return are ENOENT, which counts as success here, and EINVAL for an argument no
		// stored rule can make. So what reaches here is a rule that could not be
		// translated or marshalled, which was never programmed either -- the reasoning
		// the establishment rollback records. A module that learns to refuse a delete of
		// a rule it holds changes that, and this line with it.
		if upf.SendMsgToUPF(upfMsgTypeDel, remove, PacketForwardingRules{}) == ie.CauseRequestRejected {
			logger.PfcpLog.Warnln("could not translate a rule a refused modification created "+
				"in order to remove it; it was not programmed either, F-SEID:", localSEID)
		}

		if upf.SendMsgToUPF(upfMsgTypeMod, before, before) == ie.CauseRequestRejected {
			logger.PfcpLog.Warnln("datapath refused to restore the rules of a refused "+
				"modification; it may be missing rules this session describes, or holding "+
				"the refused modification's version of them, F-SEID:", localSEID)
		}
	}

	cause := upf.SendMsgToUPF(upfMsgTypeMod, session.PacketForwardingRules, updated)
	if cause == ie.CauseRequestRejected {
		return sendError(ErrWriteToDatapath)
	}

	if upf.enableEndMarker {
		err := upf.SendEndMarkers(&endMarkerList)
		if err != nil {
			logger.PfcpLog.Errorln("sending End Markers Failed:", err)
		}
	}

	delPDRs := make([]pdr, 0, MaxItems)
	delFARs := make([]far, 0, MaxItems)
	delQERs := make([]qer, 0, MaxItems)

	for _, rPDR := range smreq.RemovePDR {
		pdrID, err := rPDR.PDRID()
		if err != nil {
			return sendError(err)
		}

		p, err := session.RemovePDR(uint32(pdrID))
		if err != nil {
			return sendError(err)
		}

		delPDRs = append(delPDRs, *p)
	}

	for _, dFAR := range smreq.RemoveFAR {
		farID, err := dFAR.FARID()
		if err != nil {
			return sendError(err)
		}

		f, err := session.RemoveFAR(farID)
		if err != nil {
			return sendError(err)
		}

		delFARs = append(delFARs, *f)
	}

	for _, dQER := range smreq.RemoveQER {
		qerID, err := dQER.QERID()
		if err != nil {
			return sendError(err)
		}

		q, err := session.RemoveQER(qerID)
		if err != nil {
			return sendError(err)
		}

		delQERs = append(delQERs, *q)
	}

	deleted := PacketForwardingRules{
		pdrs: delPDRs,
		fars: delFARs,
		qers: delQERs,
	}

	cause = upf.SendMsgToUPF(upfMsgTypeDel, deleted, PacketForwardingRules{})
	if cause == ie.CauseRequestRejected {
		return sendError(ErrWriteToDatapath)
	}

	err := pConn.store.PutSession(session)
	if err != nil {
		logger.PfcpLog.Errorf("failed to put PFCP session to store: %v", err)
	}

	// Build response message
	smres := message.NewSessionModificationResponse(0, /* MO?? <-- what's this */
		0,                                    /* FO <-- what's this? */
		remoteSEID,                           /* seid */
		smreq.SequenceNumber,                 /* seq # */
		0,                                    /* priority */
		ie.NewCause(ie.CauseRequestAccepted), /* accept it blindly for the time being */
	)

	return smres, nil
}

func (pConn *PFCPConn) handleSessionDeletionRequest(msg message.Message) (message.Message, error) {
	upf := pConn.upf

	sdreq, ok := msg.(*message.SessionDeletionRequest)
	if !ok {
		return nil, errUnmarshal(errMsgUnexpectedType)
	}

	// Zero until the session is found. The lookup's own refusal is answered separately
	// below; every refusal after it is for a session the control plane has an SEID for,
	// and carries it.
	var remoteSEID uint64

	sendError := func(err error) (message.Message, error) {
		smres := message.NewSessionDeletionResponse(0, /* MO?? <-- what's this */
			0,                                    /* FO <-- what's this? */
			remoteSEID,                           /* seid */
			sdreq.SequenceNumber,                 /* seq # */
			0,                                    /* priority */
			ie.NewCause(ie.CauseRequestRejected), /* accept it blindly for the time being */
		)

		return smres, err
	}

	/* retrieve sessionRecord */
	localSEID := sdreq.SEID()

	session, ok := pConn.store.GetSession(localSEID)
	if !ok {
		// 29.244 clause 7.2.2.4.2: a message for a session this node has no context for
		// is answered "Session context not found", under header SEID 0.
		err := ErrNotFoundWithParam("PFCP session", "localSEID", localSEID)

		return message.NewSessionDeletionResponse(0, 0, 0, sdreq.SequenceNumber, 0,
			ie.NewCause(ie.CauseSessionContextNotFound)), err
	}

	remoteSEID = session.remoteSEID

	// This caller needs more than the cause. A batch that ran out of time is answered
	// accepted, which elsewhere is the answer that destroys nothing -- but here accepted
	// is what releases the address and forgets the session, and the session is the only
	// thing that still names the rules the batch may not have removed. Once it is gone,
	// no later message can name them: the control plane has been told it is gone too.
	//
	// So an unfinished deletion keeps the session and is refused. That costs the session
	// rather than the rules: a control plane that retries converges, because a delete of a
	// rule the datapath no longer holds is answered success, and one that does not leaves
	// the session here until the association is torn down -- counted, logged, and removed
	// at teardown with its rules still named. Where this UPF allocates the UE addresses it
	// also holds the address meanwhile, so that no other UE is given one a surviving rule
	// still matches; where the control plane allocates them there is no pool here to hold
	// it in, and the address goes back to the control plane's regardless.
	//
	// A modification that removes rules must not do the same. Its session is live and is
	// kept whatever the answer, and refusing would send it into a rollback whose restore
	// the late removal could then undo. That is why this is decided here and not in
	// SendMsgToUPF.
	cause, finished := upf.SendMsgToUPFWithCompletion(
		upfMsgTypeDel, session.PacketForwardingRules, PacketForwardingRules{})
	if cause == ie.CauseRequestRejected || !finished {
		return sendError(ErrWriteToDatapath)
	}

	upf.ippool.Release(session.localSEID)

	/* delete sessionRecord */
	pConn.RemoveSession(session)

	// Build response message
	smres := message.NewSessionDeletionResponse(0, /* MO?? <-- what's this */
		0,                                    /* FO <-- what's this? */
		session.remoteSEID,                   /* seid */
		sdreq.SequenceNumber,                 /* seq # */
		0,                                    /* priority */
		ie.NewCause(ie.CauseRequestAccepted), /* accept it blindly for the time being */
	)

	return smres, nil
}

// handleDigestReport reports a downlink data notification to the control plane, and
// returns whether this association is the one holding the session. A notification
// carries only an F-SEID, so the caller cannot know which association owns it and has
// to offer it around.
func (pConn *PFCPConn) handleDigestReport(fseid uint64) bool {
	session, ok := pConn.store.GetSession(fseid)
	if !ok {
		return false
	}

	seq := pConn.getSeqNum()
	srreq := message.NewSessionReportRequest(0, /* MO?? <-- what's this */
		0,                            /* FO <-- what's this? */
		0,                            /* seid */
		seq,                          /* seq # */
		0,                            /* priority */
		ie.NewReportType(0, 0, 0, 1), /*upir, erir, usar, dldr int*/
	)
	srreq.Header.SEID = session.remoteSEID

	var pdrID uint32

	var farID uint32

	for _, pdr := range session.pdrs {
		if pdr.srcIface == core {
			pdrID = pdr.pdrID

			farID = pdr.farID

			break
		}
	}

	for _, far := range session.fars {
		if far.farID == farID {
			if far.applyAction&ActionNotify == 0 {
				// This association holds the session, so the notification is answered
				// here whatever the outcome: offering it on would only find peers that
				// do not know the F-SEID at all.
				logger.PfcpLog.Errorln("packet received for forwarding far. discard")

				return true
			}
		}
	}

	if pdrID == 0 {
		logger.PfcpLog.Errorln("no Pdr found for downlink")

		return true
	}

	srreq.DownlinkDataReport = ie.NewDownlinkDataReport(
		ie.NewPDRID(uint16(pdrID)))

	logger.PfcpLog.With("F-SEID", fseid, "PDR ID", pdrID).Infoln("sending Downlink Data Report")

	pConn.SendPFCPMsg(srreq)

	return true
}

// pfcpSRRspFlagDROBU is the DROBU bit of the PFCPSRRsp-Flags IE, TS 29.244 8.2.32. It asks
// the user plane to discard the packets it buffered for the session. go-pfcp offers a
// HasDROBU helper for PFCPSMReq-Flags but not for these, so the bit is tested here.
const pfcpSRRspFlagDROBU = 0x01

// reportDropBufferedRequest says when the control plane has asked for buffered packets to
// be discarded and this element cannot honour it. The BUFF apply action reaches this
// datapath as notify-only, so there is no store to discard -- honouring DROBU is part of
// downlink buffering, which is not implemented here. Without this the control plane's log
// reads as though buffering had been stopped while nothing acted on the request.
func reportDropBufferedRequest(srres *message.SessionReportResponse) {
	if srres.PFCPSRRspFlags == nil {
		return
	}

	flags, err := srres.PFCPSRRspFlags.PFCPSRRspFlags()
	if err != nil {
		logger.PfcpLog.Warnln("could not read PFCPSRRsp-Flags:", err)
		return
	}

	if flags&pfcpSRRspFlagDROBU != 0 {
		logger.PfcpLog.Warnln("DROBU requested for seq, but this datapath notifies rather than buffers, "+
			"so there is nothing to discard:", srres.SequenceNumber)
	}
}

func (pConn *PFCPConn) handleSessionReportResponse(msg message.Message) error {
	upf := pConn.upf

	srres, ok := msg.(*message.SessionReportResponse)
	if !ok {
		return errUnmarshal(errMsgUnexpectedType)
	}

	if srres.Cause == nil {
		// Reject responses that omit the mandatory Cause to avoid nil deref on malformed PFCP messages.
		addr := pConn.RemoteAddr().String()
		logger.PfcpLog.Warnln("session Report Response without Cause from", addr)
		return errUnmarshal(ErrCauseMissing)
	}

	cause, err := srres.Cause.Cause()
	if err != nil {
		return errUnmarshal(err)
	}

	reportDropBufferedRequest(srres)

	if cause == ie.CauseRequestAccepted {
		return nil
	}

	logger.PfcpLog.Warnln("session req not accepted seq:", srres.SequenceNumber)

	seid := srres.SEID()

	if cause == ie.CauseSessionContextNotFound {
		sessItem, ok := pConn.store.GetSession(seid)
		if !ok {
			return errProcess(ErrNotFoundWithParam("PFCP session context", "SEID", seid))
		}

		logger.PfcpLog.Warnln("context not found, deleting session locally")

		// The address goes back to the pool before the session is forgotten, because
		// nothing can return it afterwards. This site already forgets the session before
		// deleting its rules, so unlike the deletion path the release precedes the
		// delete; the free queue is FIFO, so the address is not handed out again while
		// the delete runs unless every other free address is already taken.
		upf.ippool.Release(seid)

		pConn.RemoveSession(sessItem)

		cause := upf.SendMsgToUPF(
			upfMsgTypeDel, sessItem.PacketForwardingRules, PacketForwardingRules{})
		if cause == ie.CauseRequestRejected {
			return errProcess(
				ErrOperationFailedWithParam("delete session from datapath", "seid", seid))
		}

		return nil
	}

	return nil
}
