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

// MaxSupportTransactionVersion defines max transaction version to return in responses.
// If the requested block contains a transaction with a higher version, an error will be returned.
const MaxSupportTransactionVersion = uint64(0) // (legacy + v0)

type ReaderWriter interface {
	Writer
	Reader
}

type Reader interface {
	AccountReader
	Balance(ctx context.Context, addr solana.PublicKey) (uint64, error)
	BalanceWithCommitment(ctx context.Context, addr solana.PublicKey, commitment rpc.CommitmentType) (uint64, error)
	SlotHeight(ctx context.Context) (uint64, error)
	LatestBlockhash(ctx context.Context) (*rpc.GetLatestBlockhashResult, error)
	ChainID(ctx context.Context) (mn.StringID, error)
	GetFeeForMessage(ctx context.Context, msg string) (uint64, error)
	GetFirstAvailableBlock(ctx context.Context) (out uint64, err error)
	GetLatestBlock(ctx context.Context) (*rpc.GetBlockResult, error)
	// GetLatestBlockHeight returns the latest block height of the node based on the configured commitment type
	GetLatestBlockHeight(ctx context.Context) (uint64, error)
	GetTransaction(ctx context.Context, txHash solana.Signature) (*rpc.GetTransactionResult, error)
	GetBlocks(ctx context.Context, startSlot uint64, endSlot *uint64) (rpc.BlocksResult, error)
	GetBlocksWithLimit(ctx context.Context, startSlot uint64, limit uint64) (*rpc.BlocksResult, error)
	GetBlockWithOpts(context.Context, uint64, *rpc.GetBlockOpts) (*rpc.GetBlockResult, error)
	GetBlock(ctx context.Context, slot uint64) (*rpc.GetBlockResult, error)
	GetSignaturesForAddressWithOpts(ctx context.Context, addr solana.PublicKey, opts *rpc.GetSignaturesForAddressOpts) ([]*rpc.TransactionSignature, error)
	SlotHeightWithCommitment(ctx context.Context, commitment rpc.CommitmentType) (uint64, error)
}

// AccountReader is an interface that allows users to pass either the solana rpc client or the relay client
type AccountReader interface {
	GetAccountInfoWithOpts(ctx context.Context, addr solana.PublicKey, opts *rpc.GetAccountInfoOpts) (*rpc.GetAccountInfoResult, error)
	GetMultipleAccountsWithOpts(ctx context.Context, accounts []solana.PublicKey, opts *rpc.GetMultipleAccountsOpts) (out *rpc.GetMultipleAccountsResult, err error)
	GetAccountDataBorshInto(ctx context.Context, addr solana.PublicKey, accountType any) (err error)
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
	chainID         string
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
func NewTestClient(endpoint string, cfg *config.TOMLConfig, requestTimeout time.Duration, log logger.Logger) (*Client, *rpc.Client, error) {
	var chainID string
	if cfg.ChainID != nil {
		chainID = *cfg.ChainID
	}

	rpcClient := Client{
		url:             endpoint,
		chainID:         chainID,
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

func NewClient(endpoint string, cfg *config.TOMLConfig, requestTimeout time.Duration, log logger.Logger) (*Client, error) {
	rpcClient, _, err := NewTestClient(endpoint, cfg, requestTimeout, log)
	return rpcClient, err
}

func (c *Client) latency(name string) func(err error) {
	start := time.Now()
	return func(err error) {
		monitor.SetClientLatency(c.chainID, time.Since(start), name, c.url, err)
	}
}

func (c *Client) Balance(ctx context.Context, addr solana.PublicKey) (bal uint64, err error) {
	done := c.latency("balance")
	defer func() { done(err) }()

	ctx, cancel := context.WithTimeout(ctx, c.contextDuration)
	defer cancel()

	v, err, _ := c.requestGroup.Do(fmt.Sprintf("GetBalance(%s)", addr.String()), func() (interface{}, error) {
		return c.rpc.GetBalance(ctx, addr, c.commitment)
	})
	if err != nil {
		return 0, err
	}
	res, ok := v.(*rpc.GetBalanceResult)
	if !ok {
		return 0, fmt.Errorf("result is unexpected type %T, expected %T", v, &rpc.GetBalanceResult{})
	}
	return res.Value, err
}

func (c *Client) BalanceWithCommitment(ctx context.Context, addr solana.PublicKey, commitment rpc.CommitmentType) (bal uint64, err error) {
	done := c.latency("balance")
	defer func() { done(err) }()

	ctx, cancel := context.WithTimeout(ctx, c.contextDuration)
	defer cancel()

	v, err, _ := c.requestGroup.Do(fmt.Sprintf("GetBalance(%s)", addr.String()), func() (interface{}, error) {
		return c.rpc.GetBalance(ctx, addr, commitment)
	})

	if err != nil {
		return 0, err
	}
	res, ok := v.(*rpc.GetBalanceResult)
	if !ok {
		return 0, fmt.Errorf("result is unexpected type %T, expected %T", v, &rpc.GetBalanceResult{})
	}
	return res.Value, err
}

func (c *Client) SlotHeight(ctx context.Context) (uint64, error) {
	return c.SlotHeightWithCommitment(ctx, rpc.CommitmentProcessed) // get the latest slot height
}

func (c *Client) SlotHeightWithCommitment(ctx context.Context, commitment rpc.CommitmentType) (height uint64, err error) {
	done := c.latency("slot_height")
	defer func() { done(err) }()

	ctx, cancel := context.WithTimeout(ctx, c.contextDuration)
	defer cancel()

	// Include the commitment in the requestGroup key so calls with different commitments won't be merged
	key := fmt.Sprintf("GetSlotHeight(%s)", commitment)
	v, err, _ := c.requestGroup.Do(key, func() (interface{}, error) {
		return c.rpc.GetSlot(ctx, commitment)
	})
	res, ok := v.(uint64)
	if !ok {
		return 0, fmt.Errorf("result is unexpected type %T, expected uint64", v)
	}
	return res, err
}

func (c *Client) GetSignaturesForAddressWithOpts(ctx context.Context, addr solana.PublicKey, opts *rpc.GetSignaturesForAddressOpts) (sigs []*rpc.TransactionSignature, err error) {
	done := c.latency("signatures_for_address")
	defer func() { done(err) }()

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

func (c *Client) GetTransaction(ctx context.Context, txHash solana.Signature) (tx *rpc.GetTransactionResult, err error) {
	done := c.latency("transaction")
	defer func() { done(err) }()
	ctx, cancel := context.WithTimeout(ctx, c.contextDuration)
	defer cancel()

	// Use txHash in the key so different signatures won't be merged on concurrent calls.
	key := fmt.Sprintf("GetTransaction(%s)", txHash.String())
	v, err, _ := c.requestGroup.Do(key, func() (interface{}, error) {
		version := MaxSupportTransactionVersion
		return c.rpc.GetTransaction(ctx, txHash, &rpc.GetTransactionOpts{Encoding: solana.EncodingBase64, Commitment: c.commitment, MaxSupportedTransactionVersion: &version})
	})
	res, ok := v.(*rpc.GetTransactionResult)
	if !ok {
		return nil, fmt.Errorf("result is unexpected type %T, expected %T", v, &rpc.GetTransactionResult{})
	}
	return res, err
}

func (c *Client) GetAccountInfoWithOpts(ctx context.Context, addr solana.PublicKey, opts *rpc.GetAccountInfoOpts) (result *rpc.GetAccountInfoResult, err error) {
	done := c.latency("account_info")
	defer func() { done(err) }()

	ctx, cancel := context.WithTimeout(ctx, c.contextDuration)
	defer cancel()
	opts.Commitment = c.commitment // overrides passed in value - use defined client commitment type
	return c.rpc.GetAccountInfoWithOpts(ctx, addr, opts)
}

func (c *Client) GetMultipleAccountsWithOpts(ctx context.Context, accounts []solana.PublicKey, opts *rpc.GetMultipleAccountsOpts) (out *rpc.GetMultipleAccountsResult, err error) {
	done := c.latency("multiple_account_info")
	defer func() { done(err) }()

	ctx, cancel := context.WithTimeout(ctx, c.contextDuration)
	defer cancel()
	opts.Commitment = c.commitment // overrides passed in value - use defined client commitment type
	return c.rpc.GetMultipleAccountsWithOpts(ctx, accounts, opts)
}

func (c *Client) GetAccountDataBorshInto(ctx context.Context, addr solana.PublicKey, inVar interface{}) (err error) {
	done := c.latency("get_account_data_borsh_into")
	defer func() { done(err) }()

	ctx, cancel := context.WithTimeout(ctx, c.txTimeout)
	defer cancel()

	err = c.rpc.GetAccountDataBorshInto(ctx, addr, &inVar)
	if err != nil {
		return fmt.Errorf("failed to get account data: %w", err)
	}

	return nil
}

func (c *Client) GetFirstAvailableBlock(ctx context.Context) (out uint64, err error) {
	done := c.latency("first_available_block")
	defer func() { done(err) }()

	ctx, cancel := context.WithTimeout(ctx, c.contextDuration)
	defer cancel()
	return c.rpc.GetFirstAvailableBlock(ctx)
}

func (c *Client) GetBlocks(ctx context.Context, startSlot uint64, endSlot *uint64) (out rpc.BlocksResult, err error) {
	done := c.latency("blocks")
	defer func() { done(err) }()

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
	res, ok := v.(rpc.BlocksResult)
	if !ok {
		return nil, fmt.Errorf("result is unexpected type %T, expected %T", v, rpc.BlocksResult{})
	}
	return res, err
}

func (c *Client) LatestBlockhash(ctx context.Context) (result *rpc.GetLatestBlockhashResult, err error) {
	done := c.latency("latest_blockhash")
	defer func() { done(err) }()

	ctx, cancel := context.WithTimeout(ctx, c.contextDuration)
	defer cancel()

	v, err, _ := c.requestGroup.Do("GetLatestBlockhash", func() (interface{}, error) {
		return c.rpc.GetLatestBlockhash(ctx, c.commitment)
	})
	res, ok := v.(*rpc.GetLatestBlockhashResult)
	if !ok {
		return nil, fmt.Errorf("result is unexpected type %T, expected %T", v, &rpc.GetLatestBlockhashResult{})
	}
	return res, err
}

func (c *Client) ChainID(ctx context.Context) (chainID mn.StringID, err error) {
	done := c.latency("chain_id")
	defer func() { done(err) }()

	ctx, cancel := context.WithTimeout(ctx, c.contextDuration)
	defer cancel()

	v, err, _ := c.requestGroup.Do("GetGenesisHash", func() (interface{}, error) {
		return c.rpc.GetGenesisHash(ctx)
	})
	if err != nil {
		return "", err
	}

	res, ok := v.(solana.Hash)
	if !ok {
		return "", fmt.Errorf("result is unexpected type %T, expected %T", v, solana.Hash{})
	}
	return mn.StringID(res.String()), nil
}

func (c *Client) GetFeeForMessage(ctx context.Context, msg string) (fee uint64, err error) {
	done := c.latency("fee_for_message")
	defer func() { done(err) }()

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
func (c *Client) SignatureStatuses(ctx context.Context, sigs []solana.Signature) (statuses []*rpc.SignatureStatusesResult, err error) {
	done := c.latency("signature_statuses")
	defer func() { done(err) }()

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
func (c *Client) SimulateTx(ctx context.Context, tx *solana.Transaction, opts *rpc.SimulateTransactionOpts) (result *rpc.SimulateTransactionResult, err error) {
	done := c.latency("simulate_tx")
	defer func() { done(err) }()

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

func (c *Client) SendTx(ctx context.Context, tx *solana.Transaction) (sig solana.Signature, err error) {
	done := c.latency("send_tx")
	defer func() { done(err) }()

	ctx, cancel := context.WithTimeout(ctx, c.txTimeout)
	defer cancel()

	opts := rpc.TransactionOpts{
		SkipPreflight:       c.skipPreflight,
		PreflightCommitment: c.commitment,
		MaxRetries:          c.maxRetries,
	}

	return c.rpc.SendTransactionWithOpts(ctx, tx, opts)
}

func (c *Client) GetLatestBlock(ctx context.Context) (result *rpc.GetBlockResult, err error) {
	// get latest confirmed slot
	slot, err := c.SlotHeightWithCommitment(ctx, c.commitment)
	if err != nil {
		return nil, fmt.Errorf("GetLatestBlock.SlotHeight: %w", err)
	}

	// get block based on slot
	done := c.latency("latest_block")
	defer func() { done(err) }()
	return c.GetBlock(ctx, slot)
}

// GetLatestBlockHeight returns the latest block height of the node based on the configured commitment type
func (c *Client) GetLatestBlockHeight(ctx context.Context) (height uint64, err error) {
	done := c.latency("latest_block_height")
	defer func() { done(err) }()
	ctx, cancel := context.WithTimeout(ctx, c.txTimeout)
	defer cancel()

	v, err, _ := c.requestGroup.Do("GetBlockHeight", func() (interface{}, error) {
		return c.rpc.GetBlockHeight(ctx, c.commitment)
	})
	res, ok := v.(uint64)
	if !ok {
		return 0, fmt.Errorf("result is unexpected type %T, expected uint64", v)
	}
	return res, err
}

func (c *Client) GetBlockWithOpts(ctx context.Context, slot uint64, opts *rpc.GetBlockOpts) (result *rpc.GetBlockResult, err error) {
	// get block based on slot with custom options set
	done := c.latency("get_block_with_opts")
	defer func() { done(err) }()
	ctx, cancel := context.WithTimeout(ctx, c.txTimeout)
	defer cancel()
	return c.rpc.GetBlockWithOpts(ctx, slot, opts)
}

func (c *Client) GetBlock(ctx context.Context, slot uint64) (result *rpc.GetBlockResult, err error) {
	done := c.latency("get_block")
	defer func() { done(err) }()
	ctx, cancel := context.WithTimeout(ctx, c.txTimeout)
	defer cancel()
	// Adding slot to the key so concurrent calls to GetBlock for different slots are not merged. Without including the slot,
	// it would treat all GetBlock calls as identical and merge them, returning whichever block it fetched first to all callers.
	key := fmt.Sprintf("GetBlockWithOpts(%d)", slot)
	v, err, _ := c.requestGroup.Do(key, func() (interface{}, error) {
		version := MaxSupportTransactionVersion
		return c.rpc.GetBlockWithOpts(ctx, slot, &rpc.GetBlockOpts{
			Commitment:                     c.commitment,
			MaxSupportedTransactionVersion: &version,
		})
	})
	res, ok := v.(*rpc.GetBlockResult)
	if !ok {
		return nil, fmt.Errorf("result is unexpected type %T, expected %T", v, &rpc.GetBlockResult{})
	}
	return res, err
}

func (c *Client) GetBlocksWithLimit(ctx context.Context, startSlot uint64, limit uint64) (result *rpc.BlocksResult, err error) {
	done := c.latency("get_blocks_with_limit")
	defer func() { done(err) }()

	ctx, cancel := context.WithTimeout(ctx, c.txTimeout)
	defer cancel()

	// Incorporate startSlot and limit into the key to differentiate on concurrent calls.
	key := fmt.Sprintf("GetBlocksWithLimit(%d,%d)", startSlot, limit)
	v, err, _ := c.requestGroup.Do(key, func() (interface{}, error) {
		return c.rpc.GetBlocksWithLimit(ctx, startSlot, limit, c.commitment)
	})
	res, ok := v.(*rpc.BlocksResult)
	if !ok {
		return nil, fmt.Errorf("result is unexpected type %T, expected %T", v, &rpc.BlocksResult{})
	}
	return res, err
}
