package client

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"golang.org/x/sync/singleflight"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	mn "github.com/smartcontractkit/chainlink-framework/multinode"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/monitor"
)

const (
	DevnetGenesisHash  = "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG"
	TestnetGenesisHash = "4uhcVJyU9pJkvQyS88uRDiswHXSCkY3zQawwpjk2NsNY"
	MainnetGenesisHash = "5eykt4UsFv8P8NJdTREpY1vzqKqZKvdpKuc147dw2N9d"
)

type ReaderWriter interface {
	Writer
	Reader
}

type Reader interface {
	AccountReader
	Balance(ctx context.Context, addr solana.PublicKey) (uint64, error)
	SlotHeight(ctx context.Context) (uint64, error)
	LatestBlockhash(ctx context.Context) (*rpc.GetLatestBlockhashResult, error)
	ChainID(ctx context.Context) (mn.StringID, error)
	GetFeeForMessage(ctx context.Context, msg string) (uint64, error)
	GetLatestBlock(ctx context.Context) (*rpc.GetBlockResult, error)
	// GetLatestBlockHeight returns the latest block height of the node based on the configured commitment type
	GetLatestBlockHeight(ctx context.Context) (uint64, error)
	GetTransaction(ctx context.Context, txHash solana.Signature) (*rpc.GetTransactionResult, error)
	GetBlocks(ctx context.Context, startSlot uint64, endSlot *uint64) (rpc.BlocksResult, error)
	GetBlocksWithLimit(ctx context.Context, startSlot uint64, limit uint64) (*rpc.BlocksResult, error)
	GetBlockWithOpts(context.Context, uint64, *rpc.GetBlockOpts) (*rpc.GetBlockResult, error)
	GetBlock(ctx context.Context, slot uint64) (*rpc.GetBlockResult, error)
	GetSignaturesForAddressWithOpts(ctx context.Context, addr solana.PublicKey, opts *rpc.GetSignaturesForAddressOpts) ([]*rpc.TransactionSignature, error)
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
	// As a rule of thumb: if two calls passing different arguments must not be merged/deduplicated, then you must incorporate those arguments into the key.
	requestGroup *singleflight.Group
}

// Return both the client and the underlying rpc client for testing
func NewTestClient(endpoint string, cfg config.Config, requestTimeout time.Duration, log logger.Logger) (*Client, *rpc.Client, error) {
	rpcClient := Client{
		url:             endpoint,
		skipPreflight:   cfg.SkipPreflight(),
		commitment:      cfg.Commitment(),
		maxRetries:      cfg.MaxRetries(),
		txTimeout:       cfg.TxTimeout(),
		contextDuration: requestTimeout,
		log:             log,
		requestGroup:    &singleflight.Group{},
	}
	rpcClient.rpc = rpc.New(endpoint)
	return &rpcClient, rpcClient.rpc, nil
}

func NewClient(endpoint string, cfg config.Config, requestTimeout time.Duration, log logger.Logger) (*Client, error) {
	rpcClient, _, err := NewTestClient(endpoint, cfg, requestTimeout, log)
	return rpcClient, err
}

func (c *Client) latency(name string) func() {
	start := time.Now()
	return func() {
		monitor.SetClientLatency(time.Since(start), name, c.url)
	}
}

func (c *Client) Balance(ctx context.Context, addr solana.PublicKey) (uint64, error) {
	done := c.latency("balance")
	defer done()

	ctx, cancel := context.WithTimeout(ctx, c.contextDuration)
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

func (c *Client) SlotHeight(ctx context.Context) (uint64, error) {
	return c.SlotHeightWithCommitment(ctx, rpc.CommitmentProcessed) // get the latest slot height
}

func (c *Client) SlotHeightWithCommitment(ctx context.Context, commitment rpc.CommitmentType) (uint64, error) {
	done := c.latency("slot_height")
	defer done()

	ctx, cancel := context.WithTimeout(ctx, c.contextDuration)
	defer cancel()

	// Include the commitment in the requestGroup key so calls with different commitments won't be merged
	key := fmt.Sprintf("GetSlotHeight(%s)", commitment)
	v, err, _ := c.requestGroup.Do(key, func() (interface{}, error) {
		return c.rpc.GetSlot(ctx, commitment)
	})
	return v.(uint64), err
}

func (c *Client) GetSignaturesForAddressWithOpts(ctx context.Context, addr solana.PublicKey, opts *rpc.GetSignaturesForAddressOpts) ([]*rpc.TransactionSignature, error) {
	done := c.latency("signatures_for_address")
	defer done()

	ctx, cancel := context.WithTimeout(ctx, c.contextDuration)
	defer cancel()
	if opts == nil {
		opts = &rpc.GetSignaturesForAddressOpts{}
	}
	if opts.Commitment == "" {
		opts.Commitment = c.commitment
	}
	return c.rpc.GetSignaturesForAddressWithOpts(ctx, addr, opts)
}

func (c *Client) GetTransaction(ctx context.Context, txHash solana.Signature) (*rpc.GetTransactionResult, error) {
	done := c.latency("transaction")
	defer done()
	ctx, cancel := context.WithTimeout(ctx, c.contextDuration)
	defer cancel()

	// Use txHash in the key so different signatures won't be merged on concurrent calls.
	key := fmt.Sprintf("GetTransaction(%s)", txHash.String())
	v, err, _ := c.requestGroup.Do(key, func() (interface{}, error) {
		return c.rpc.GetTransaction(ctx, txHash, &rpc.GetTransactionOpts{Encoding: solana.EncodingBase64, Commitment: c.commitment})
	})
	return v.(*rpc.GetTransactionResult), err
}

func (c *Client) GetAccountInfoWithOpts(ctx context.Context, addr solana.PublicKey, opts *rpc.GetAccountInfoOpts) (*rpc.GetAccountInfoResult, error) {
	done := c.latency("account_info")
	defer done()

	ctx, cancel := context.WithTimeout(ctx, c.contextDuration)
	defer cancel()
	opts.Commitment = c.commitment // overrides passed in value - use defined client commitment type
	return c.rpc.GetAccountInfoWithOpts(ctx, addr, opts)
}

func (c *Client) GetBlocks(ctx context.Context, startSlot uint64, endSlot *uint64) (out rpc.BlocksResult, err error) {
	done := c.latency("blocks")
	defer done()

	ctx, cancel := context.WithTimeout(ctx, c.contextDuration)
	defer cancel()

	// Incorporate startSlot/endSlot into the key to differentiate concurrent calls with different ranges
	endSlotStr := "nil"
	if endSlot != nil {
		endSlotStr = fmt.Sprint(*endSlot)
	}
	key := fmt.Sprintf("GetBlocks(%d,%s)", startSlot, endSlotStr)
	v, err, _ := c.requestGroup.Do(key, func() (interface{}, error) {
		return c.rpc.GetBlocks(ctx, startSlot, endSlot, c.commitment)
	})
	return v.(rpc.BlocksResult), err
}

func (c *Client) LatestBlockhash(ctx context.Context) (*rpc.GetLatestBlockhashResult, error) {
	done := c.latency("latest_blockhash")
	defer done()

	ctx, cancel := context.WithTimeout(ctx, c.contextDuration)
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

func (c *Client) GetFeeForMessage(ctx context.Context, msg string) (uint64, error) {
	done := c.latency("fee_for_message")
	defer done()

	// msg is base58 encoded data

	ctx, cancel := context.WithTimeout(ctx, c.contextDuration)
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

func (c *Client) GetLatestBlock(ctx context.Context) (*rpc.GetBlockResult, error) {
	// get latest confirmed slot
	slot, err := c.SlotHeightWithCommitment(ctx, c.commitment)
	if err != nil {
		return nil, fmt.Errorf("GetLatestBlock.SlotHeight: %w", err)
	}

	// get block based on slot
	done := c.latency("latest_block")
	defer done()
	return c.GetBlock(ctx, slot)
}

// GetLatestBlockHeight returns the latest block height of the node based on the configured commitment type
func (c *Client) GetLatestBlockHeight(ctx context.Context) (uint64, error) {
	done := c.latency("latest_block_height")
	defer done()
	ctx, cancel := context.WithTimeout(ctx, c.txTimeout)
	defer cancel()

	v, err, _ := c.requestGroup.Do("GetBlockHeight", func() (interface{}, error) {
		return c.rpc.GetBlockHeight(ctx, c.commitment)
	})
	return v.(uint64), err
}

func (c *Client) GetBlockWithOpts(ctx context.Context, slot uint64, opts *rpc.GetBlockOpts) (*rpc.GetBlockResult, error) {
	// get block based on slot with custom options set
	done := c.latency("get_block_with_opts")
	defer done()
	ctx, cancel := context.WithTimeout(ctx, c.txTimeout)
	defer cancel()
	return c.rpc.GetBlockWithOpts(ctx, slot, opts)
}

func (c *Client) GetBlock(ctx context.Context, slot uint64) (*rpc.GetBlockResult, error) {
	done := c.latency("get_block")
	defer done()
	ctx, cancel := context.WithTimeout(ctx, c.txTimeout)
	defer cancel()

	// Adding slot to the key so concurrent calls to GetBlock for different slots are not merged. Without including the slot,
	// it would treat all GetBlock calls as identical and merge them, returning whichever block it fetched first to all callers.
	key := fmt.Sprintf("GetBlockWithOpts(%d)", slot)
	v, err, _ := c.requestGroup.Do(key, func() (interface{}, error) {
		version := uint64(0) // pull all tx types (legacy + v0)
		return c.rpc.GetBlockWithOpts(ctx, slot, &rpc.GetBlockOpts{
			Commitment:                     c.commitment,
			MaxSupportedTransactionVersion: &version,
		})
	})
	return v.(*rpc.GetBlockResult), err
}

func (c *Client) GetBlocksWithLimit(ctx context.Context, startSlot uint64, limit uint64) (*rpc.BlocksResult, error) {
	done := c.latency("get_blocks_with_limit")
	defer done()

	ctx, cancel := context.WithTimeout(ctx, c.txTimeout)
	defer cancel()

	// Incorporate startSlot and limit into the key to differentiate on concurrent calls.
	key := fmt.Sprintf("GetBlocksWithLimit(%d,%d)", startSlot, limit)
	v, err, _ := c.requestGroup.Do(key, func() (interface{}, error) {
		return c.rpc.GetBlocksWithLimit(ctx, startSlot, limit, c.commitment)
	})
	return v.(*rpc.BlocksResult), err
}
