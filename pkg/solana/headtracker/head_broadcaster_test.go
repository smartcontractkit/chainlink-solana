package headtracker_test

import (
	"context"
	"testing"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	commonhtrk "github.com/smartcontractkit/chainlink-relay/pkg/headtracker"
	commontypes "github.com/smartcontractkit/chainlink-relay/pkg/types"
	commonmocks "github.com/smartcontractkit/chainlink-relay/pkg/types/mocks"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/headtracker"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/headtracker/types"

	// configtest "github.com/smartcontractkit/chainlink/v2/core/internal/testutils/configtest/v2"
	"github.com/smartcontractkit/chainlink-solana/pkg/internal/testutils"
	// "github.com/smartcontractkit/chainlink/v2/core/internal/testutils/pgtest"
	"github.com/smartcontractkit/chainlink-relay/pkg/logger"
	// "github.com/smartcontractkit/chainlink/v2/core/services/chainlink"
	// "github.com/smartcontractkit/chainlink/v2/core/store/models"
	"github.com/smartcontractkit/chainlink-relay/pkg/services"
	"github.com/smartcontractkit/chainlink-relay/pkg/utils"
	"github.com/smartcontractkit/chainlink-solana/pkg/internal/cltest"
)

func waitHeadBroadcasterToStart(t *testing.T, hb commontypes.HeadBroadcaster[*types.Head, types.Hash]) {
	t.Helper()

	subscriber := &cltest.MockHeadTrackable{}
	_, unsubscribe := hb.Subscribe(subscriber)
	defer unsubscribe()

	hb.BroadcastNewLongestChain(cltest.Head(1))
	g := gomega.NewWithT(t)
	g.Eventually(subscriber.OnNewLongestChainCount).Should(gomega.Equal(int32(1)))
}

func TestHeadBroadcaster_Subscribe(t *testing.T) {
	t.Parallel()
	g := gomega.NewWithT(t)

	lggr, _ := logger.New()
	sub := commonmocks.NewSubscription(t)
	client := cltest.NewClientMockWithDefaultChain(t)
	cfg := headtracker.NewConfig()

	chchHeaders := make(chan chan<- *types.Head, 1)

	client.On("SubscribeNewHead", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			chchHeaders <- args.Get(1).(chan<- *types.Head)
		}).
		Return(sub, nil)
	client.On("HeadByNumber", mock.Anything, mock.Anything).Return(cltest.Head(1), nil)

	sub.On("Unsubscribe").Return()
	sub.On("Err").Return(nil)

	checker1 := &cltest.MockHeadTrackable{}
	checker2 := &cltest.MockHeadTrackable{}

	hb := headtracker.NewHeadBroadcaster(lggr)
	hs := headtracker.NewHeadSaver(cfg, lggr)
	mailMon := utils.NewMailboxMonitor(t.Name())
	ht := headtracker.NewHeadTracker(lggr, client, cfg, hb, hs, mailMon)

	var ms services.MultiStart
	require.NoError(t, ms.Start(testutils.Context(t), mailMon, hb, ht))
	t.Cleanup(func() { require.NoError(t, services.CloseAll(mailMon, hb, ht)) })

	latest1, unsubscribe1 := hb.Subscribe(checker1)
	// "latest head" is nil here because we didn't receive any yet
	assert.Equal(t, (*types.Head)(nil), latest1)

	firstHead := cltest.Head(1)
	secondHead := cltest.Head(2)
	firstHead.Parent = secondHead
	secondHead.Block.PreviousBlockhash = firstHead.Block.Blockhash

	headers := <-chchHeaders
	headers <- firstHead
	g.Eventually(checker1.OnNewLongestChainCount).Should(gomega.Equal(int32(1)))

	latest2, _ := hb.Subscribe(checker2)
	// "latest head" is set here to the most recent head received
	assert.NotNil(t, latest2)
	assert.Equal(t, firstHead.BlockNumber(), latest2.BlockNumber())

	unsubscribe1()

	headers <- secondHead
	g.Eventually(checker2.OnNewLongestChainCount).Should(gomega.Equal(int32(1)))
}

func TestHeadBroadcaster_BroadcastNewLongestChain(t *testing.T) {
	t.Parallel()
	g := gomega.NewWithT(t)

	lggr, _ := logger.New()
	broadcaster := headtracker.NewHeadBroadcaster(lggr)

	err := broadcaster.Start(testutils.Context(t))
	require.NoError(t, err)

	waitHeadBroadcasterToStart(t, broadcaster)

	subscriber1 := &cltest.MockHeadTrackable{}
	subscriber2 := &cltest.MockHeadTrackable{}
	_, unsubscribe1 := broadcaster.Subscribe(subscriber1)
	_, unsubscribe2 := broadcaster.Subscribe(subscriber2)

	broadcaster.BroadcastNewLongestChain(cltest.Head(1))
	g.Eventually(subscriber1.OnNewLongestChainCount).Should(gomega.Equal(int32(1)))

	unsubscribe1()

	broadcaster.BroadcastNewLongestChain(cltest.Head(2))
	g.Eventually(subscriber2.OnNewLongestChainCount).Should(gomega.Equal(int32(2)))

	unsubscribe2()

	subscriber3 := &cltest.MockHeadTrackable{}
	_, unsubscribe3 := broadcaster.Subscribe(subscriber3)
	broadcaster.BroadcastNewLongestChain(cltest.Head(1))
	g.Eventually(subscriber3.OnNewLongestChainCount).Should(gomega.Equal(int32(1)))

	unsubscribe3()

	// no subscribers - shall do nothing
	broadcaster.BroadcastNewLongestChain(cltest.Head(0))

	err = broadcaster.Close()
	require.NoError(t, err)

	require.Equal(t, int32(1), subscriber3.OnNewLongestChainCount())
}

func TestHeadBroadcaster_TrackableCallbackTimeout(t *testing.T) {
	t.Parallel()

	lggr, _ := logger.New()
	broadcaster := headtracker.NewHeadBroadcaster(lggr)

	err := broadcaster.Start(testutils.Context(t))
	require.NoError(t, err)

	waitHeadBroadcasterToStart(t, broadcaster)

	slowAwaiter := cltest.NewAwaiter()
	fastAwaiter := cltest.NewAwaiter()
	slow := &sleepySubscriber{awaiter: slowAwaiter, delay: commonhtrk.TrackableCallbackTimeout * 2}
	fast := &sleepySubscriber{awaiter: fastAwaiter, delay: commonhtrk.TrackableCallbackTimeout / 2}
	_, unsubscribe1 := broadcaster.Subscribe(slow)
	_, unsubscribe2 := broadcaster.Subscribe(fast)

	broadcaster.BroadcastNewLongestChain(cltest.Head(1))
	slowAwaiter.AwaitOrFail(t, testutils.WaitTimeout(t))
	fastAwaiter.AwaitOrFail(t, testutils.WaitTimeout(t))

	require.True(t, slow.contextDone)
	require.False(t, fast.contextDone)

	unsubscribe1()
	unsubscribe2()

	err = broadcaster.Close()
	require.NoError(t, err)
}

type sleepySubscriber struct {
	awaiter     cltest.Awaiter
	delay       time.Duration
	contextDone bool
}

func (ss *sleepySubscriber) OnNewLongestChain(ctx context.Context, head *types.Head) {
	time.Sleep(ss.delay)
	select {
	case <-ctx.Done():
		ss.contextDone = true
	default:
	}
	ss.awaiter.ItHappened()
}
