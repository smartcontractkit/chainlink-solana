package logpoller_test

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/mocks"
)

func TestLogPoller_ProcessAndSave(t *testing.T) {
	t.Parallel()

	client := new(mocks.RPCClient)
	saver := new(testSaver)
	poller := logpoller.New(client, logger.Nop(), saver)

	require.NoError(t, poller.AddFilter("TestEvent"))

	clientExpectSingleEvent(client)

	require.NoError(t, poller.Start(tests.Context(t)))

	t.Cleanup(func() {
		require.NoError(t, poller.Close())
	})

	tests.AssertEventually(t, func() bool {
		return saver.Called()
	})

	client.AssertExpectations(t)
}

type testSaver struct {
	called atomic.Bool
	count  atomic.Uint64
}

func (s *testSaver) SaveEvent(event logpoller.ProgramEvent) error {
	s.called.Store(true)
	s.count.Store(s.count.Load() + 1)

	return nil
}

func (s *testSaver) Called() bool {
	return s.called.Load()
}

func (s *testSaver) Count() uint64 {
	return s.count.Load()
}
