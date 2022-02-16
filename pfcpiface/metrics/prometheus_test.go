package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNewPrometheusService(t *testing.T) {
	t.Run("cannot register multiple times without stop", func(t *testing.T) {
		_, err := NewPrometheusService()
		require.NoError(t, err)

		_, err = NewPrometheusService()
		require.Error(t, err)
	})

	// TODO: we currently need to reset the DefaultRegisterer between tests.
	// Use custom registries to avoid global state.
	prometheus.DefaultRegisterer = prometheus.NewRegistry()

	t.Run("can register multiple times with stop", func(t *testing.T) {
		var s *Service
		s, err := NewPrometheusService()
		require.NoError(t, err)

		err = s.Stop()
		require.NoError(t, err)

		s, err = NewPrometheusService()
		require.NoError(t, err)
	})
}
