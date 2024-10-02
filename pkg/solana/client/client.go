package client

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	mn "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/multinode"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"golang.org/x/sync/singleflight"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/monitor"
)

const (
	DevnetGenesisHash  = "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG"
	TestnetGenesisHash = "4uhcVJyU9pJkvQyS88uRDiswHXSCkY3zQawwpjk2NsNY"
	MainnetGenesisHash = "5eykt4UsFv8P8NJdTREpY1vzqKqZKvdpKuc147dw2N9d"
)

//go:generate mockery --name ReaderWriter --output ./mocks/
type ReaderWriter interface {
	Writer
	Reader
}

type Reader interface {
	AccountReader
	Balance(addr solana.PublicKey) (uint64, error)
	SlotHeight() (uint64, error)
	LatestBlockhash() (*rpc.GetLatestBlockhashResult, error)
	ChainID(ctx context.Context) (mn.StringID, error)
	GetFeeForMessage(msg string) (uint64, error)
	GetLatestBlock() (*rpc.GetBlockResult, error)
}

// AccountReader is an interface that allows users to pass either the solana rpc client or the relay client
type AccountReader interface {
	GetAccountInfoWithOpts(ctx context.Context, addr solana.PublicKey, opts *rpc.GetAccountInfoOpts) (*rpc.GetAccountInfoResult, error)
}

type Writer interface {
	SendTx(ctx context.Context, tx *solana.Transaction) (solana.Signature, error)
	SimulateTx(ctx context.Context, tx *solana.Transaction, opts *rpc.SimulateTransactionOpts) (*rpc.SimulateTransactionResult, error)
	SignatureStatuses(ctx context.Context, sigs []solana.Signature) ([]*rpc.SignatureStatusesResult, error)
}

var _ ReaderWriter = (*Client)(nil)

type Client struct {
	url             string
	rpc             *rpc.Client
	skipPreflight   bool // to enable or disable preflight checks
	commitment      rpc.CommitmentType
	maxRetries      *uint
	txTimeout       time.Duration
	contextDuration time.Duration
	log             logger.Logger

	// provides a duplicate function call suppression mechanism
	requestGroup *singleflight.Group

	// MultiNode
	pollInterval               time.Duration
	finalizedBlockPollInterval time.Duration
	stateMu                    sync.RWMutex // protects state* fields
	subs                       map[mn.Subscription]struct{}

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

func NewClient(endpoint string, cfg *config.TOMLConfig, requestTimeout time.Duration, log logger.Logger) (*Client, error) {
	return &Client{
		url:                        endpoint,
		rpc:                        rpc.New(endpoint),
		skipPreflight:              cfg.SkipPreflight(),
		commitment:                 cfg.Commitment(),
		maxRetries:                 cfg.MaxRetries(),
		txTimeout:                  cfg.TxTimeout(),
		contextDuration:            requestTimeout,
		log:                        log,
		requestGroup:               &singleflight.Group{},
		pollInterval:               cfg.MultiNode.PollInterval(),
		finalizedBlockPollInterval: cfg.MultiNode.FinalizedBlockPollInterval(),
	}, nil
}

type Head struct {
	rpc.GetBlockResult
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
	return h.BlockHeight != nil
}

var _ mn.RPCClient[mn.StringID, *Head] = (*Client)(nil)
var _ mn.SendTxRPCClient[*solana.Transaction] = (*Client)(nil)

func (c *Client) Dial(ctx context.Context) error {
	return nil
}

func (c *Client) SubscribeToHeads(ctx context.Context) (<-chan *Head, mn.Subscription, error) {
	if c.pollInterval == 0 {
		return nil, nil, errors.New("PollInterval is 0")
	}
	timeout := c.pollInterval
	poller, channel := mn.NewPoller[*Head](c.pollInterval, c.LatestBlock, timeout, c.log)
	if err := poller.Start(ctx); err != nil {
		return nil, nil, err
	}
	return channel, &poller, nil
}

func (c *Client) SubscribeToFinalizedHeads(ctx context.Context) (<-chan *Head, mn.Subscription, error) {
	if c.finalizedBlockPollInterval == 0 {
		return nil, nil, errors.New("FinalizedBlockPollInterval is 0")
	}
	timeout := c.finalizedBlockPollInterval
	poller, channel := mn.NewPoller[*Head](c.finalizedBlockPollInterval, c.LatestFinalizedBlock, timeout, c.log)
	if err := poller.Start(ctx); err != nil {
		return nil, nil, err
	}
	return channel, &poller, nil
}

func (c *Client) LatestBlock(ctx context.Context) (*Head, error) {
	latestBlockHeight, err := c.rpc.GetBlockHeight(ctx, rpc.CommitmentConfirmed)
	if err != nil {
		return nil, err
	}

	block, err := c.rpc.GetBlock(ctx, latestBlockHeight)
	if err != nil {
		return nil, err
	}

	head := &Head{GetBlockResult: *block}
	c.onNewHead(ctx, c.chStopInFlight, head)
	return head, nil
}

func (c *Client) LatestFinalizedBlock(ctx context.Context) (*Head, error) {
	// TODO: Do we need this?
	// capture chStopInFlight to ensure we are not updating chainInfo with observations related to previous life cycle
	//ctx, cancel, chStopInFlight, _, _ := c.acquireQueryCtx(ctx, c.rpcTimeout)

	finalizedBlockHeight, err := c.rpc.GetBlockHeight(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return nil, err
	}

	block, err := c.rpc.GetBlock(ctx, finalizedBlockHeight)
	if err != nil {
		return nil, err
	}

	head := &Head{GetBlockResult: *block}
	c.onNewFinalizedHead(ctx, c.chStopInFlight, head)
	return head, nil
}

func (c *Client) onNewHead(ctx context.Context, requestCh <-chan struct{}, head *Head) {
	if head == nil {
		return
	}

	c.chainInfoLock.Lock()
	defer c.chainInfoLock.Unlock()
	if !mn.CtxIsHeathCheckRequest(ctx) {
		c.highestUserObservations.BlockNumber = max(c.highestUserObservations.BlockNumber, head.BlockNumber())
		c.highestUserObservations.TotalDifficulty = mn.MaxTotalDifficulty(c.highestUserObservations.TotalDifficulty, head.BlockDifficulty())
	}
	select {
	case <-requestCh: // no need to update latestChainInfo, as rpcClient already started new life cycle
		return
	default:
		c.latestChainInfo.BlockNumber = head.BlockNumber()
		c.latestChainInfo.TotalDifficulty = head.BlockDifficulty()
	}
}

func (c *Client) onNewFinalizedHead(ctx context.Context, requestCh <-chan struct{}, head *Head) {
	if head == nil {
		return
	}
	c.chainInfoLock.Lock()
	defer c.chainInfoLock.Unlock()
	if !mn.CtxIsHeathCheckRequest(ctx) {
		c.highestUserObservations.FinalizedBlockNumber = max(c.highestUserObservations.FinalizedBlockNumber, head.BlockNumber())
	}
	select {
	case <-requestCh: // no need to update latestChainInfo, as rpcClient already started new life cycle
		return
	default:
		c.latestChainInfo.FinalizedBlockNumber = head.BlockNumber()
	}
}

func (c *Client) Ping(ctx context.Context) error {
	version, err := c.rpc.GetVersion(ctx)
	if err != nil {
		return fmt.Errorf("ping failed: %v", err)
	}
	c.log.Debugf("ping client version: %s", version.SolanaCore)
	return err
}

func (c *Client) IsSyncing(ctx context.Context) (bool, error) {
	// Not in use for Solana
	return false, nil
}

func (c *Client) UnsubscribeAllExcept(subs ...mn.Subscription) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	keepSubs := map[mn.Subscription]struct{}{}
	for _, sub := range subs {
		keepSubs[sub] = struct{}{}
	}

	for sub := range c.subs {
		if _, keep := keepSubs[sub]; !keep {
			sub.Unsubscribe()
			delete(c.subs, sub)
		}
	}
}

// cancelInflightRequests closes and replaces the chStopInFlight
func (c *Client) cancelInflightRequests() {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	close(c.chStopInFlight)
	c.chStopInFlight = make(chan struct{})
}

func (c *Client) Close() {
	defer func() {
		err := c.rpc.Close()
		if err != nil {
			c.log.Errorf("error closing rpc: %v", err)
		}
	}()
	c.cancelInflightRequests()
	c.UnsubscribeAllExcept()
	c.chainInfoLock.Lock()
	c.latestChainInfo = mn.ChainInfo{}
	c.chainInfoLock.Unlock()
}

func (c *Client) GetInterceptedChainInfo() (latest, highestUserObservations mn.ChainInfo) {
	c.chainInfoLock.Lock()
	defer c.chainInfoLock.Unlock()
	return c.latestChainInfo, c.highestUserObservations
}

func (c *Client) SendTransaction(ctx context.Context, tx *solana.Transaction) error {
	// TODO: Use Transaction Sender
	_, err := c.SendTx(ctx, tx)
	return err
}

func (c *Client) latency(name string) func() {
	start := time.Now()
	return func() {
		monitor.SetClientLatency(time.Since(start), name, c.url)
	}
}

func (c *Client) Balance(addr solana.PublicKey) (uint64, error) {
	done := c.latency("balance")
	defer done()

	ctx, cancel := context.WithTimeout(context.Background(), c.contextDuration)
	defer cancel()

	v, err, _ := c.requestGroup.Do(fmt.Sprintf("GetBalance(%s)", addr.String()), func() (interface{}, error) {
		return c.rpc.GetBalance(ctx, addr, c.commitment)
	})
	if err != nil {
		return 0, err
	}
	res := v.(*rpc.GetBalanceResult)
	return res.Value, err
}

func (c *Client) SlotHeight() (uint64, error) {
	return c.SlotHeightWithCommitment(rpc.CommitmentProcessed) // get the latest slot height
}

func (c *Client) SlotHeightWithCommitment(commitment rpc.CommitmentType) (uint64, error) {
	done := c.latency("slot_height")
	defer done()

	ctx, cancel := context.WithTimeout(context.Background(), c.contextDuration)
	defer cancel()
	v, err, _ := c.requestGroup.Do("GetSlotHeight", func() (interface{}, error) {
		return c.rpc.GetSlot(ctx, commitment)
	})
	return v.(uint64), err
}

func (c *Client) GetAccountInfoWithOpts(ctx context.Context, addr solana.PublicKey, opts *rpc.GetAccountInfoOpts) (*rpc.GetAccountInfoResult, error) {
	done := c.latency("account_info")
	defer done()

	ctx, cancel := context.WithTimeout(ctx, c.contextDuration)
	defer cancel()
	opts.Commitment = c.commitment // overrides passed in value - use defined client commitment type
	return c.rpc.GetAccountInfoWithOpts(ctx, addr, opts)
}

func (c *Client) LatestBlockhash() (*rpc.GetLatestBlockhashResult, error) {
	done := c.latency("latest_blockhash")
	defer done()

	ctx, cancel := context.WithTimeout(context.Background(), c.contextDuration)
	defer cancel()

	v, err, _ := c.requestGroup.Do("GetLatestBlockhash", func() (interface{}, error) {
		return c.rpc.GetLatestBlockhash(ctx, c.commitment)
	})
	return v.(*rpc.GetLatestBlockhashResult), err
}

func (c *Client) ChainID(ctx context.Context) (mn.StringID, error) {
	done := c.latency("chain_id")
	defer done()

	ctx, cancel := context.WithTimeout(ctx, c.contextDuration)
	defer cancel()
	v, err, _ := c.requestGroup.Do("GetGenesisHash", func() (interface{}, error) {
		return c.rpc.GetGenesisHash(ctx)
	})
	if err != nil {
		return "", err
	}
	hash := v.(solana.Hash)

	var network string
	switch hash.String() {
	case DevnetGenesisHash:
		network = "devnet"
	case TestnetGenesisHash:
		network = "testnet"
	case MainnetGenesisHash:
		network = "mainnet"
	default:
		c.log.Warnf("unknown genesis hash - assuming solana chain is 'localnet'")
		network = "localnet"
	}
	return mn.StringID(network), nil
}

func (c *Client) GetFeeForMessage(msg string) (uint64, error) {
	done := c.latency("fee_for_message")
	defer done()

	// msg is base58 encoded data

	ctx, cancel := context.WithTimeout(context.Background(), c.contextDuration)
	defer cancel()
	res, err := c.rpc.GetFeeForMessage(ctx, msg, c.commitment)
	if err != nil {
		return 0, fmt.Errorf("error in GetFeeForMessage: %w", err)
	}

	if res == nil || res.Value == nil {
		return 0, errors.New("nil pointer in GetFeeForMessage")
	}
	return *res.Value, nil
}

// https://docs.solana.com/developing/clients/jsonrpc-api#getsignaturestatuses
func (c *Client) SignatureStatuses(ctx context.Context, sigs []solana.Signature) ([]*rpc.SignatureStatusesResult, error) {
	done := c.latency("signature_statuses")
	defer done()

	ctx, cancel := context.WithTimeout(ctx, c.contextDuration)
	defer cancel()

	// searchTransactionHistory = false
	res, err := c.rpc.GetSignatureStatuses(ctx, false, sigs...)
	if err != nil {
		return nil, fmt.Errorf("error in GetSignatureStatuses: %w", err)
	}

	if res == nil || res.Value == nil {
		return nil, errors.New("nil pointer in GetSignatureStatuses")
	}
	return res.Value, nil
}

// https://docs.solana.com/developing/clients/jsonrpc-api#simulatetransaction
// opts - (optional) use `nil` to use defaults
func (c *Client) SimulateTx(ctx context.Context, tx *solana.Transaction, opts *rpc.SimulateTransactionOpts) (*rpc.SimulateTransactionResult, error) {
	done := c.latency("simulate_tx")
	defer done()

	ctx, cancel := context.WithTimeout(ctx, c.contextDuration)
	defer cancel()

	if opts == nil {
		opts = &rpc.SimulateTransactionOpts{
			SigVerify:  true, // verify signature
			Commitment: c.commitment,
		}
	}

	res, err := c.rpc.SimulateTransactionWithOpts(ctx, tx, opts)
	if err != nil {
		return nil, fmt.Errorf("error in SimulateTransactionWithOpts: %w", err)
	}

	if res == nil || res.Value == nil {
		return nil, errors.New("nil pointer in SimulateTransactionWithOpts")
	}

	return res.Value, nil
}

func (c *Client) SendTx(ctx context.Context, tx *solana.Transaction) (solana.Signature, error) {
	done := c.latency("send_tx")
	defer done()

	ctx, cancel := context.WithTimeout(ctx, c.txTimeout)
	defer cancel()

	opts := rpc.TransactionOpts{
		SkipPreflight:       c.skipPreflight,
		PreflightCommitment: c.commitment,
		MaxRetries:          c.maxRetries,
	}

	return c.rpc.SendTransactionWithOpts(ctx, tx, opts)
}

func (c *Client) GetLatestBlock() (*rpc.GetBlockResult, error) {
	// get latest confirmed slot
	slot, err := c.SlotHeightWithCommitment(c.commitment)
	if err != nil {
		return nil, fmt.Errorf("GetLatestBlock.SlotHeight: %w", err)
	}

	// get block based on slot
	done := c.latency("latest_block")
	defer done()
	ctx, cancel := context.WithTimeout(context.Background(), c.txTimeout)
	defer cancel()
	v, err, _ := c.requestGroup.Do("GetBlockWithOpts", func() (interface{}, error) {
		version := uint64(0) // pull all tx types (legacy + v0)
		return c.rpc.GetBlockWithOpts(ctx, slot, &rpc.GetBlockOpts{
			Commitment:                     c.commitment,
			MaxSupportedTransactionVersion: &version,
		})
	})
	return v.(*rpc.GetBlockResult), err
}
