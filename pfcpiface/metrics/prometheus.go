// SPDX-License-Identifier: Apache-2.0
// Copyright 2021-present Intel Corporation

package metrics

import (
	"github.com/prometheus/client_golang/prometheus/promauto"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type Service struct {
	msgCount    *prometheus.CounterVec
	msgDuration *prometheus.HistogramVec

	sessions        *prometheus.GaugeVec
	sessionDuration *prometheus.HistogramVec
}

func NewPrometheusService() (*Service, error) {
	reg := prometheus.NewRegistry()

	msgCount := promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
		Name: "pfcp_messages_total",
		Help: "Counter for incoming and outgoing PFCP messages",
	}, []string{"node_id", "message_type", "direction", "result"})

	msgDuration := promauto.With(reg).NewHistogramVec(prometheus.HistogramOpts{
		Name:    "pfcp_messages_duration_seconds",
		Help:    "The latency of the PFCP request",
		Buckets: []float64{1e-6, 1e-5, 1e-4, 1e-3, 1e-2, 1e-1, 1, 1e1},
	}, []string{"node_id", "message_type", "direction"})

	sessions := promauto.With(reg).NewGaugeVec(prometheus.GaugeOpts{
		Name: "pfcp_sessions",
		Help: "Number of PFCP sessions currently in the UPF",
	}, []string{"node_id"})

	sessionDuration := promauto.With(reg).NewHistogramVec(prometheus.HistogramOpts{
		Name: "pfcp_session_duration_seconds",
		Help: "The lifetime of PFCP session",
		Buckets: []float64{
			1 * time.Minute.Seconds(),
			10 * time.Minute.Seconds(),
			30 * time.Minute.Seconds(),

			1 * time.Hour.Seconds(),
			6 * time.Hour.Seconds(),
			12 * time.Hour.Seconds(),
			24 * time.Hour.Seconds(),

			7 * 24 * time.Hour.Seconds(),
			4 * 7 * 24 * time.Hour.Seconds(),
		},
	}, []string{"node_id"})

	s := &Service{
		msgCount:    msgCount,
		msgDuration: msgDuration,

		sessions:        sessions,
		sessionDuration: sessionDuration,
	}

	return s, nil
}

func (s *Service) SaveMessages(msg *Message) {
	s.msgCount.WithLabelValues(msg.NodeID, msg.MsgType, msg.Direction, msg.Result).Inc()
	s.msgDuration.WithLabelValues(msg.NodeID, msg.MsgType, msg.Direction).Observe(msg.Duration)
}

func (s *Service) SaveSessions(sess *Session) {
	if sess.Duration == 0 {
		s.sessions.WithLabelValues(sess.NodeID).Inc()
		return
	}

	s.sessions.WithLabelValues(sess.NodeID).Dec()
	s.sessionDuration.WithLabelValues(sess.NodeID).Observe(sess.Duration)
}

func (s *Service) Stop() error {
	prometheus.Unregister(s.msgCount)
	prometheus.Unregister(s.msgDuration)
	prometheus.Unregister(s.sessions)
	prometheus.Unregister(s.sessionDuration)

	return nil
}
