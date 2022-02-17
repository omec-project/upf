// SPDX-License-Identifier: Apache-2.0
// Copyright 2021-present Intel Corporation

package metrics

import "time"

type Message struct {
	NodeID    string
	MsgType   string
	Direction string
	Result    string

	StartedAt time.Time
	Duration  float64
}

func NewMessage(msgType, direction string) *Message {
	return &Message{
		MsgType:   msgType,
		Direction: direction,

		StartedAt: time.Now(),
	}
}

func (m *Message) Finish(nodeID, result string) {
	m.NodeID = nodeID
	m.Result = result
	m.Duration = time.Since(m.StartedAt).Seconds()
}

type Session struct {
	NodeID string

	CreatedAt time.Time
	Duration  float64
}

func NewSession(nodeID string) *Session {
	return &Session{
		NodeID:    nodeID,
		CreatedAt: time.Now(),
	}
}

func (s *Session) Delete() {
	s.Duration = time.Since(s.CreatedAt).Seconds()
}

type InstrumentPFCP interface {
	SaveMessages(m *Message)
	SaveSessions(s *Session)
	Stop() error
}
