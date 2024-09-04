package client

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"golang.org/x/sync/singleflight"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	mn "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/multinode"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/monitor"
)

var _ ReaderWriter = (*RpcClient)(nil)

type Head struct {
	rpc.GetBlockResult
}

func (h *Head) BlockNumber() int64 {
	if h.BlockHeight == nil {
		return 0
	}
	return int64(*h.BlockHeight)
}

func (h *Head) BlockDifficulty() *big.Int {
	return nil
}

func (h *Head) IsValid() bool {
	return true
}

type RpcClient struct {
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
}

// TODO: BCI-4061: Implement RPC Client for MultiNode

func (c *RpcClient) Dial(ctx context.Context) error {
	//TODO implement me
	panic("implement me")
}

func (c *RpcClient) SubscribeToHeads(ctx context.Context) (<-chan *Head, mn.Subscription, error) {
	//TODO implement me
	panic("implement me")
}

func (c *RpcClient) SubscribeToFinalizedHeads(ctx context.Context) (<-chan *Head, mn.Subscription, error) {
	//TODO implement me
	panic("implement me")
}

func (c *RpcClient) Ping(ctx context.Context) error {
	//TODO implement me
	panic("implement me")
}

func (c *RpcClient) IsSyncing(ctx context.Context) (bool, error) {
	//TODO implement me
	panic("implement me")
}

func (c *RpcClient) UnsubscribeAllExcept(subs ...mn.Subscription) {
	//TODO implement me
	panic("implement me")
}

func (c *RpcClient) Close() {
	//TODO implement me
	panic("implement me")
}

func (c *RpcClient) GetInterceptedChainInfo() (latest, highestUserObservations mn.ChainInfo) {
	//TODO implement me
	panic("implement me")
}

func NewRpcClient(endpoint string, cfg config.Config, requestTimeout time.Duration, log logger.Logger) (*RpcClient, error) {
	return &RpcClient{
		url:             endpoint,
		rpc:             rpc.New(endpoint),
		skipPreflight:   cfg.SkipPreflight(),
		commitment:      cfg.Commitment(),
		maxRetries:      cfg.MaxRetries(),
		txTimeout:       cfg.TxTimeout(),
		contextDuration: requestTimeout,
		log:             log,
		requestGroup:    &singleflight.Group{},
	}, nil
}

func (c *RpcClient) latency(name string) func() {
	start := time.Now()
	return func() {
		monitor.SetClientLatency(time.Since(start), name, c.url)
	}
}

func (c *RpcClient) Balance(addr solana.PublicKey) (uint64, error) {
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

func (c *RpcClient) SlotHeight() (uint64, error) {
	return c.SlotHeightWithCommitment(rpc.CommitmentProcessed) // get the latest slot height
}

func (c *RpcClient) SlotHeightWithCommitment(commitment rpc.CommitmentType) (uint64, error) {
	done := c.latency("slot_height")
	defer done()

	ctx, cancel := context.WithTimeout(context.Background(), c.contextDuration)
	defer cancel()
	v, err, _ := c.requestGroup.Do("GetSlotHeight", func() (interface{}, error) {
		return c.rpc.GetSlot(ctx, commitment)
	})
	return v.(uint64), err
}

func (c *RpcClient) GetAccountInfoWithOpts(ctx context.Context, addr solana.PublicKey, opts *rpc.GetAccountInfoOpts) (*rpc.GetAccountInfoResult, error) {
	done := c.latency("account_info")
	defer done()

	ctx, cancel := context.WithTimeout(ctx, c.contextDuration)
	defer cancel()
	opts.Commitment = c.commitment // overrides passed in value - use defined client commitment type
	return c.rpc.GetAccountInfoWithOpts(ctx, addr, opts)
}

func (c *RpcClient) LatestBlockhash() (*rpc.GetLatestBlockhashResult, error) {
	done := c.latency("latest_blockhash")
	defer done()

	ctx, cancel := context.WithTimeout(context.Background(), c.contextDuration)
	defer cancel()

	v, err, _ := c.requestGroup.Do("GetLatestBlockhash", func() (interface{}, error) {
		return c.rpc.GetLatestBlockhash(ctx, c.commitment)
	})
	return v.(*rpc.GetLatestBlockhashResult), err
}

func (c *RpcClient) ChainID(ctx context.Context) (mn.StringID, error) {
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

func (c *RpcClient) GetFeeForMessage(msg string) (uint64, error) {
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
func (c *RpcClient) SignatureStatuses(ctx context.Context, sigs []solana.Signature) ([]*rpc.SignatureStatusesResult, error) {
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
func (c *RpcClient) SimulateTx(ctx context.Context, tx *solana.Transaction, opts *rpc.SimulateTransactionOpts) (*rpc.SimulateTransactionResult, error) {
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

func (c *RpcClient) SendTransaction(ctx context.Context, tx *solana.Transaction) error {
	// TODO: Implement
	return nil
}

func (c *RpcClient) SendTx(ctx context.Context, tx *solana.Transaction) (solana.Signature, error) {
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

func (c *RpcClient) GetLatestBlock() (*rpc.GetBlockResult, error) {
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
