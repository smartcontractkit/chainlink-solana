package client

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"

	mnCfg "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/multinode/config"
)

type MultiNodeClient[RPC any, HEAD Head] struct {
	cfg         *mnCfg.MultiNodeConfig
	log         logger.Logger
	rpc         *RPC
	ctxTimeout  time.Duration
	stateMu     sync.RWMutex // protects state* fields
	subsSliceMu sync.RWMutex
	subs        map[Subscription]struct{}

	latestBlock          func(ctx context.Context, rpc *RPC) (HEAD, error)
	latestFinalizedBlock func(ctx context.Context, rpc *RPC) (HEAD, error)

	// chStopInFlight can be closed to immediately cancel all in-flight requests on
	// this RpcClient. Closing and replacing should be serialized through
	// stateMu since it can happen on state transitions as well as RpcClient Close.
	chStopInFlight chan struct{}

	chainInfoLock sync.RWMutex
	// intercepted values seen by callers of the rpcClient excluding health check calls. Need to ensure MultiNode provides repeatable read guarantee
	highestUserObservations ChainInfo
	// most recent chain info observed during current lifecycle (reseted on DisconnectAll)
	latestChainInfo ChainInfo
}

// WrappedSubscription is used to ensure that the subscription is removed from the client when unsubscribed
type WrappedSubscription struct {
	Subscription
	removeSub func(sub Subscription)
}

func (w *WrappedSubscription) Unsubscribe() {
	w.Subscription.Unsubscribe()
	if w.removeSub != nil {
		w.removeSub(w)
	}
}

func NewMultiNodeClient[RPC any, HEAD Head](
	cfg *mnCfg.MultiNodeConfig, rpc *RPC, ctxTimeout time.Duration, log logger.Logger,
	latestBlock func(ctx context.Context, rpc *RPC) (HEAD, error),
	latestFinalizedBlock func(ctx context.Context, rpc *RPC) (HEAD, error),
) (*MultiNodeClient[RPC, HEAD], error) {
	return &MultiNodeClient[RPC, HEAD]{
		cfg:                  cfg,
		rpc:                  rpc,
		log:                  log,
		ctxTimeout:           ctxTimeout,
		latestBlock:          latestBlock,
		latestFinalizedBlock: latestFinalizedBlock,
		subs:                 make(map[Subscription]struct{}),
		chStopInFlight:       make(chan struct{}),
	}, nil
}

func (m *MultiNodeClient[RPC, HEAD]) LenSubs() int {
	m.subsSliceMu.RLock()
	defer m.subsSliceMu.RUnlock()
	return len(m.subs)
}

func (m *MultiNodeClient[RPC, HEAD]) removeSubscription(sub Subscription) {
	m.subsSliceMu.Lock()
	defer m.subsSliceMu.Unlock()
	delete(m.subs, sub)
}

// registerSub adds the sub to the rpcClient list
func (m *MultiNodeClient[RPC, HEAD]) registerSub(sub Subscription, stopInFLightCh chan struct{}) error {
	m.subsSliceMu.Lock()
	defer m.subsSliceMu.Unlock()
	// ensure that the `sub` belongs to current life cycle of the `rpcClient` and it should not be killed due to
	// previous `DisconnectAll` call.
	select {
	case <-stopInFLightCh:
		sub.Unsubscribe()
		return fmt.Errorf("failed to register subscription - all in-flight requests were canceled")
	default:
	}
	m.subs[sub] = struct{}{}
	return nil
}

func (m *MultiNodeClient[RPC, HEAD]) LatestBlock(ctx context.Context) (HEAD, error) {
	// capture chStopInFlight to ensure we are not updating chainInfo with observations related to previous life cycle
	ctx, cancel, chStopInFlight, rpc := m.AcquireQueryCtx(ctx, m.ctxTimeout)
	defer cancel()

	head, err := m.latestBlock(ctx, rpc)
	if err != nil {
		return head, err
	}

	if !head.IsValid() {
		return head, errors.New("invalid head")
	}

	m.OnNewHead(ctx, chStopInFlight, head)
	return head, nil
}

func (m *MultiNodeClient[RPC, HEAD]) LatestFinalizedBlock(ctx context.Context) (HEAD, error) {
	ctx, cancel, chStopInFlight, rpc := m.AcquireQueryCtx(ctx, m.ctxTimeout)
	defer cancel()

	head, err := m.latestFinalizedBlock(ctx, rpc)
	if err != nil {
		return head, err
	}

	if !head.IsValid() {
		return head, errors.New("invalid head")
	}

	m.OnNewFinalizedHead(ctx, chStopInFlight, head)
	return head, nil
}

func (m *MultiNodeClient[RPC, HEAD]) SubscribeToHeads(ctx context.Context) (<-chan HEAD, Subscription, error) {
	ctx, cancel, chStopInFlight, _ := m.AcquireQueryCtx(ctx, m.ctxTimeout)
	defer cancel()

	// TODO: BCFR-1070 - Add BlockPollInterval
	pollInterval := m.cfg.FinalizedBlockPollInterval() // Use same interval as finalized polling
	if pollInterval == 0 {
		return nil, nil, errors.New("PollInterval is 0")
	}
	timeout := pollInterval
	poller, channel := NewPoller[HEAD](pollInterval, func(pollRequestCtx context.Context) (HEAD, error) {
		if CtxIsHeathCheckRequest(ctx) {
			pollRequestCtx = CtxAddHealthCheckFlag(pollRequestCtx)
		}
		return m.LatestBlock(pollRequestCtx)
	}, timeout, m.log)

	if err := poller.Start(ctx); err != nil {
		return nil, nil, err
	}

	sub := &WrappedSubscription{
		Subscription: &poller,
		removeSub:    m.removeSubscription,
	}

	err := m.registerSub(sub, chStopInFlight)
	if err != nil {
		sub.Unsubscribe()
		return nil, nil, err
	}

	return channel, sub, nil
}

func (m *MultiNodeClient[RPC, HEAD]) SubscribeToFinalizedHeads(ctx context.Context) (<-chan HEAD, Subscription, error) {
	ctx, cancel, chStopInFlight, _ := m.AcquireQueryCtx(ctx, m.ctxTimeout)
	defer cancel()

	finalizedBlockPollInterval := m.cfg.FinalizedBlockPollInterval()
	if finalizedBlockPollInterval == 0 {
		return nil, nil, errors.New("FinalizedBlockPollInterval is 0")
	}
	timeout := finalizedBlockPollInterval
	poller, channel := NewPoller[HEAD](finalizedBlockPollInterval, func(pollRequestCtx context.Context) (HEAD, error) {
		if CtxIsHeathCheckRequest(ctx) {
			pollRequestCtx = CtxAddHealthCheckFlag(pollRequestCtx)
		}
		return m.LatestFinalizedBlock(pollRequestCtx)
	}, timeout, m.log)
	if err := poller.Start(ctx); err != nil {
		return nil, nil, err
	}

	sub := &WrappedSubscription{
		Subscription: &poller,
		removeSub:    m.removeSubscription,
	}

	err := m.registerSub(sub, chStopInFlight)
	if err != nil {
		poller.Unsubscribe()
		return nil, nil, err
	}

	return channel, sub, nil
}

func (m *MultiNodeClient[RPC, HEAD]) OnNewHead(ctx context.Context, requestCh <-chan struct{}, head HEAD) {
	if !head.IsValid() {
		return
	}

	m.chainInfoLock.Lock()
	defer m.chainInfoLock.Unlock()
	if !CtxIsHeathCheckRequest(ctx) {
		m.highestUserObservations.BlockNumber = max(m.highestUserObservations.BlockNumber, head.BlockNumber())
	}
	select {
	case <-requestCh: // no need to update latestChainInfo, as rpcClient already started new life cycle
		return
	default:
		m.latestChainInfo.BlockNumber = head.BlockNumber()
	}
}

func (m *MultiNodeClient[RPC, HEAD]) OnNewFinalizedHead(ctx context.Context, requestCh <-chan struct{}, head HEAD) {
	if !head.IsValid() {
		return
	}

	m.chainInfoLock.Lock()
	defer m.chainInfoLock.Unlock()
	if !CtxIsHeathCheckRequest(ctx) {
		m.highestUserObservations.FinalizedBlockNumber = max(m.highestUserObservations.FinalizedBlockNumber, head.BlockNumber())
	}
	select {
	case <-requestCh: // no need to update latestChainInfo, as rpcClient already started new life cycle
		return
	default:
		m.latestChainInfo.FinalizedBlockNumber = head.BlockNumber()
	}
}

// MakeQueryCtx returns a context that cancels if:
// 1. Passed in ctx cancels
// 2. Passed in channel is closed
// 3. Default timeout is reached (queryTimeout)
func MakeQueryCtx(ctx context.Context, ch services.StopChan, timeout time.Duration) (context.Context, context.CancelFunc) {
	var chCancel, timeoutCancel context.CancelFunc
	ctx, chCancel = ch.Ctx(ctx)
	ctx, timeoutCancel = context.WithTimeout(ctx, timeout)
	cancel := func() {
		chCancel()
		timeoutCancel()
	}
	return ctx, cancel
}

func (m *MultiNodeClient[RPC, HEAD]) AcquireQueryCtx(parentCtx context.Context, timeout time.Duration) (ctx context.Context, cancel context.CancelFunc,
	chStopInFlight chan struct{}, raw *RPC) {
	// Need to wrap in mutex because state transition can cancel and replace context
	m.stateMu.RLock()
	chStopInFlight = m.chStopInFlight
	cp := *m.rpc
	raw = &cp
	m.stateMu.RUnlock()
	ctx, cancel = MakeQueryCtx(parentCtx, chStopInFlight, timeout)
	return
}

func (m *MultiNodeClient[RPC, HEAD]) UnsubscribeAllExcept(subs ...Subscription) {
	m.subsSliceMu.Lock()
	defer m.subsSliceMu.Unlock()

	keepSubs := map[Subscription]struct{}{}
	for _, sub := range subs {
		keepSubs[sub] = struct{}{}
	}

	for sub := range m.subs {
		if _, keep := keepSubs[sub]; !keep {
			// Release lock to avoid deadlock on unsubscribe
			m.subsSliceMu.Unlock()
			sub.Unsubscribe()
			m.subsSliceMu.Lock()
			delete(m.subs, sub)
		}
	}
}

// CancelInflightRequests closes and replaces the chStopInFlight
func (m *MultiNodeClient[RPC, HEAD]) CancelInflightRequests() {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	close(m.chStopInFlight)
	m.chStopInFlight = make(chan struct{})
}

func (m *MultiNodeClient[RPC, HEAD]) Close() {
	m.CancelInflightRequests()
	m.UnsubscribeAllExcept()
	m.chainInfoLock.Lock()
	m.latestChainInfo = ChainInfo{}
	m.chainInfoLock.Unlock()
}

func (m *MultiNodeClient[RPC, HEAD]) GetInterceptedChainInfo() (latest, highestUserObservations ChainInfo) {
	m.chainInfoLock.Lock()
	defer m.chainInfoLock.Unlock()
	return m.latestChainInfo, m.highestUserObservations
}
