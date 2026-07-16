// Package fakes provides in-process implementations of chain capabilities for
// use by cre-cli's `cre workflow simulate`. Solana counterpart to
// chainlink-aptos/capabilities/fakes.
package fakes

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	commonCap "github.com/smartcontractkit/chainlink-common/pkg/capabilities"
	caperrors "github.com/smartcontractkit/chainlink-common/pkg/capabilities/errors"
	solcap "github.com/smartcontractkit/chainlink-common/pkg/capabilities/v2/chain-capabilities/solana"
	solanaserver "github.com/smartcontractkit/chainlink-common/pkg/capabilities/v2/chain-capabilities/solana/server"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	"github.com/smartcontractkit/chainlink-common/pkg/types/core"

	ccipcommon "github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/common"

	commoncodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/common"

	mock_forwarder "github.com/smartcontractkit/chainlink-solana/contracts/generated/mock_forwarder"
)

// Payload boundary constants — must match keystone-forwarder so receivers see
// the same on-chain instruction shape in simulation and prod.
const (
	maxOracles       = 16
	reportContextLen = 96
	signatureLen     = 65
)

const simUnimplementedMsg = "not implemented in cre-cli simulate; raise an issue if your workflow needs this read"

// FakeSolanaChain implements solanaserver.ClientCapability via gagliardetto/solana-go
// and the mock_forwarder Anchor program. Counterpart to FakeAptosChain.
type FakeSolanaChain struct {
	commonCap.CapabilityInfo
	services.Service
	eng *services.Engine

	client                *rpc.Client
	transmitter           solana.PrivateKey
	forwarderProgramID    solana.PublicKey
	forwarderStateAccount solana.PublicKey
	chainSelector         uint64
	dryRunWrites          bool

	// log trigger callback channels and their registered filters
	mu                sync.RWMutex
	callbackCh        map[string]chan commonCap.TriggerAndId[*solcap.Log]
	logTriggerFilters map[string]*solcap.FilterLogTriggerRequest
}

var (
	_ services.Service               = (*FakeSolanaChain)(nil)
	_ solanaserver.ClientCapability  = (*FakeSolanaChain)(nil)
	_ commonCap.ExecutableCapability = (*FakeSolanaChain)(nil)
)

// NewFakeSolanaChain wires a FakeSolanaChain to `client` and the mock forwarder
// at `forwarderProgramID` / `forwarderStateAccount`. `dryRunWrites=true` routes
// WriteReport through SimulateTransaction instead of SendTransaction.
func NewFakeSolanaChain(
	lggr logger.Logger,
	client *rpc.Client,
	transmitter solana.PrivateKey,
	forwarderProgramID solana.PublicKey,
	forwarderStateAccount solana.PublicKey,
	chainSelector uint64,
	dryRunWrites bool,
) (*FakeSolanaChain, error) {
	if client == nil {
		return nil, fmt.Errorf("solana rpc client is required")
	}
	if len(transmitter) == 0 {
		return nil, fmt.Errorf("transmitter private key is required")
	}
	info, err := commonCap.NewCapabilityInfo(
		fmt.Sprintf("solana:ChainSelector:%d@1.0.0", chainSelector),
		commonCap.CapabilityTypeCombined,
		"A fake Solana chain capability",
	)
	if err != nil {
		return nil, fmt.Errorf("new capability info: %w", err)
	}

	fc := &FakeSolanaChain{
		CapabilityInfo:        info,
		client:                client,
		transmitter:           transmitter,
		forwarderProgramID:    forwarderProgramID,
		forwarderStateAccount: forwarderStateAccount,
		chainSelector:         chainSelector,
		dryRunWrites:          dryRunWrites,
		callbackCh:            make(map[string]chan commonCap.TriggerAndId[*solcap.Log]),
		logTriggerFilters:     make(map[string]*solcap.FilterLogTriggerRequest),
	}
	fc.Service, fc.eng = services.Config{
		Name:  fmt.Sprintf("FakeSolanaChain.%d", chainSelector),
		Start: fc.start,
		Close: fc.close,
	}.NewServiceEngine(lggr)
	return fc, nil
}

func (fc *FakeSolanaChain) start(_ context.Context) error {
	fc.eng.Debugw("Solana Chain started")
	return nil
}

func (fc *FakeSolanaChain) close() error {
	fc.eng.Debugw("Solana Chain closed")
	return nil
}

func (fc *FakeSolanaChain) ChainSelector() uint64 { return fc.chainSelector }
func (fc *FakeSolanaChain) Description() string   { return fc.CapabilityInfo.Description }
func (fc *FakeSolanaChain) Name() string          { return fc.ID }
func (fc *FakeSolanaChain) Initialise(ctx context.Context, _ core.StandardCapabilitiesDependencies) error {
	return fc.Start(ctx)
}

func (fc *FakeSolanaChain) RegisterToWorkflow(_ context.Context, request commonCap.RegisterToWorkflowRequest) error {
	fc.eng.Infow("Registered to Solana Chain", "workflowID", request.Metadata.WorkflowID)
	return nil
}

func (fc *FakeSolanaChain) UnregisterFromWorkflow(_ context.Context, request commonCap.UnregisterFromWorkflowRequest) error {
	fc.eng.Infow("Unregistered from Solana Chain", "workflowID", request.Metadata.WorkflowID)
	return nil
}

func (fc *FakeSolanaChain) Execute(_ context.Context, request commonCap.CapabilityRequest) (commonCap.CapabilityResponse, error) {
	fc.eng.Infow("Solana Chain executed", "request", request)
	return commonCap.CapabilityResponse{}, nil
}

// ---------- reads ----------

func (fc *FakeSolanaChain) GetAccountInfoWithOpts(
	ctx context.Context,
	_ commonCap.RequestMetadata,
	input *solcap.GetAccountInfoWithOptsRequest,
) (*commonCap.ResponseAndMetadata[*solcap.GetAccountInfoWithOptsReply], caperrors.Error) {
	if input == nil {
		return nil, caperrors.NewPublicUserError(fmt.Errorf("getAccountInfoWithOptsRequest is nil"), caperrors.InvalidArgument)
	}
	pk, err := pubkeyFromBytes(input.Account)
	if err != nil {
		return nil, caperrors.NewPublicUserError(fmt.Errorf("account: %w", err), caperrors.InvalidArgument)
	}

	out, err := fc.client.GetAccountInfo(ctx, pk)
	if err != nil {
		return nil, caperrors.NewPublicSystemError(fmt.Errorf("solana GetAccountInfo: %w", err), caperrors.Unavailable)
	}

	reply := &solcap.GetAccountInfoWithOptsReply{}
	if out != nil && out.Value != nil {
		reply.Value = accountToProto(out.Value)
	}
	return &commonCap.ResponseAndMetadata[*solcap.GetAccountInfoWithOptsReply]{Response: reply}, nil
}

func (fc *FakeSolanaChain) GetBalance(
	ctx context.Context,
	_ commonCap.RequestMetadata,
	input *solcap.GetBalanceRequest,
) (*commonCap.ResponseAndMetadata[*solcap.GetBalanceReply], caperrors.Error) {
	if input == nil {
		return nil, caperrors.NewPublicUserError(fmt.Errorf("getBalanceRequest is nil"), caperrors.InvalidArgument)
	}
	pk, err := pubkeyFromBytes(input.Addr)
	if err != nil {
		return nil, caperrors.NewPublicUserError(fmt.Errorf("addr: %w", err), caperrors.InvalidArgument)
	}

	out, err := fc.client.GetBalance(ctx, pk, rpc.CommitmentConfirmed)
	if err != nil {
		return nil, caperrors.NewPublicSystemError(fmt.Errorf("solana GetBalance: %w", err), caperrors.Unavailable)
	}
	return &commonCap.ResponseAndMetadata[*solcap.GetBalanceReply]{
		Response: &solcap.GetBalanceReply{Value: out.Value},
	}, nil
}

func (fc *FakeSolanaChain) GetSlotHeight(
	ctx context.Context,
	_ commonCap.RequestMetadata,
	_ *solcap.GetSlotHeightRequest,
) (*commonCap.ResponseAndMetadata[*solcap.GetSlotHeightReply], caperrors.Error) {
	slot, err := fc.client.GetSlot(ctx, rpc.CommitmentConfirmed)
	if err != nil {
		return nil, caperrors.NewPublicSystemError(fmt.Errorf("solana GetSlot: %w", err), caperrors.Unavailable)
	}
	return &commonCap.ResponseAndMetadata[*solcap.GetSlotHeightReply]{
		Response: &solcap.GetSlotHeightReply{Height: slot},
	}, nil
}

// The remaining reads are not implemented in v1 — the canary workflow doesn't
// use them and the proto→solana-go conversion is substantial. Fill in as
// workflows demand them.

func (fc *FakeSolanaChain) GetBlock(_ context.Context, _ commonCap.RequestMetadata, _ *solcap.GetBlockRequest) (*commonCap.ResponseAndMetadata[*solcap.GetBlockReply], caperrors.Error) {
	return nil, unimplemented("GetBlock")
}
func (fc *FakeSolanaChain) GetFeeForMessage(_ context.Context, _ commonCap.RequestMetadata, _ *solcap.GetFeeForMessageRequest) (*commonCap.ResponseAndMetadata[*solcap.GetFeeForMessageReply], caperrors.Error) {
	return nil, unimplemented("GetFeeForMessage")
}
func (fc *FakeSolanaChain) GetMultipleAccountsWithOpts(_ context.Context, _ commonCap.RequestMetadata, _ *solcap.GetMultipleAccountsWithOptsRequest) (*commonCap.ResponseAndMetadata[*solcap.GetMultipleAccountsWithOptsReply], caperrors.Error) {
	return nil, unimplemented("GetMultipleAccountsWithOpts")
}
func (fc *FakeSolanaChain) GetSignatureStatuses(_ context.Context, _ commonCap.RequestMetadata, _ *solcap.GetSignatureStatusesRequest) (*commonCap.ResponseAndMetadata[*solcap.GetSignatureStatusesReply], caperrors.Error) {
	return nil, unimplemented("GetSignatureStatuses")
}
func (fc *FakeSolanaChain) GetTransaction(_ context.Context, _ commonCap.RequestMetadata, _ *solcap.GetTransactionRequest) (*commonCap.ResponseAndMetadata[*solcap.GetTransactionReply], caperrors.Error) {
	return nil, unimplemented("GetTransaction")
}
func (fc *FakeSolanaChain) GetProgramAccounts(_ context.Context, _ commonCap.RequestMetadata, _ *solcap.GetProgramAccountsRequest) (*commonCap.ResponseAndMetadata[*solcap.GetProgramAccountsReply], caperrors.Error) {
	return nil, unimplemented("GetProgramAccounts")
}

// ---------- triggers ----------

func (fc *FakeSolanaChain) RegisterLogTrigger(_ context.Context, triggerID string, _ commonCap.RequestMetadata, input *solcap.FilterLogTriggerRequest) (<-chan commonCap.TriggerAndId[*solcap.Log], caperrors.Error) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	fc.callbackCh[triggerID] = make(chan commonCap.TriggerAndId[*solcap.Log])
	fc.logTriggerFilters[triggerID] = input
	return fc.callbackCh[triggerID], nil
}

func (fc *FakeSolanaChain) UnregisterLogTrigger(_ context.Context, triggerID string, _ commonCap.RequestMetadata, _ *solcap.FilterLogTriggerRequest) caperrors.Error {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	delete(fc.logTriggerFilters, triggerID)
	delete(fc.callbackCh, triggerID)
	return nil
}

func (fc *FakeSolanaChain) AckEvent(_ context.Context, _ string, _ string, _ string) caperrors.Error {
	return nil
}

// ManualTrigger validates a caller-supplied log against the registered filter and
// delivers it to the workflow's trigger callback channel. Used by cre-cli's `simulate` to replay a known
// on-chain event.
func (fc *FakeSolanaChain) ManualTrigger(ctx context.Context, triggerID string, log *solcap.Log) error {
	if log == nil {
		return errors.New("solana log trigger payload is nil")
	}

	fc.eng.Debugf("ManualTrigger: %s", log.String())

	fc.mu.RLock()
	filter := fc.logTriggerFilters[triggerID]
	ch := fc.callbackCh[triggerID]
	fc.mu.RUnlock()

	if ch == nil {
		return fmt.Errorf("solana log trigger %q is not registered", triggerID)
	}
	if filter != nil {
		if err := fakeSolanaLogMatchesFilter(log, filter); err != nil {
			return fmt.Errorf("log does not match registered filter for trigger %s: %w", triggerID, err)
		}
	}

	go func() {
		select {
		case ch <- fc.createManualTriggerEvent(log):
			// Successfully sent trigger response
		case <-ctx.Done():
			// Context cancelled, cleanup goroutine
			fc.eng.Debug("ManualTrigger goroutine cancelled due to context cancellation")
		}
	}()

	return nil
}

// fakeSolanaLogMatchesFilter checks whether log satisfies the
// FilterLogTriggerRequest registered for a trigger. Solana has no EVM-style
// topics: an event is identified by the emitting program address plus an 8-byte
// Anchor discriminator (sha256("event:<EventName>")[:8]).
//
//   - Address: the emitting program's public key. Required — omitting it would
//     match events from every program, a common and dangerous mistake.
//   - EventName: when set, the log's EventSig must equal the Anchor discriminator
//     derived from the event name.
//   - Subkeys: each decodes one top-level scalar event field (via the filter's
//     IDL JSON) and matches if any of its Comparers evaluates true.
func fakeSolanaLogMatchesFilter(log *solcap.Log, filter *solcap.FilterLogTriggerRequest) error {
	if log == nil {
		return errors.New("log is nil")
	}
	if len(filter.GetAddress()) == 0 {
		return errors.New("filter is missing program address: " +
			"omitting it would match events emitted by every program; " +
			"set Address to the emitting program's public key")
	}
	if len(filter.GetAddress()) != solana.PublicKeyLength {
		return fmt.Errorf("filter program address must be %d bytes, got %d", solana.PublicKeyLength, len(filter.GetAddress()))
	}
	if len(log.GetAddress()) != solana.PublicKeyLength {
		return fmt.Errorf("log program address must be %d bytes, got %d", solana.PublicKeyLength, len(log.GetAddress()))
	}
	if len(filter.GetSubkeys()) > 0 {
		if err := subkeyFieldMatches(log, filter); err != nil {
			return fmt.Errorf("subkey filter mismatch: %w", err)
		}
	}
	if cfg := filter.GetCpiFilterConfig(); cfg != nil {
		if len(cfg.GetDestAddress()) != solana.PublicKeyLength {
			return fmt.Errorf("CPI filter destination address must be %d bytes, got %d", solana.PublicKeyLength, len(cfg.GetDestAddress()))
		}
		if len(cfg.GetMethodName()) == 0 {
			return errors.New("CPI filter method name cannot be empty")
		}
	}
	if !bytes.Equal(log.GetAddress(), filter.GetAddress()) {
		return fmt.Errorf("log program address %x does not match filter address %x", log.GetAddress(), filter.GetAddress())
	}
	if name := filter.GetEventName(); name != "" {
		if len(log.GetEventSig()) != 8 {
			return fmt.Errorf("log event signature must be 8 bytes, got %d", len(log.GetEventSig()))
		}
		want := commoncodec.NewDiscriminatorHashPrefix(name, false)
		if !bytes.Equal(log.GetEventSig(), want) {
			return fmt.Errorf("log event signature %x does not match discriminator %x for event %q", log.GetEventSig(), want, name)
		}
	}
	return nil
}

func (fc *FakeSolanaChain) createManualTriggerEvent(log *solcap.Log) commonCap.TriggerAndId[*solcap.Log] {
	return commonCap.TriggerAndId[*solcap.Log]{
		Trigger: log,
		Id:      manualSolanaTriggerEventID(log),
	}
}

func manualSolanaTriggerEventID(log *solcap.Log) string {
	return fmt.Sprintf("manual-solana-chain-trigger-%x-%x-%d", log.GetBlockHash(), log.GetTxHash(), log.GetLogIndex())
}

// ---------- writes ----------

func (fc *FakeSolanaChain) WriteReport(
	ctx context.Context,
	_ commonCap.RequestMetadata,
	input *solcap.WriteReportRequest,
) (*commonCap.ResponseAndMetadata[*solcap.WriteReportReply], caperrors.Error) {
	fc.eng.Infow("Solana Chain WriteReport Started")
	fc.eng.Debugw("Solana Chain WriteReport Input", "input", input)

	if input == nil {
		return nil, caperrors.NewPublicUserError(fmt.Errorf("writeReportRequest is nil"), caperrors.InvalidArgument)
	}
	if input.Report == nil {
		return nil, caperrors.NewPublicUserError(fmt.Errorf("report must not be nil"), caperrors.InvalidArgument)
	}
	if input.ComputeConfig == nil {
		return nil, caperrors.NewPublicUserError(fmt.Errorf("computeConfig must not be nil"), caperrors.InvalidArgument)
	}
	if input.ComputeConfig.ComputeLimit == 0 {
		return nil, caperrors.NewPublicUserError(fmt.Errorf("computeConfig.computeLimit must be > 0"), caperrors.InvalidArgument)
	}
	receiver, err := pubkeyFromBytes(input.Receiver)
	if err != nil {
		return nil, caperrors.NewPublicUserError(fmt.Errorf("receiver: %w", err), caperrors.InvalidArgument)
	}

	payload, err := buildReportPayload(input.Report)
	if err != nil {
		return nil, caperrors.NewPublicUserError(fmt.Errorf("build report payload: %w", err), caperrors.InvalidArgument)
	}

	authority, _, err := deriveForwarderAuthority(fc.forwarderProgramID, fc.forwarderStateAccount, receiver)
	if err != nil {
		return nil, caperrors.NewPublicSystemError(fmt.Errorf("derive forwarder authority: %w", err), caperrors.Internal)
	}

	ix, err := mock_forwarder.NewReportInstruction(
		payload,
		fc.forwarderStateAccount,
		fc.transmitter.PublicKey(),
		authority,
		receiver,
		solana.SystemProgramID,
	)
	if err != nil {
		return nil, caperrors.NewPublicSystemError(fmt.Errorf("build report instruction: %w", err), caperrors.Internal)
	}

	// Append the workflow-supplied remaining accounts to the receiver CPI.
	if generic, ok := ix.(*solana.GenericInstruction); ok {
		for _, acc := range input.RemainingAccounts {
			if acc == nil {
				continue
			}
			pk, perr := pubkeyFromBytes(acc.PublicKey)
			if perr != nil {
				return nil, caperrors.NewPublicUserError(fmt.Errorf("remaining account: %w", perr), caperrors.InvalidArgument)
			}
			generic.AccountValues = append(generic.AccountValues, &solana.AccountMeta{
				PublicKey:  pk,
				IsWritable: acc.IsWritable,
			})
		}
	}

	if fc.dryRunWrites {
		return fc.writeReportDryRun(ctx, ix, receiver)
	}
	return fc.writeReportBroadcast(ctx, ix, receiver)
}

func (fc *FakeSolanaChain) writeReportBroadcast(
	ctx context.Context,
	ix solana.Instruction,
	receiver solana.PublicKey,
) (*commonCap.ResponseAndMetadata[*solcap.WriteReportReply], caperrors.Error) {
	// SendAndConfirm: retries blockhash + send, polls statuses, returns full
	// GetTransactionResult (meta + logs) in one call.
	txres, err := ccipcommon.SendAndConfirm(
		ctx, fc.client,
		[]solana.Instruction{ix},
		fc.transmitter,
		rpc.CommitmentConfirmed,
	)

	reply := &solcap.WriteReportReply{}
	if txres != nil && txres.Transaction != nil {
		// First signature is the payer (transmitter) sig — same as Solana convention.
		tx, decErr := txres.Transaction.GetTransaction()
		if decErr == nil && len(tx.Signatures) > 0 {
			sig := tx.Signatures[0]
			reply.TxSignature = sig[:]
		}
	}
	if txres != nil && txres.Meta != nil {
		fee := txres.Meta.Fee
		reply.TransactionFee = &fee
	}

	if err != nil {
		// SendAndConfirm returns an error both for RPC failures (no meta) and
		// for on-chain reverts (meta present, Meta.Err != nil). Distinguish.
		var logs []string
		if txres != nil && txres.Meta != nil {
			logs = txres.Meta.LogMessages
		}
		s := err.Error()
		reply.ErrorMessage = &s
		reply.TxStatus = solcap.TxStatus_TX_STATUS_FATAL
		reply.ReceiverContractExecutionStatus = receiverStatusFromLogs(logs, fc.forwarderProgramID)
		fc.eng.Infow("Solana Chain WriteReport Failed", "err", err, "logs", logs)
		return &commonCap.ResponseAndMetadata[*solcap.WriteReportReply]{Response: reply}, nil
	}

	recv := solcap.ReceiverContractExecutionStatus_RECEIVER_CONTRACT_EXECUTION_STATUS_SUCCESS
	reply.ReceiverContractExecutionStatus = &recv
	reply.TxStatus = solcap.TxStatus_TX_STATUS_SUCCESS
	fc.eng.Infow("Solana Chain WriteReport Successful", "receiver", receiver.String(), "fee", reply.GetTransactionFee())
	return &commonCap.ResponseAndMetadata[*solcap.WriteReportReply]{Response: reply}, nil
}

func (fc *FakeSolanaChain) writeReportDryRun(
	ctx context.Context,
	ix solana.Instruction,
	receiver solana.PublicKey,
) (*commonCap.ResponseAndMetadata[*solcap.WriteReportReply], caperrors.Error) {
	fc.eng.Infow("Solana Chain WriteReport Dry-Run Enabled")
	out, err := ccipcommon.SimulateTransactionWithOpts(
		ctx, fc.client,
		[]solana.Instruction{ix},
		fc.transmitter,
		rpc.SimulateTransactionOpts{
			SigVerify:              false,
			Commitment:             rpc.CommitmentConfirmed,
			ReplaceRecentBlockhash: true,
		},
	)
	if err != nil {
		return nil, caperrors.NewPublicSystemError(fmt.Errorf("SimulateTransaction: %w", err), caperrors.Unavailable)
	}
	if out == nil || out.Value == nil {
		return nil, caperrors.NewPublicSystemError(fmt.Errorf("SimulateTransaction: empty result"), caperrors.Internal)
	}
	res := out.Value

	reply := &solcap.WriteReportReply{}
	if units := res.UnitsConsumed; units != nil && *units > 0 {
		fee := *units
		reply.TransactionFee = &fee
	}
	if res.Err != nil {
		s := fmt.Sprintf("%v", res.Err)
		reply.ErrorMessage = &s
		reply.TxStatus = solcap.TxStatus_TX_STATUS_FATAL
		reply.ReceiverContractExecutionStatus = receiverStatusFromLogs(res.Logs, fc.forwarderProgramID)
		fc.eng.Infow("Solana Chain WriteReport Dry-Run Reverted", "err", res.Err, "logs", res.Logs)
	} else {
		recv := solcap.ReceiverContractExecutionStatus_RECEIVER_CONTRACT_EXECUTION_STATUS_SUCCESS
		reply.ReceiverContractExecutionStatus = &recv
		reply.TxStatus = solcap.TxStatus_TX_STATUS_SUCCESS
		fc.eng.Infow("Solana Chain WriteReport Dry-Run Successful", "receiver", receiver.String())
	}
	return &commonCap.ResponseAndMetadata[*solcap.WriteReportReply]{Response: reply}, nil
}

// ---------- helpers ----------

func pubkeyFromBytes(b []byte) (solana.PublicKey, error) {
	if len(b) != solana.PublicKeyLength {
		return solana.PublicKey{}, fmt.Errorf("public key must be %d bytes, got %d", solana.PublicKeyLength, len(b))
	}
	var pk solana.PublicKey
	copy(pk[:], b)
	return pk, nil
}

func unimplemented(method string) caperrors.Error {
	return caperrors.NewPublicUserError(
		errors.New(method+": "+simUnimplementedMsg),
		caperrors.Unimplemented,
	)
}

// deriveForwarderAuthority computes the PDA seeds ["forwarder", state, receiver]
// under the mock_forwarder program — same scheme as keystone-forwarder.
func deriveForwarderAuthority(programID, state, receiver solana.PublicKey) (solana.PublicKey, uint8, error) {
	pk, bump, err := solana.FindProgramAddress([][]byte{
		[]byte("forwarder"),
		state.Bytes(),
		receiver.Bytes(),
	}, programID)
	return pk, bump, err
}

// receiverStatusFromLogs distinguishes forwarder-side aborts from receiver-side
// aborts by scanning program logs for "Program <forwarderID> failed". Returns
// nil when we can't tell — caller should leave the field unset.
func receiverStatusFromLogs(logs []string, forwarderID solana.PublicKey) *solcap.ReceiverContractExecutionStatus {
	fwdMarker := "Program " + forwarderID.String() + " failed"
	for _, l := range logs {
		if strings.Contains(l, fwdMarker) {
			return nil // forwarder-level failure; can't blame receiver
		}
	}
	st := solcap.ReceiverContractExecutionStatus_RECEIVER_CONTRACT_EXECUTION_STATUS_REVERTED
	return &st
}
