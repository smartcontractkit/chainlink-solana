package client

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

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
	return h != nil && h.BlockHeight != nil && *h.BlockHeight > 0 && h.BlockHash != nil
}

var _ mn.RPCClient[mn.StringID, *Head] = (*MultiNodeClient)(nil)
var _ mn.SendTxRPCClient[*solana.Transaction, *SendTxResult] = (*MultiNodeClient)(nil)

type MultiNodeClient struct {
	Client
	*mn.MultiNodeClient[rpc.Client, *Head]
	cfg *config.TOMLConfig
}

func NewMultiNodeClient(endpoint string, cfg *config.TOMLConfig, requestTimeout time.Duration, log logger.Logger) (*MultiNodeClient, error) {
	client, err := NewClient(endpoint, cfg, requestTimeout, log)
	if err != nil {
		return nil, err
	}
	multiNodeClient, err := mn.NewMultiNodeClient[rpc.Client, *Head](
		&cfg.MultiNode, client.rpc, client.contextDuration, client.log, LatestBlock, LatestFinalizedBlock)
	if err != nil {
		return nil, err
	}
	return &MultiNodeClient{
		Client:          *client,
		MultiNodeClient: multiNodeClient,
		cfg:             cfg,
	}, nil
}

func (m *MultiNodeClient) Dial(ctx context.Context) error {
	// Not relevant for Solana as the RPCs don't need to be dialled.m
	return nil
}

func LatestBlock(ctx context.Context, rawRPC *rpc.Client) (*Head, error) {
	result, err := rawRPC.GetLatestBlockhash(ctx, rpc.CommitmentConfirmed)
	if err != nil {
		return nil, err
	}
	return &Head{
		BlockHeight: &result.Value.LastValidBlockHeight,
		BlockHash:   &result.Value.Blockhash,
	}, nil
}

func LatestFinalizedBlock(ctx context.Context, rawRPC *rpc.Client) (*Head, error) {
	result, err := rawRPC.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return nil, err
	}
	return &Head{
		BlockHeight: &result.Value.LastValidBlockHeight,
		BlockHash:   &result.Value.Blockhash,
	}, nil
}

func (m *MultiNodeClient) Ping(ctx context.Context) error {
	version, err := m.Client.rpc.GetVersion(ctx)
	if err != nil {
		return fmt.Errorf("ping failed: %v", err)
	}
	m.Client.log.Debugf("ping client version: %s", version.SolanaCore)
	return err
}

func (m *MultiNodeClient) IsSyncing(ctx context.Context) (bool, error) {
	// Not in use for Solana
	return false, nil
}

func (m *MultiNodeClient) Close() {
	defer func() {
		err := m.Client.rpc.Close()
		if err != nil {
			m.Client.log.Errorf("error closing rpc: %v", err)
		}
	}()
	m.MultiNodeClient.Close()
}

type SendTxResult struct {
	err  error
	code mn.SendTxReturnCode
	sig  solana.Signature
}

var _ mn.SendTxResult = (*SendTxResult)(nil)

func NewSendTxResult(err error) *SendTxResult {
	result := &SendTxResult{
		err: err,
	}
	result.code = ClassifySendError(nil, err)
	return result
}

func (r *SendTxResult) Error() error {
	return r.err
}

func (r *SendTxResult) Code() mn.SendTxReturnCode {
	return r.code
}

func (r *SendTxResult) Signature() solana.Signature {
	return r.sig
}

func (m *MultiNodeClient) SendTransaction(ctx context.Context, tx *solana.Transaction) *SendTxResult {
	var sendTxResult = &SendTxResult{}
	sendTxResult.sig, sendTxResult.err = m.SendTx(ctx, tx)
	sendTxResult.code = ClassifySendError(tx, sendTxResult.err)
	return sendTxResult
}
