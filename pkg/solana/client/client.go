package solana

import (
	"context"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logger"
	"go4.org/syncutil/singleflight"
)

type ReaderWriter interface {
	Writer
	Reader
}

type Reader interface {
	Balance(addr solana.PublicKey) (uint64, error)
	SlotHeight() (uint64, error)
	AccountInfo(addr solana.PublicKey, opts *rpc.GetAccountInfoOpts) (*rpc.GetAccountInfoResult, error)
	LatestBlockhash(commitment rpc.CommitmentType) (*rpc.GetLatestBlockhashResult, error)
}

type Writer interface {
	SendTx(tx *solana.Transaction) (solana.Signature, error)
}

var _ ReaderWriter = (*Client)(nil)

type Client struct {
	rpc             *rpc.Client
	skipPreflight   bool // to enable or disable preflight checks
	commitment      rpc.CommitmentType
	txTimeout       time.Duration
	contextDuration time.Duration
	log             logger.Logger

	// provides a duplicate function call suppression mechanism
	requestGroup *singleflight.Group
}

func NewClient(endpoint string, cfg config.Config, requestTimeout time.Duration, log logger.Logger) (*Client, error) {
	return &Client{
		rpc:             rpc.New(endpoint),
		skipPreflight:   !cfg.UsePreflight(),
		commitment:      cfg.Commitment(),
		txTimeout:       cfg.TxTimeout(),
		contextDuration: requestTimeout,
		log:             log,
		requestGroup:    &singleflight.Group{},
	}, nil
}

func (c *Client) Balance(addr solana.PublicKey) (uint64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.contextDuration)
	defer cancel()

	v, err, _ := c.requestGroup.Do("GetBalance", func() (interface{}, error) {
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
	if err != nil {
		return 0, err
	}
	return v.(uint64), nil
}

func (c *Client) AccountInfo(addr solana.PublicKey, opts *rpc.GetAccountInfoOpts) (*rpc.GetAccountInfoResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.contextDuration)
	defer cancel()
	opts.Commitment = c.commitment // use client commitment type
	return c.rpc.GetAccountInfoWithOpts(ctx, addr, opts)
}

func (c *Client) LatestBlockhash(commitment rpc.CommitmentType) (*rpc.GetLatestBlockhashResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.contextDuration)
	defer cancel()
	return c.rpc.GetLatestBlockhash(ctx, c.commitment)
}

func (c *Client) SendTx(tx *solana.Transaction) (solana.Signature, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.txTimeout)
	defer cancel()
	return c.rpc.SendTransactionWithOpts(ctx, tx, c.skipPreflight, c.commitment)
}
