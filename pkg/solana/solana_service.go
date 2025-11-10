package solana

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	commonsol "github.com/smartcontractkit/chainlink-common/pkg/types/chains/solana"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query/primitives"
)

type solanaService struct {
	chain  Chain
	logger logger.Logger
}

// DONE
func (ss *solanaService) GetBlock(ctx context.Context, req commonsol.GetBlockRequest) (*commonsol.GetBlockReply, error) {
	reader, err := ss.chain.Reader()
	if err != nil {
		return nil, fmt.Errorf("failed to get reader: %w", err)
	}

	result, err := reader.GetBlockWithOpts(ctx, req.Slot, &rpc.GetBlockOpts{
		Encoding:                       solana.EncodingType(req.Opts.Encoding),
		TransactionDetails:             rpc.TransactionDetailsType(req.Opts.TransactionDetails),
		Rewards:                        req.Opts.Rewards,
		Commitment:                     rpc.CommitmentType(req.Opts.Commitment),
		MaxSupportedTransactionVersion: req.Opts.MaxSupportedTransactionVersion,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get block: %w", err)
	}

	return convertBlock(result), nil
}

func (ss *solanaService) GetAccountInfoWithOpts(ctx context.Context, req commonsol.GetAccountInfoRequest) (*commonsol.GetAccountInfoReply, error) {
	reader, err := ss.chain.Reader()
	if err != nil {
		return nil, fmt.Errorf("failed to get reader: %w", err)
	}
	opts := convertAccountInfoOpts(req.Opts)
	account, err := reader.GetAccountInfoWithOpts(ctx, solana.PublicKey(req.Account), opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get account info: %w", err)
	}

	return convertAccount(account), nil
}

// TODO
func (ss *solanaService) GetBalance(ctx context.Context, req commonsol.GetBalanceRequest) (*commonsol.GetBalanceReply, error) {
	reader, err := ss.chain.Reader()
	if err != nil {
		return nil, fmt.Errorf("failed to get reader: %w", err)
	}

	// TODO pass commitment from req
	balance, err := reader.Balance(ctx, solana.PublicKey(req.Addr))
	if err != nil {
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}

	return &commonsol.GetBalanceReply{
		Value: balance,
	}, nil
}

func (ss *solanaService) GetSlotHeight(ctx context.Context, req commonsol.GetSlotHeightRequest) (*commonsol.GetSlotHeightReply, error) {
	return nil, nil
}

func (ss *solanaService) SubmitTransaction(ctx context.Context, req commonsol.SubmitTransactionRequest) (*commonsol.SubmitTransactionReply, error) {
	return nil, nil
}
func (ss *solanaService) RegisterLogTracking(ctx context.Context, req commonsol.LPFilterQuery) error {
	return nil
}
func (ss *solanaService) UnregisterLogTracking(ctx context.Context, filterName string) error {
	return nil
}
func (ss *solanaService) QueryTrackedLogs(ctx context.Context, filterQuery []query.Expression,
	limitAndSort query.LimitAndSort, confidenceLevel primitives.ConfidenceLevel) ([]*commonsol.Log, error) {
	return nil, nil
}
func (ss *solanaService) GetMultipleAccountsWithOpts(ctx context.Context, req commonsol.GetMultipleAccountsRequest) (*commonsol.GetMultipleAccountsReply, error) {
	return nil, nil
}
func (ss *solanaService) GetTransaction(ctx context.Context, req commonsol.GetTransactionRequest) (*commonsol.GetTransactionReply, error) {
	return nil, nil
}
func (ss *solanaService) GetFeeForMessage(ctx context.Context, req commonsol.GetFeeForMessageRequest) (*commonsol.GetFeeForMessageReply, error) {
	return nil, nil
}

func convertAccount(acc *rpc.GetAccountInfoResult) *commonsol.GetAccountInfoReply {
	if acc == nil {
		return nil
	}

	var a *commonsol.Account
	if acc.Value != nil {
		acc.Value.Data.GetBinary()
		a = &commonsol.Account{
			Lamports:   acc.Value.Lamports,
			Executable: acc.Value.Executable,
			Owner:      commonsol.PublicKey(acc.Value.Owner),
		}
	}

	return &commonsol.GetAccountInfoReply{
		RPCContext: commonsol.RPCContext{
			Context: commonsol.Context{
				Slot: acc.Context.Slot,
			},
		},
		Value: a,
	}
}

func convertAccountInfoOpts(opts *commonsol.GetAccountInfoOpts) *rpc.GetAccountInfoOpts {
	var ds *rpc.DataSlice
	if opts.DataSlice != nil {
		ds = &rpc.DataSlice{}
		ds.Length = opts.DataSlice.Length
		ds.Offset = opts.DataSlice.Offset
	}

	return &rpc.GetAccountInfoOpts{
		Encoding:       solana.EncodingType(opts.Encoding),
		Commitment:     rpc.CommitmentType(opts.Commitment),
		MinContextSlot: opts.MinContextSlot,
		DataSlice:      ds,
	}
}

func convertBlock(block *rpc.GetBlockResult) *commonsol.GetBlockReply {
	if block == nil {
		return nil
	}

	// Hashes
	bh := commonsol.Hash(block.Blockhash)
	pbh := commonsol.Hash(block.PreviousBlockhash)

	// Signatures
	var sigs []commonsol.Signature
	if n := len(block.Signatures); n > 0 {
		sigs = make([]commonsol.Signature, n)
		for i, s := range block.Signatures {
			sigs[i] = commonsol.Signature(s)
		}
	}

	var txs []commonsol.TransactionWithMeta
	if n := len(block.Transactions); n > 0 {
		txs = make([]commonsol.TransactionWithMeta, n)
		for i, tx := range block.Transactions {
			var perTxBT *commonsol.UnixTimeSeconds
			if tx.BlockTime != nil {
				t := commonsol.UnixTimeSeconds(int64(*block.BlockTime))
				perTxBT = &t
			}
			var txJSON []byte
			if tx.Transaction != nil {
				if b, err := json.Marshal(tx.Transaction); err == nil {
					txJSON = b
				}
			}

			var metaJSON []byte
			if tx.Meta != nil {
				if b, err := json.Marshal(tx.Meta); err == nil {
					metaJSON = b
				}
			}

			var txBytes []byte
			if tx.Transaction != nil {
				// DataBytesOrJSON.GetBinary() returns the decoded bytes for base encodings;
				// for json/jsonParsed it returns nil.
				txBytes = tx.Transaction.GetBinary()
			}

			txs[i] = commonsol.TransactionWithMeta{
				// Slot is unknown from getBlock response; leave at zero.
				BlockTime:       perTxBT,
				Version:         commonsol.TransactionVersion(tx.Version),
				TransactionJSON: txJSON,
				MetaJSON:        metaJSON,
				TxBytes:         txBytes,
			}
		}
	}

	var bt *commonsol.UnixTimeSeconds
	if block.BlockTime != nil {
		bt = (*commonsol.UnixTimeSeconds)(block.BlockTime)
	}

	return &commonsol.GetBlockReply{
		Blockhash:         bh,
		PreviousBlockhash: pbh,
		ParentSlot:        block.ParentSlot,
		Transactions:      txs,
		Signatures:        sigs,
		BlockTime:         bt,
		BlockHeight:       block.BlockHeight,
	}
}
