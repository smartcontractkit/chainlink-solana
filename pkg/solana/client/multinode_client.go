package client

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"

	mn "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/multinode"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

type Head struct {
	BlockHeight *uint64
	BlockHash   *solana.Hash
}

func (h *Head) BlockNumber() int64 {
	if !h.IsValid() {
		return 0
	}
	// nolint:gosec
	// G115: integer overflow conversion uint64 -&gt; int64
	return int64(*h.BlockHeight)
}

func (h *Head) BlockDifficulty() *big.Int {
	// Not relevant for Solana
	return nil
}

func (h *Head) IsValid() bool {
	return h != nil && h.BlockHeight != nil && h.BlockHash != nil
}

var _ mn.RPCClient[mn.StringID, *Head] = (*MultiNodeClient)(nil)
var _ mn.SendTxRPCClient[*solana.Transaction, *SendTxResult] = (*MultiNodeClient)(nil)

type MultiNodeClient struct {
	Client
	cfg         *config.TOMLConfig
	stateMu     sync.RWMutex // protects state* fields
	subsSliceMu sync.RWMutex
	subs        map[mn.Subscription]struct{}

	// chStopInFlight can be closed to immediately cancel all in-flight requests on
	// this RpcClient. Closing and replacing should be serialized through
	// stateMu since it can happen on state transitions as well as RpcClient Close.
	chStopInFlight chan struct{}

	chainInfoLock sync.RWMutex
	// intercepted values seen by callers of the rpcClient excluding health check calls. Need to ensure MultiNode provides repeatable read guarantee
	highestUserObservations mn.ChainInfo
	// most recent chain info observed during current lifecycle (reseted on DisconnectAll)
	latestChainInfo mn.ChainInfo
}

func NewMultiNodeClient(endpoint string, cfg *config.TOMLConfig, requestTimeout time.Duration, log logger.Logger) (*MultiNodeClient, error) {
	client, err := NewClient(endpoint, cfg, requestTimeout, log)
	if err != nil {
		return nil, err
	}

	return &MultiNodeClient{
		Client:         *client,
		cfg:            cfg,
		subs:           make(map[mn.Subscription]struct{}),
		chStopInFlight: make(chan struct{}),
	}, nil
}

// registerSub adds the sub to the rpcClient list
func (m *MultiNodeClient) registerSub(sub mn.Subscription, stopInFLightCh chan struct{}) error {
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
	// TODO: BCI-3358 - delete sub when caller unsubscribes.
	m.subs[sub] = struct{}{}
	return nil
}

func (m *MultiNodeClient) Dial(ctx context.Context) error {
	// Not relevant for Solana as the RPCs don't need to be dialled.
	return nil
}

func (m *MultiNodeClient) SubscribeToHeads(ctx context.Context) (<-chan *Head, mn.Subscription, error) {
	ctx, cancel, chStopInFlight, _ := m.acquireQueryCtx(ctx, m.cfg.TxTimeout())
	defer cancel()

	pollInterval := m.cfg.MultiNode.PollInterval()
	if pollInterval == 0 {
		return nil, nil, errors.New("PollInterval is 0")
	}
	timeout := pollInterval
	poller, channel := mn.NewPoller[*Head](pollInterval, m.LatestBlock, timeout, m.log)
	if err := poller.Start(ctx); err != nil {
		return nil, nil, err
	}

	err := m.registerSub(&poller, chStopInFlight)
	if err != nil {
		poller.Unsubscribe()
		return nil, nil, err
	}

	return channel, &poller, nil
}

func (m *MultiNodeClient) SubscribeToFinalizedHeads(ctx context.Context) (<-chan *Head, mn.Subscription, error) {
	ctx, cancel, chStopInFlight, _ := m.acquireQueryCtx(ctx, m.contextDuration)
	defer cancel()

	finalizedBlockPollInterval := m.cfg.MultiNode.FinalizedBlockPollInterval()
	if finalizedBlockPollInterval == 0 {
		return nil, nil, errors.New("FinalizedBlockPollInterval is 0")
	}
	timeout := finalizedBlockPollInterval
	poller, channel := mn.NewPoller[*Head](finalizedBlockPollInterval, m.LatestFinalizedBlock, timeout, m.log)
	if err := poller.Start(ctx); err != nil {
		return nil, nil, err
	}

	err := m.registerSub(&poller, chStopInFlight)
	if err != nil {
		poller.Unsubscribe()
		return nil, nil, err
	}

	return channel, &poller, nil
}

func (m *MultiNodeClient) LatestBlock(ctx context.Context) (*Head, error) {
	// capture chStopInFlight to ensure we are not updating chainInfo with observations related to previous life cycle
	ctx, cancel, chStopInFlight, rawRPC := m.acquireQueryCtx(ctx, m.contextDuration)
	defer cancel()

	result, err := rawRPC.GetLatestBlockhash(ctx, rpc.CommitmentConfirmed)
	if err != nil {
		return nil, err
	}

	head := &Head{
		BlockHeight: &result.Value.LastValidBlockHeight,
		BlockHash:   &result.Value.Blockhash,
	}
	m.onNewHead(ctx, chStopInFlight, head)
	return head, nil
}

func (m *MultiNodeClient) LatestFinalizedBlock(ctx context.Context) (*Head, error) {
	ctx, cancel, chStopInFlight, rawRPC := m.acquireQueryCtx(ctx, m.contextDuration)
	defer cancel()

	result, err := rawRPC.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return nil, err
	}

	head := &Head{
		BlockHeight: &result.Value.LastValidBlockHeight,
		BlockHash:   &result.Value.Blockhash,
	}
	m.onNewFinalizedHead(ctx, chStopInFlight, head)
	return head, nil
}

func (m *MultiNodeClient) onNewHead(ctx context.Context, requestCh <-chan struct{}, head *Head) {
	if head == nil {
		return
	}

	m.chainInfoLock.Lock()
	defer m.chainInfoLock.Unlock()
	if !mn.CtxIsHeathCheckRequest(ctx) {
		m.highestUserObservations.BlockNumber = max(m.highestUserObservations.BlockNumber, head.BlockNumber())
	}
	select {
	case <-requestCh: // no need to update latestChainInfo, as rpcClient already started new life cycle
		return
	default:
		m.latestChainInfo.BlockNumber = head.BlockNumber()
	}
}

func (m *MultiNodeClient) onNewFinalizedHead(ctx context.Context, requestCh <-chan struct{}, head *Head) {
	if head == nil {
		return
	}
	m.chainInfoLock.Lock()
	defer m.chainInfoLock.Unlock()
	if !mn.CtxIsHeathCheckRequest(ctx) {
		m.highestUserObservations.FinalizedBlockNumber = max(m.highestUserObservations.FinalizedBlockNumber, head.BlockNumber())
	}
	select {
	case <-requestCh: // no need to update latestChainInfo, as rpcClient already started new life cycle
		return
	default:
		m.latestChainInfo.FinalizedBlockNumber = head.BlockNumber()
	}
}

// makeQueryCtx returns a context that cancels if:
// 1. Passed in ctx cancels
// 2. Passed in channel is closed
// 3. Default timeout is reached (queryTimeout)
func makeQueryCtx(ctx context.Context, ch services.StopChan, timeout time.Duration) (context.Context, context.CancelFunc) {
	var chCancel, timeoutCancel context.CancelFunc
	ctx, chCancel = ch.Ctx(ctx)
	ctx, timeoutCancel = context.WithTimeout(ctx, timeout)
	cancel := func() {
		chCancel()
		timeoutCancel()
	}
	return ctx, cancel
}

func (m *MultiNodeClient) acquireQueryCtx(parentCtx context.Context, timeout time.Duration) (ctx context.Context, cancel context.CancelFunc,
	chStopInFlight chan struct{}, raw *rpc.Client) {
	// Need to wrap in mutex because state transition can cancel and replace context
	m.stateMu.RLock()
	chStopInFlight = m.chStopInFlight
	cp := *m.rpc
	raw = &cp
	m.stateMu.RUnlock()
	ctx, cancel = makeQueryCtx(parentCtx, chStopInFlight, timeout)
	return
}

func (m *MultiNodeClient) Ping(ctx context.Context) error {
	version, err := m.rpc.GetVersion(ctx)
	if err != nil {
		return fmt.Errorf("ping failed: %v", err)
	}
	m.log.Debugf("ping client version: %s", version.SolanaCore)
	return err
}

func (m *MultiNodeClient) IsSyncing(ctx context.Context) (bool, error) {
	// Not in use for Solana
	return false, nil
}

func (m *MultiNodeClient) UnsubscribeAllExcept(subs ...mn.Subscription) {
	m.subsSliceMu.Lock()
	defer m.subsSliceMu.Unlock()

	keepSubs := map[mn.Subscription]struct{}{}
	for _, sub := range subs {
		keepSubs[sub] = struct{}{}
	}

	for sub := range m.subs {
		if _, keep := keepSubs[sub]; !keep {
			sub.Unsubscribe()
			delete(m.subs, sub)
		}
	}
}

// cancelInflightRequests closes and replaces the chStopInFlight
func (m *MultiNodeClient) cancelInflightRequests() {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	close(m.chStopInFlight)
	m.chStopInFlight = make(chan struct{})
}

func (m *MultiNodeClient) Close() {
	defer func() {
		err := m.rpc.Close()
		if err != nil {
			m.log.Errorf("error closing rpc: %v", err)
		}
	}()
	m.cancelInflightRequests()
	m.UnsubscribeAllExcept()
	m.chainInfoLock.Lock()
	m.latestChainInfo = mn.ChainInfo{}
	m.chainInfoLock.Unlock()
}

func (m *MultiNodeClient) GetInterceptedChainInfo() (latest, highestUserObservations mn.ChainInfo) {
	m.chainInfoLock.Lock()
	defer m.chainInfoLock.Unlock()
	return m.latestChainInfo, m.highestUserObservations
}

type SendTxResult struct {
	err   error
	txErr error
	code  mn.SendTxReturnCode
	sig   solana.Signature
}

var _ mn.SendTxResult = (*SendTxResult)(nil)

func NewSendTxResult(err error) *SendTxResult {
	return &SendTxResult{
		err: err,
	}
}

func (r *SendTxResult) Error() error {
	return r.err
}

func (r *SendTxResult) TxError() error {
	return r.txErr
}

func (r *SendTxResult) Code() mn.SendTxReturnCode {
	return r.code
}

func (r *SendTxResult) Signature() solana.Signature {
	return r.sig
}

func (m *MultiNodeClient) SendTransaction(ctx context.Context, tx *solana.Transaction) *SendTxResult {
	var sendTxResult = &SendTxResult{}
	sendTxResult.sig, sendTxResult.txErr = m.SendTx(ctx, tx)
	sendTxResult.code = ClassifySendError(tx, sendTxResult.txErr)
	return sendTxResult
}
