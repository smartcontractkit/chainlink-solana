package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
)

func TestSimpleGauge(t *testing.T) {
	// panic on empty logger
	require.Panics(t, func() { newSimpleGauge(nil, "") })

	lgr, logs := logger.TestObserved(t, zapcore.ErrorLevel)

	// invalid name
	g := newSimpleGauge(lgr, t.Name())
	g.set(0, prometheus.Labels{})
	g.delete(prometheus.Labels{})
	require.Equal(t, 2, logs.FilterMessage("gauge not found").Len())

	// happy path is tested by each individual metric implementation
	// to match proper metrics and labels
}
