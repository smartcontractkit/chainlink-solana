package client

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/pkg/errors"
	"golang.org/x/sync/singleflight"

	htrktypes "github.com/smartcontractkit/chainlink-solana/pkg/common/headtracker/types"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	headtracker "github.com/smartcontractkit/chainlink-solana/pkg/solana/headtracker/types"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logger"
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
	ChainID() (string, error)
	GetFeeForMessage(msg string) (uint64, error)
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

var _ htrktypes.Client[*headtracker.Head, *Subscription, headtracker.ChainID, headtracker.Hash] = (*Client)(nil)

type Client struct {
	rpc             *rpc.Client
	skipPreflight   bool // to enable or disable preflight checks
	commitment      rpc.CommitmentType
	maxRetries      *uint
	txTimeout       time.Duration
	contextDuration time.Duration
	log             logger.Logger
	pollingInterval time.Duration

	// provides a duplicate function call suppression mechanism
	requestGroup *singleflight.Group
}

func NewClient(endpoint string, cfg config.Config, requestTimeout time.Duration, log logger.Logger) (*Client, error) {
	return &Client{
		rpc:             rpc.New(endpoint),
		skipPreflight:   cfg.SkipPreflight(),
		commitment:      cfg.Commitment(),
		maxRetries:      cfg.MaxRetries(),
		txTimeout:       cfg.TxTimeout(),
		pollingInterval: cfg.PollingInterval(), //TODO: Add this in the config in core
		contextDuration: requestTimeout,
		log:             log,
		requestGroup:    &singleflight.Group{},
	}, nil
}

func (c *Client) Balance(addr solana.PublicKey) (uint64, error) {
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
	ctx, cancel := context.WithTimeout(context.Background(), c.contextDuration)
	defer cancel()
	v, err, _ := c.requestGroup.Do("GetSlotHeight", func() (interface{}, error) {
		return c.rpc.GetSlot(ctx, rpc.CommitmentProcessed) // get the latest slot height
	})
	return v.(uint64), err
}

func (c *Client) GetAccountInfoWithOpts(ctx context.Context, addr solana.PublicKey, opts *rpc.GetAccountInfoOpts) (*rpc.GetAccountInfoResult, error) {
	ctx, cancel := context.WithTimeout(ctx, c.contextDuration)
	defer cancel()
	opts.Commitment = c.commitment // overrides passed in value - use defined client commitment type
	return c.rpc.GetAccountInfoWithOpts(ctx, addr, opts)
}

func (c *Client) LatestBlockhash() (*rpc.GetLatestBlockhashResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.contextDuration)
	defer cancel()

	v, err, _ := c.requestGroup.Do("GetLatestBlockhash", func() (interface{}, error) {
		return c.rpc.GetLatestBlockhash(ctx, c.commitment)
	})
	return v.(*rpc.GetLatestBlockhashResult), err
}

func (c *Client) ChainID() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.contextDuration)
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
	return network, nil
}

// TODO: requires refactor. Do we want to store chainID? how do we want to cache ChainID?
func (c *Client) ConfiguredChainID() headtracker.ChainID {
	chainID, err := c.ChainID()
	if err != nil {
		c.log.Warnf("unable to determine configured chain ID: %v", err)
		return headtracker.ChainID(headtracker.Localnet)
	}
	return headtracker.StringToChainID(chainID)
}

func (c *Client) GetFeeForMessage(msg string) (uint64, error) {
	// msg is base58 encoded data

	ctx, cancel := context.WithTimeout(context.Background(), c.contextDuration)
	defer cancel()
	res, err := c.rpc.GetFeeForMessage(ctx, msg, c.commitment)
	if err != nil {
		return 0, errors.Wrap(err, "error in GetFeeForMessage")
	}

	if res == nil || res.Value == nil {
		return 0, errors.New("nil pointer in GetFeeForMessage")
	}
	return *res.Value, nil
}

// https://docs.solana.com/developing/clients/jsonrpc-api#getsignaturestatuses
func (c *Client) SignatureStatuses(ctx context.Context, sigs []solana.Signature) ([]*rpc.SignatureStatusesResult, error) {
	ctx, cancel := context.WithTimeout(ctx, c.contextDuration)
	defer cancel()

	// searchTransactionHistory = false
	res, err := c.rpc.GetSignatureStatuses(ctx, false, sigs...)
	if err != nil {
		return nil, errors.Wrap(err, "error in GetSignatureStatuses")
	}

	if res == nil || res.Value == nil {
		return nil, errors.New("nil pointer in GetSignatureStatuses")
	}
	return res.Value, nil
}

// https://docs.solana.com/developing/clients/jsonrpc-api#simulatetransaction
// opts - (optional) use `nil` to use defaults
func (c *Client) SimulateTx(ctx context.Context, tx *solana.Transaction, opts *rpc.SimulateTransactionOpts) (*rpc.SimulateTransactionResult, error) {
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
		return nil, errors.Wrap(err, "error in SimulateTransactionWithOpts")
	}

	if res == nil || res.Value == nil {
		return nil, errors.New("nil pointer in SimulateTransactionWithOpts")
	}

	return res.Value, nil
}

func (c *Client) SendTx(ctx context.Context, tx *solana.Transaction) (solana.Signature, error) {
	ctx, cancel := context.WithTimeout(ctx, c.txTimeout)
	defer cancel()

	opts := rpc.TransactionOpts{
		SkipPreflight:       c.skipPreflight,
		PreflightCommitment: c.commitment,
		MaxRetries:          c.maxRetries,
	}

	return c.rpc.SendTransactionWithOpts(ctx, tx, opts)
}

func (c *Client) HeadByNumber(ctx context.Context, number *big.Int) (*headtracker.Head, error) {
	ctx, cancel := context.WithTimeout(ctx, c.contextDuration)
	defer cancel()
	block, err := c.GetBlock(ctx, number.Uint64())
	if err != nil {
		return nil, err
	}
	if block == nil {
		return nil, errors.New("invalid block in HeadByNumber")
	}
	chainId := c.ConfiguredChainID()
	// TODO: check if parent head can be linked in the headsaver
	head := &headtracker.Head{
		Slot:  number.Int64(),
		Block: *block,
		ID:    chainId,
	}
	return head, nil
}

// SubscribeNewHead polls the RPC endpoint for new blocks.
func (c *Client) SubscribeNewHead(ctx context.Context, ch chan<- *headtracker.Head) (*Subscription, error) {
	subscription := NewSubscription(ctx, c)

	go func() {
		ticker := time.NewTicker(c.pollingInterval)

		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				block, slot, err := c.getLatestBlock(ctx)
				// TODO: Improve error handling
				if err != nil {
					subscription.errChan <- err
					continue
				}

				// Create a new Head object and send to channel
				head := &headtracker.Head{
					Slot:  int64(slot),
					Block: *block,
					ID:    c.ConfiguredChainID(),
				}
				ch <- head
			}
		}
	}()

	return subscription, nil
}

// getLatestBlock queries the latest slot and returns the block.
func (c *Client) getLatestBlock(ctx context.Context) (block *rpc.GetBlockResult, slot uint64, err error) {
	ctx, cancel := context.WithTimeout(ctx, c.contextDuration)
	defer cancel()

	slot, err = c.GetLatestSlot(ctx)
	if err != nil {
		return nil, slot, errors.Wrap(err, "error in GetLatestSlot")
	}

	block, err = c.GetBlock(ctx, slot)
	if err != nil {
		return nil, slot, err
	}

	return block, slot, nil
}

func (c *Client) GetBlock(ctx context.Context, slot uint64) (out *rpc.GetBlockResult, err error) {
	ctx, cancel := context.WithTimeout(ctx, c.contextDuration)
	defer cancel()

	res, err, _ := c.requestGroup.Do("GetBlock", func() (interface{}, error) {
		return c.rpc.GetBlock(ctx, slot)
	})

	if err != nil {
		return nil, errors.Wrap(err, "error in GetBlock")
	}
	if res == nil {
		return nil, errors.New("nil pointer in GetBlock")
	}

	return res.(*rpc.GetBlockResult), err
}

// TODO: confirm commitment for RPC again. Public RPC nodes cannot handle CommitmentProcessed due to requests being too frequent.
func (c *Client) GetLatestSlot(ctx context.Context) (uint64, error) {
	ctx, cancel := context.WithTimeout(ctx, c.contextDuration)
	defer cancel()

	res, err, _ := c.requestGroup.Do("GetSlot", func() (interface{}, error) {
		return c.rpc.GetSlot(ctx, c.commitment)
	})

	if err != nil {
		return 0, errors.Wrap(err, "error in GetSlot")
	}

	if res == nil {
		return 0, errors.New("nil pointer in GetSlot")
	}

	return res.(uint64), err
}

func (c *Client) GetBlocks(ctx context.Context, startSlot, endSlot uint64) (blocks []uint64, err error) {
	ctx, cancel := context.WithTimeout(ctx, c.contextDuration)
	defer cancel()

	res, err, _ := c.requestGroup.Do("GetBlocks", func() (interface{}, error) {
		return c.rpc.GetBlocks(ctx, startSlot, &endSlot, c.commitment)
	})

	if err != nil {
		return nil, errors.Wrap(err, "error in GetBlocks")
	}
	if res == nil {
		return nil, errors.New("nil pointer in GetBlocks")
	}

	blocks = make([]uint64, len(res.(rpc.BlocksResult)))
	copy(blocks, res.(rpc.BlocksResult))

	return blocks, err
}
