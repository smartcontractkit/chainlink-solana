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
	logpollertypes "github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
)

type solanaService struct {
	chain  Chain
	logger logger.Logger
}

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

	return convertBlock(result, req.Opts.Encoding), nil
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

	return convertAccountResult(account), nil
}

func (ss *solanaService) GetBalance(ctx context.Context, req commonsol.GetBalanceRequest) (*commonsol.GetBalanceReply, error) {
	reader, err := ss.chain.Reader()
	if err != nil {
		return nil, fmt.Errorf("failed to get reader: %w", err)
	}

	balance, err := reader.BalanceWithCommitment(ctx, solana.PublicKey(req.Addr), rpc.CommitmentType(req.Commitment))
	if err != nil {
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}

	return &commonsol.GetBalanceReply{
		Value: balance,
	}, nil
}

func (ss *solanaService) SimulateTX(ctx context.Context, req commonsol.SimulateTXRequest) (*commonsol.SimulateTXReply, error) {
	tx, err := solana.TransactionFromBase64(req.EncodedTransaction)
	if err != nil {
		return nil, fmt.Errorf("failed to decode transaction: %w", err)
	}
	accounts := &rpc.SimulateTransactionAccountsOpts{
		Encoding:  solana.EncodingType(req.Opts.Accounts.Encoding),
		Addresses: make([]solana.PublicKey, len(req.Opts.Accounts.Addresses)),
	}
	for _, addr := range req.Opts.Accounts.Addresses {
		accounts.Addresses = append(accounts.Addresses, solana.PublicKey(addr))
	}

	res, err := ss.chain.MultiClient().SimulateTx(ctx, tx, &rpc.SimulateTransactionOpts{
		SigVerify:              req.Opts.SigVerify,
		Commitment:             rpc.CommitmentType(req.Opts.Commitment),
		ReplaceRecentBlockhash: req.Opts.ReplaceRecentBlockhash,
		Accounts:               accounts,
	})
	if err != nil {
		return nil, fmt.Errorf("simulate tx failed")
	}
	var simErr string
	if res.Err != nil {
		simErr = fmt.Sprintf("%v", res.Err)
	}
	return &commonsol.SimulateTXReply{
		Err:           simErr,
		Logs:          res.Logs,
		Accounts:      convertAccounts(res.Accounts),
		UnitsConsumed: res.UnitsConsumed,
	}, nil
}

func (ss *solanaService) RegisterLogTracking(ctx context.Context, req commonsol.LPFilterQuery) error {
	lp := ss.chain.LogPoller()
	if lp.HasFilter(ctx, req.Name) {
		return nil
	}

	f, err := convertFilter(req)
	if err != nil {
		return err
	}

	err = lp.RegisterFilter(ctx, f)
	if err != nil {
		return fmt.Errorf("failed to register fitler: %w", err)
	}

	return nil
}

func (ss *solanaService) UnregisterLogTracking(ctx context.Context, filterName string) error {
	lp := ss.chain.LogPoller()
	if !lp.HasFilter(ctx, filterName) {
		return nil
	}

	return lp.UnregisterFilter(ctx, filterName)
}

func (ss *solanaService) QueryTrackedLogs(ctx context.Context, filterQuery []query.Expression,
	limitAndSort query.LimitAndSort) ([]*commonsol.Log, error) {
	lp := ss.chain.LogPoller()
	// TODO derive query name from filterQuery
	logs, err := lp.FilteredLogs(ctx, filterQuery, limitAndSort, "")
	if err != nil {
		return nil, fmt.Errorf("failed to filter logs: %w", err)
	}

	res := make([]*commonsol.Log, len(logs))
	for _, l := range logs {
		res = append(res, &commonsol.Log{

			ChainID:        l.ChainID,
			LogIndex:       l.LogIndex,
			BlockHash:      commonsol.Hash(l.BlockHash),
			BlockNumber:    l.BlockNumber,
			BlockTimestamp: l.BlockTimestamp,
			Address:        commonsol.PublicKey(l.Address),
			EventSig:       commonsol.EventSignature(l.EventSig),
			TxHash:         commonsol.Signature(l.TxHash),
			Data:           l.Data,
			SequenceNum:    l.SequenceNum,
			Error:          l.Error,
		})
	}

	return res, nil
}

// TODO
func (ss *solanaService) GetSignatureStatuses(ctx context.Context, req commonsol.GetSignatureStatusesRequest) (*commonsol.GetSignatureStatusesReply, error) {
	return nil, nil
}

func (ss *solanaService) GetLatestBlockhash(ctx context.Context, req commonsol.GetLatestBlockhashRequest) (*commonsol.GetLatestBlockhashReply, error) {
	return nil, nil
}

func (ss *solanaService) GetSlotHeight(ctx context.Context, req commonsol.GetSlotHeightRequest) (*commonsol.GetSlotHeightReply, error) {
	return nil, nil
}

func (ss *solanaService) SubmitTransaction(ctx context.Context, req commonsol.SubmitTransactionRequest) (*commonsol.SubmitTransactionReply, error) {
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

// converters
func convertFilter(f commonsol.LPFilterQuery) (logpollertypes.Filter, error) {
	var idl logpollertypes.EventIdl
	err := json.Unmarshal(f.EventIdlJSON, &idl)
	if err != nil {
		return logpollertypes.Filter{}, fmt.Errorf("invalid event idl: %w", err)
	}

	return logpollertypes.Filter{
		Name:            f.Name,
		Address:         logpollertypes.PublicKey(f.Address),
		EventName:       f.EventName,
		EventSig:        logpollertypes.EventSignature(f.EventSig),
		StartingBlock:   f.StartingBlock,
		EventIdl:        idl,
		SubkeyPaths:     logpollertypes.SubKeyPaths(f.SubkeyPaths),
		Retention:       f.Retention,
		MaxLogsKept:     f.MaxLogsKept,
		IncludeReverted: f.IncludeReverted,
	}, nil
}

func convertAccounts(accs []*rpc.Account) []*commonsol.Account {
	ret := make([]*commonsol.Account, len(accs))
	for _, acc := range accs {
		ret = append(ret, &commonsol.Account{
			Lamports:   acc.Lamports,
			Owner:      commonsol.PublicKey(acc.Owner),
			Data:       convertDataBytesOrJSON(acc.Data, ""),
			Executable: acc.Executable,
			RentEpoch:  acc.RentEpoch,
			Space:      acc.Space,
		})
	}

	return ret
}

func convertAccountResult(acc *rpc.GetAccountInfoResult) *commonsol.GetAccountInfoReply {
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

func convertDataBytesOrJSON(obj *rpc.DataBytesOrJSON, enc commonsol.EncodingType) *commonsol.DataBytesOrJSON {
	if obj == nil {
		return nil
	}
	if enc == "" {
		enc = commonsol.EncodingJSON // default
	}
	var txJSON []byte
	if b, err := json.Marshal(obj); err == nil {
		txJSON = b
	}
	txBytes := obj.GetBinary()

	return &commonsol.DataBytesOrJSON{
		RawDataEncoding: enc,
		AsDecodedBinary: txBytes,
		AsJSON:          txJSON,
	}
}

func convertBlock(block *rpc.GetBlockResult, enc commonsol.EncodingType) *commonsol.GetBlockReply {
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

			var metaJSON []byte
			if tx.Meta != nil {
				if b, err := json.Marshal(tx.Meta); err == nil {
					metaJSON = b
				}
			}

			txs[i] = commonsol.TransactionWithMeta{
				BlockTime:   perTxBT,
				Version:     commonsol.TransactionVersion(tx.Version),
				Transaction: convertDataBytesOrJSON(tx.Transaction, enc),
				MetaJSON:    metaJSON,
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
