package solana

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/google/uuid"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
	commonsol "github.com/smartcontractkit/chainlink-common/pkg/types/chains/solana"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query"
	solprimitives "github.com/smartcontractkit/chainlink-common/pkg/types/query/primitives/solana"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/retry"
	logpollertypes "github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/utils"
)

type solanaService struct {
	commontypes.UnimplementedSolanaService
	chain  Chain
	logger logger.Logger
}

func (ss *solanaService) GetBlock(ctx context.Context, req commonsol.GetBlockRequest) (*commonsol.GetBlockReply, error) {
	reader, err := ss.chain.Reader()
	if err != nil {
		return nil, fmt.Errorf("failed to get reader: %w", err)
	}

	result, err := reader.GetBlockWithOpts(ctx, req.Slot, &rpc.GetBlockOpts{
		Commitment: rpc.CommitmentType(req.Opts.Commitment),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get block: %w", err)
	}

	return convertBlock(result), nil
}

func (ss *solanaService) GetLatestLPBlock(ctx context.Context) (*commonsol.LPBlock, error) {
	lp := ss.chain.LogPoller()
	n, err := lp.GetLatestBlock(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest lp block: %w", err)
	}

	return &commonsol.LPBlock{
		Slot: uint64(n), //nolint:gosec // G115
	}, nil
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

	return convertAccountResult(account, req.Opts.Encoding)
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
		Addresses: make([]solana.PublicKey, 0, len(req.Opts.Accounts.Addresses)),
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
		return nil, fmt.Errorf("simulate tx failed: %w", err)
	}
	var simErr string
	if res.Err != nil {
		simErr = fmt.Sprintf("%v", res.Err)
	}

	accs, err := convertAccounts(res.Accounts)
	if err != nil {
		return nil, err
	}
	return &commonsol.SimulateTXReply{
		Err:           simErr,
		Logs:          res.Logs,
		Accounts:      accs,
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
		return fmt.Errorf("failed to register filter: %w", err)
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
	queryName, err := deriveNameFromFilterQuery(filterQuery)
	if err != nil {
		return nil, err
	}

	logs, err := lp.FilteredLogs(ctx, filterQuery, limitAndSort, queryName)
	if err != nil {
		return nil, fmt.Errorf("failed to filter logs: %w", err)
	}

	res := make([]*commonsol.Log, 0, len(logs))
	for _, l := range logs {
		res = append(res, &commonsol.Log{
			ChainID:        l.ChainID,
			LogIndex:       l.LogIndex,
			BlockHash:      commonsol.Hash(l.BlockHash),
			BlockNumber:    l.BlockNumber,
			BlockTimestamp: uint64(l.BlockTimestamp.Unix()), //nolint:gosec // G115
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

func (ss *solanaService) GetFiltersNames(ctx context.Context) ([]string, error) {
	filters, err := ss.chain.LogPoller().GetFilters(ctx)
	if err != nil {
		return nil, err
	}
	filterNames := make([]string, 0, len(filters))
	for name := range filters {
		filterNames = append(filterNames, name)
	}
	return filterNames, nil
}

var (
	errMissingEventSigPrimitive = errors.New("missing event signature primitive in filter query")
	errMissingAddressPrimitive  = errors.New("missing address primitive in filter query")
)

func deriveNameFromFilterQuery(filter []query.Expression) (string, error) {
	var address string
	var eventSig string

	for _, expr := range filter {
		if expr.IsPrimitive() {
			switch primitive := expr.Primitive.(type) {
			case *solprimitives.Address:
				address = solana.PublicKey(primitive.PubKey).String()
			case *solprimitives.EventSig:
				eventSig = fmt.Sprintf("%x", primitive.Sig)
			}
		}
	}

	var errs []error
	if address == "" {
		errs = append(errs, errMissingAddressPrimitive)
	}
	if eventSig == "" {
		errs = append(errs, errMissingEventSigPrimitive)
	}
	if len(errs) > 0 {
		return "", errors.Join(errs...)
	}

	return address + "-" + eventSig, nil
}

func (ss *solanaService) GetSignatureStatuses(ctx context.Context, req commonsol.GetSignatureStatusesRequest) (*commonsol.GetSignatureStatusesReply, error) {
	sigs := make([]solana.Signature, 0, len(req.Sigs))
	for _, s := range req.Sigs {
		sigs = append(sigs, solana.Signature(s))
	}

	res, err := ss.chain.MultiClient().SignatureStatuses(ctx, sigs)
	if err != nil {
		return nil, fmt.Errorf("failed to get signature statuses: %w", err)
	}

	statuses := make([]commonsol.GetSignatureStatusesResult, 0, len(res))
	for _, r := range res {
		var stErr string
		if r.Err != nil {
			stErr = fmt.Sprintf("%v", r.Err)
		}
		statuses = append(statuses, commonsol.GetSignatureStatusesResult{
			Slot:               r.Slot,
			Err:                stErr,
			Confirmations:      r.Confirmations,
			ConfirmationStatus: commonsol.ConfirmationStatusType(r.ConfirmationStatus),
		})
	}
	return &commonsol.GetSignatureStatusesReply{
		Results: statuses,
	}, nil
}

func (ss *solanaService) GetSlotHeight(ctx context.Context, req commonsol.GetSlotHeightRequest) (*commonsol.GetSlotHeightReply, error) {
	reader, err := ss.chain.Reader()
	if err != nil {
		return nil, fmt.Errorf("failed to get reader: %w", err)
	}

	slot, err := reader.SlotHeightWithCommitment(ctx, rpc.CommitmentType(req.Commitment))
	if err != nil {
		return nil, fmt.Errorf("failed to get slot height: %w", err)
	}

	return &commonsol.GetSlotHeightReply{Height: slot}, nil
}

func (ss *solanaService) SubmitTransaction(ctx context.Context, req commonsol.SubmitTransactionRequest) (*commonsol.SubmitTransactionReply, error) {
	txID, err := uuid.NewUUID() // NOTE: TXM expects us to generate an ID, rather than return one
	if err != nil {
		return nil, err
	}
	tx, err := solana.TransactionFromBase64(req.EncodedTransaction)
	if err != nil {
		return nil, fmt.Errorf("invalid transaction payload: %w", err)
	}
	// remove dummy signatures (were injected by tx.MarshalBinary)
	tx.Signatures = tx.Signatures[:0]
	forwarder := solana.PublicKey(req.Receiver)
	r, err := ss.chain.Reader()
	if err != nil {
		return nil, fmt.Errorf("failed to get reader: %w", err)
	}

	blockhash, err := r.LatestBlockhash(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest blockhash: %w", err)
	}
	transactionID := txID.String()
	var cfg []utils.SetTxConfig
	if req.Cfg != nil {
		cfg = append(cfg, utils.SetEstimateComputeUnitLimit(false))
		if req.Cfg.ComputeLimit != nil {
			cfg = append(cfg, utils.SetComputeUnitLimit(*req.Cfg.ComputeLimit))
		}
	}

	tx.Message.RecentBlockhash = blockhash.Value.Blockhash
	err = ss.chain.TxManager().Enqueue(ctx, forwarder.String(), tx, &transactionID, blockhash.Value.LastValidBlockHeight, cfg...)
	if err != nil {
		return nil, fmt.Errorf("failed to enqueue transaction: %w", err)
	}

	maximumWaitTimeForConfirmation := ss.chain.Config().WF().AcceptanceTimeout()
	retryContext, cancel := context.WithTimeout(ctx, maximumWaitTimeForConfirmation)
	defer cancel()

	txStatus, err := retry.Do(retryContext, ss.logger, func(ctx context.Context) (commonsol.TransactionStatus, error) {
		txStatus, txStatusErr := ss.chain.TxManager().GetTransactionStatus(ctx, transactionID)
		if txStatusErr != nil {
			return commonsol.TxFatal, txStatusErr
		}

		switch txStatus {
		case commontypes.Fatal, commontypes.Failed:
			return commonsol.TxFatal, nil
		case commontypes.Unconfirmed, commontypes.Finalized:
			return commonsol.TxSuccess, nil
		case commontypes.Pending, commontypes.Unknown:
			return commonsol.TxFatal, fmt.Errorf("tx still in state pending or unknown, tx status is %d for tx with ID %s", txStatus, txID)
		default:
			return commonsol.TxFatal, fmt.Errorf("unexpected transaction status %d for tx with ID %s", txStatus, txID)
		}
	})

	if err != nil {
		return nil, fmt.Errorf("failed getting transaction status. %w", err)
	}

	if txStatus == commonsol.TxFatal {
		return &commonsol.SubmitTransactionReply{Status: txStatus, IdempotencyKey: transactionID}, nil
	}

	return &commonsol.SubmitTransactionReply{Status: txStatus, IdempotencyKey: transactionID}, nil
}

func (ss *solanaService) GetMultipleAccountsWithOpts(ctx context.Context, req commonsol.GetMultipleAccountsRequest) (*commonsol.GetMultipleAccountsReply, error) {
	r, err := ss.chain.Reader()
	if err != nil {
		return nil, fmt.Errorf("failed to get reader: %w", err)
	}
	var opts *rpc.GetMultipleAccountsOpts
	var enc commonsol.EncodingType
	if req.Opts != nil {
		enc = req.Opts.Encoding
		var ds *rpc.DataSlice
		if req.Opts.DataSlice != nil {
			ds = &rpc.DataSlice{
				Offset: req.Opts.DataSlice.Offset,
				Length: req.Opts.DataSlice.Length,
			}
		}
		opts = &rpc.GetMultipleAccountsOpts{
			Encoding:       solana.EncodingType(req.Opts.Encoding),
			Commitment:     rpc.CommitmentType(req.Opts.Commitment),
			DataSlice:      ds,
			MinContextSlot: req.Opts.MinContextSlot,
		}
	}
	res, err := r.GetMultipleAccountsWithOpts(ctx, convertPubKeys(req.Accounts), opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get multiple accounts with opts: %w", err)
	}

	accounts := make([]*commonsol.Account, 0, len(res.Value))
	for _, acc := range res.Value {
		data, err := convertDataBytesOrJSON(acc.Data, enc)
		if err != nil {
			return nil, fmt.Errorf("conversion data bytes or json failed: %w", err)
		}
		accounts = append(accounts, &commonsol.Account{
			Lamports:   acc.Lamports,
			Owner:      commonsol.PublicKey(acc.Owner),
			Data:       data,
			Executable: acc.Executable,
			RentEpoch:  acc.RentEpoch,
			Space:      acc.Space,
		})
	}

	return &commonsol.GetMultipleAccountsReply{
		RPCContext: commonsol.RPCContext{
			Slot: res.Context.Slot,
		},
		Value: accounts,
	}, nil
}

func (ss *solanaService) GetTransaction(ctx context.Context, req commonsol.GetTransactionRequest) (*commonsol.GetTransactionReply, error) {
	r, err := ss.chain.Reader()
	if err != nil {
		return nil, fmt.Errorf("failed to get reader: %w", err)
	}

	tx, err := r.GetTransaction(ctx, solana.Signature(req.Signature))
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}

	var bt *commonsol.UnixTimeSeconds
	if tx.BlockTime != nil {
		bt = (*commonsol.UnixTimeSeconds)(tx.BlockTime)
	}
	ptx, err := convertTransactionEnvelope(tx)
	if err != nil {
		return nil, err
	}

	return &commonsol.GetTransactionReply{
		Version:     commonsol.TransactionVersion(tx.Version),
		Slot:        tx.Slot,
		BlockTime:   bt,
		Transaction: ptx,
		Meta:        convertTransactionMeta(tx.Meta),
	}, nil
}

func (ss *solanaService) GetFeeForMessage(ctx context.Context, req commonsol.GetFeeForMessageRequest) (*commonsol.GetFeeForMessageReply, error) {
	fee, err := ss.chain.MultiClient().GetFeeForMessage(ctx, req.Message)
	if err != nil {
		return nil, fmt.Errorf("failed to get fee for message: %w", err)
	}

	return &commonsol.GetFeeForMessageReply{
		Fee: fee,
	}, nil
}

// converters
func convertTransactionEnvelope(tx *rpc.GetTransactionResult) (*commonsol.TransactionResultEnvelope, error) {
	if tx == nil || tx.Transaction == nil {
		return nil, nil
	}
	out := &commonsol.TransactionResultEnvelope{}
	data := tx.Transaction.GetData()
	out.AsDecodedBinary = commonsol.Data{
		Content:  data.Content,
		Encoding: commonsol.EncodingType(data.Encoding),
	}

	ptx, err := tx.Transaction.GetTransaction()
	if err != nil {
		return nil, fmt.Errorf("failed to get parse tx envelope: %w", err)
	}
	out.AsParsedTransaction = convertTransaction(ptx)

	return out, nil
}

func convertTransaction(tx *solana.Transaction) *commonsol.Transaction {
	if tx == nil {
		return nil
	}
	out := &commonsol.Transaction{}
	for _, s := range tx.Signatures {
		out.Signatures = append(out.Signatures, commonsol.Signature(s))
	}

	out.Message = convertMessage(tx.Message)
	return out
}

func convertMessage(m solana.Message) commonsol.Message {
	out := commonsol.Message{
		AccountKeys:         make(commonsol.PublicKeySlice, len(m.AccountKeys)),
		Header:              convertMessageHeader(m.Header),
		RecentBlockhash:     commonsol.Hash(m.RecentBlockhash),
		Instructions:        make([]commonsol.CompiledInstruction, len(m.Instructions)),
		AddressTableLookups: convertAddressTableLookupSlice(m.AddressTableLookups),
	}

	for i, pk := range m.AccountKeys {
		out.AccountKeys[i] = commonsol.PublicKey(pk)
	}

	for i, ix := range m.Instructions {
		out.Instructions[i] = convertCompiledInstruction(ix)
	}

	return out
}

func convertAddressTableLookupSlice(in []solana.MessageAddressTableLookup) commonsol.MessageAddressTableLookupSlice {
	if len(in) == 0 {
		return nil
	}
	out := make(commonsol.MessageAddressTableLookupSlice, len(in))
	for i, atl := range in {
		out[i] = commonsol.MessageAddressTableLookup{
			AccountKey:      commonsol.PublicKey(atl.AccountKey),
			WritableIndexes: atl.WritableIndexes,
			ReadonlyIndexes: atl.ReadonlyIndexes,
		}
	}
	return out
}

func convertCompiledInstruction(ix solana.CompiledInstruction) commonsol.CompiledInstruction {
	out := commonsol.CompiledInstruction{
		ProgramIDIndex: ix.ProgramIDIndex,
		Data:           ix.Data,
		Accounts:       ix.Accounts,
	}
	return out
}

func convertMessageHeader(h solana.MessageHeader) commonsol.MessageHeader {
	// Same field semantics/types (uint8), copy directly.
	return commonsol.MessageHeader{
		NumRequiredSignatures:       h.NumRequiredSignatures,
		NumReadonlySignedAccounts:   h.NumReadonlySignedAccounts,
		NumReadonlyUnsignedAccounts: h.NumReadonlyUnsignedAccounts,
	}
}
func convertTransactionMeta(meta *rpc.TransactionMeta) *commonsol.TransactionMeta {
	if meta == nil {
		return nil
	}
	var metaErr string
	if meta.Err != nil {
		metaErr = fmt.Sprintf("%v", meta.Err)
	}
	out := &commonsol.TransactionMeta{
		Err:                  metaErr,
		Fee:                  meta.Fee,
		PreBalances:          meta.PreBalances,
		PostBalances:         meta.PostBalances,
		LogMessages:          meta.LogMessages,
		ComputeUnitsConsumed: nil,
		InnerInstructions:    nil,
		PreTokenBalances:     nil,
		PostTokenBalances:    nil,
		LoadedAddresses:      commonsol.LoadedAddresses{},
		ReturnData:           commonsol.ReturnData{},
	}

	if len(meta.InnerInstructions) > 0 {
		out.InnerInstructions = make([]commonsol.InnerInstruction, 0, len(meta.InnerInstructions))
		for _, in := range meta.InnerInstructions {
			out.InnerInstructions = append(out.InnerInstructions, convertInnerInstruction(in))
		}
	}

	if len(meta.PreTokenBalances) > 0 {
		out.PreTokenBalances = make([]commonsol.TokenBalance, 0, len(meta.PreTokenBalances))
		for _, tb := range meta.PreTokenBalances {
			out.PreTokenBalances = append(out.PreTokenBalances, convertTokenBalance(tb))
		}
	}
	if len(meta.PostTokenBalances) > 0 {
		out.PostTokenBalances = make([]commonsol.TokenBalance, 0, len(meta.PostTokenBalances))
		for _, tb := range meta.PostTokenBalances {
			out.PostTokenBalances = append(out.PostTokenBalances, convertTokenBalance(tb))
		}
	}

	out.LoadedAddresses = commonsol.LoadedAddresses{
		ReadOnly: convertSolPubKeysToCommon(meta.LoadedAddresses.ReadOnly),
		Writable: convertSolPubKeysToCommon(meta.LoadedAddresses.Writable),
	}

	rd := meta.ReturnData
	out.ReturnData = commonsol.ReturnData{
		ProgramId: commonsol.PublicKey(rd.ProgramId),
		Data: commonsol.Data{
			Content:  rd.Data.Content,
			Encoding: commonsol.EncodingType(rd.Data.Encoding),
		},
	}

	if meta.ComputeUnitsConsumed != nil {
		v := *meta.ComputeUnitsConsumed
		out.ComputeUnitsConsumed = &v
	}

	return out
}

func convertInnerInstruction(in rpc.InnerInstruction) commonsol.InnerInstruction {
	out := commonsol.InnerInstruction{
		Index:        in.Index,
		Instructions: make([]commonsol.CompiledInstruction, 0, len(in.Instructions)),
	}
	for _, ci := range in.Instructions {
		out.Instructions = append(out.Instructions, commonsol.CompiledInstruction{
			ProgramIDIndex: ci.ProgramIDIndex,
			Accounts:       ci.Accounts,
			Data:           ci.Data,
			StackHeight:    ci.StackHeight,
		})
	}
	return out
}

func convertTokenBalance(tb rpc.TokenBalance) commonsol.TokenBalance {
	var owner *commonsol.PublicKey
	if tb.Owner != nil {
		pk := commonsol.PublicKey(*tb.Owner)
		owner = &pk
	}
	var programID *commonsol.PublicKey
	if tb.ProgramId != nil {
		pk := commonsol.PublicKey(*tb.ProgramId)
		programID = &pk
	}

	var ui *commonsol.UiTokenAmount
	if tb.UiTokenAmount != nil {
		ui = &commonsol.UiTokenAmount{
			Amount:         tb.UiTokenAmount.Amount,
			Decimals:       tb.UiTokenAmount.Decimals,
			UiAmountString: tb.UiTokenAmount.UiAmountString,
		}
	}

	return commonsol.TokenBalance{
		AccountIndex:  tb.AccountIndex,
		Owner:         owner,
		ProgramId:     programID,
		Mint:          commonsol.PublicKey(tb.Mint),
		UiTokenAmount: ui,
	}
}

func convertPubKeys(keys []commonsol.PublicKey) []solana.PublicKey {
	ret := make([]solana.PublicKey, 0, len(keys))
	for _, acc := range keys {
		ret = append(ret, solana.PublicKey(acc))
	}
	return ret
}

func convertSolPubKeysToCommon(keys []solana.PublicKey) []commonsol.PublicKey {
	ret := make([]commonsol.PublicKey, 0, len(keys))
	for _, acc := range keys {
		ret = append(ret, commonsol.PublicKey(acc))
	}
	return ret
}

func convertFilter(f commonsol.LPFilterQuery) (logpollertypes.Filter, error) {
	var idl logpollertypes.EventIdl
	err := json.Unmarshal(f.ContractIdlJSON, &idl)
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

func convertAccounts(accs []*rpc.Account) ([]*commonsol.Account, error) {
	ret := make([]*commonsol.Account, 0, len(accs))
	for _, acc := range accs {
		data, err := convertDataBytesOrJSON(acc.Data, "")
		if err != nil {
			return nil, fmt.Errorf("conversion data bytes or json failed: %w", err)
		}
		ret = append(ret, &commonsol.Account{
			Lamports:   acc.Lamports,
			Owner:      commonsol.PublicKey(acc.Owner),
			Data:       data,
			Executable: acc.Executable,
			RentEpoch:  acc.RentEpoch,
			Space:      acc.Space,
		})
	}

	return ret, nil
}

func convertAccountResult(acc *rpc.GetAccountInfoResult, enc commonsol.EncodingType) (*commonsol.GetAccountInfoReply, error) {
	if acc == nil {
		return nil, nil
	}

	var a *commonsol.Account
	data, err := convertDataBytesOrJSON(acc.Value.Data, enc)
	if err != nil {
		return nil, err
	}
	if acc.Value != nil {
		a = &commonsol.Account{
			Lamports:   acc.Value.Lamports,
			Executable: acc.Value.Executable,
			Owner:      commonsol.PublicKey(acc.Value.Owner),
			Data:       data,
		}
	}

	return &commonsol.GetAccountInfoReply{
		RPCContext: commonsol.RPCContext{
			Slot: acc.Context.Slot,
		},
		Value: a,
	}, nil
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

func convertDataBytesOrJSON(obj *rpc.DataBytesOrJSON, pref commonsol.EncodingType) (*commonsol.DataBytesOrJSON, error) {
	if obj == nil {
		return nil, nil
	}
	if pref == "" {
		pref = commonsol.EncodingBase64
	}

	txBytes := obj.GetBinary()

	txJSON, jsonErr := json.Marshal(obj)
	if jsonErr != nil && len(txBytes) == 0 {
		return nil, fmt.Errorf("failed to marshal tx data: %w", jsonErr)
	}

	switch pref {
	case commonsol.EncodingBase64:
		if len(txBytes) != 0 {
			return &commonsol.DataBytesOrJSON{
				RawDataEncoding: commonsol.EncodingBase64,
				AsDecodedBinary: txBytes,
				AsJSON:          txJSON,
			}, nil
		}

		// Fallback: decode ["<base64>", "base64"] manually
		var arr []string
		if err := json.Unmarshal(txJSON, &arr); err != nil {
			return nil, fmt.Errorf("expected base64 bytes but GetBinary() empty; also failed to parse json: %w json=%s", err, string(txJSON))
		}
		if len(arr) != 2 {
			return nil, fmt.Errorf("expected [data,encoding] json array, got len=%d json=%s", len(arr), string(txJSON))
		}

		s := arr[0]
		enc := arr[1]
		if enc != "base64" {
			return nil, fmt.Errorf("expected encoding base64, got %q json=%s", enc, string(txJSON))
		}

		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("base64 decode failed: %w", err)
		}

		return &commonsol.DataBytesOrJSON{
			RawDataEncoding: commonsol.EncodingBase64,
			AsDecodedBinary: b,
			AsJSON:          txJSON,
		}, nil

	case commonsol.EncodingJSON, commonsol.EncodingJSONParsed:
		// Caller explicitly wants JSON. Return it even if bytes exist.
		return &commonsol.DataBytesOrJSON{
			RawDataEncoding: pref,
			AsDecodedBinary: txBytes,
			AsJSON:          txJSON,
		}, nil

	default:
		// Treat unknown as base64 preference
		if len(txBytes) == 0 {
			return nil, fmt.Errorf("expected binary account data but got empty bytes: %s", string(txJSON))
		}
		return &commonsol.DataBytesOrJSON{
			RawDataEncoding: commonsol.EncodingBase64,
			AsDecodedBinary: txBytes,
			AsJSON:          txJSON,
		}, nil
	}
}

func convertBlock(block *rpc.GetBlockResult) *commonsol.GetBlockReply {
	if block == nil {
		return nil
	}

	// Hashes
	bh := commonsol.Hash(block.Blockhash)
	pbh := commonsol.Hash(block.PreviousBlockhash)

	var bt *commonsol.UnixTimeSeconds
	if block.BlockTime != nil {
		bt = (*commonsol.UnixTimeSeconds)(block.BlockTime)
	}

	return &commonsol.GetBlockReply{
		Blockhash:         bh,
		PreviousBlockhash: pbh,
		ParentSlot:        block.ParentSlot,
		BlockTime:         bt,
		BlockHeight:       block.BlockHeight,
	}
}
