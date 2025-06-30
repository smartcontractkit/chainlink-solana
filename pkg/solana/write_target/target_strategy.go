package writetarget

import (
	"context"
	"crypto/sha3"
	"errors"
	"fmt"
	"maps"

	"github.com/gagliardetto/solana-go"
	"github.com/google/uuid"
	"github.com/smartcontractkit/chainlink-common/pkg/capabilities"
	ocr3types "github.com/smartcontractkit/chainlink-common/pkg/capabilities/consensus/ocr3/types"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
	wt "github.com/smartcontractkit/chainlink-framework/capabilities/writetarget"
	ks_forwarder "github.com/smartcontractkit/chainlink-solana/contracts/generated/keystone_forwarder"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainwriter"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/txm"

	binary "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go/rpc"
)

var (
	_ wt.TargetStrategy = &targetStrategy{}
)

type targetStrategy struct {
	client client.Reader
	txm    txm.TxManager

	forwarder   string
	accounts    accounts
	lookupTable map[solana.PublicKey]solana.PublicKeySlice
	lggr        logger.Logger
}

type accounts struct {
	state              solana.PublicKey
	forwarderID        solana.PublicKey
	oraclesConfig      solana.PublicKey
	transmitter        solana.PublicKey
	forwarderAuthority solana.PublicKey
	remainings         solana.AccountMetaSlice
	lookupTable        solana.PublicKey
}

func newTargetStrategy(client client.Reader, txm txm.TxManager, cfg config.Workflow, lggr logger.Logger) wt.TargetStrategy {
	lookup := make(map[solana.PublicKey]solana.PublicKeySlice)

	var accs accounts
	//TODO extract accs from config
	lookup[accs.lookupTable] = solana.PublicKeySlice{
		accs.oraclesConfig,
		accs.state,
		accs.transmitter,
		solana.SystemProgramID,
	}

	return &targetStrategy{
		client:      client,
		txm:         txm,
		accounts:    accs,
		forwarder:   cfg.ForwarderAddress(),
		lggr:        lggr,
		lookupTable: lookup,
	}
}

func (ts *targetStrategy) QueryTransmissionState(ctx context.Context, reportID uint16, request capabilities.CapabilityRequest) (*wt.TransmissionState, error) {
	r, err := getRequest(request)
	if err != nil {
		return nil, err
	}

	executionState, err := ts.deriveExecutionState(r)
	if err != nil {
		return nil, fmt.Errorf("failed to derive execution state: %w", err)
	}

	acc, err := ts.client.GetAccountInfoWithOpts(ctx, executionState, &rpc.GetAccountInfoOpts{Commitment: rpc.CommitmentProcessed})
	if err != nil {
		return nil, fmt.Errorf("failed to get transmission state latest value: %w", err)
	}

	if acc.Value == nil {
		ts.lggr.Infow("non-empty report - transmission not attempted", "request", request,
			"reportLen", len(r.Inputs.SignedReport.Report),
			"reportContextLen", len(r.Inputs.SignedReport.Context),
			"nSignatures", len(r.Inputs.SignedReport.Signatures),
			"executionID", request.Metadata.WorkflowExecutionID)

		return &wt.TransmissionState{
			Status:      wt.TransmissionStateNotAttempted,
			Transmitter: "",
			Err:         nil,
		}, nil
	}

	var transmissionInfo ks_forwarder.ExecutionState

	err = binary.UnmarshalBorsh(&transmissionInfo, acc.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed to unmarashal transmission info: %w", err)
	}

	if transmissionInfo.Success {
		ts.lggr.Infow("returning without a transmission attempt - report already onchain ", "executionID", request.Metadata.WorkflowExecutionID)
		return &wt.TransmissionState{
			Status:      wt.TransmissionStateSucceeded,
			Transmitter: transmissionInfo.Transmitter.String(),
			Err:         nil,
		}, nil
	}

	ts.lggr.Infow("returning without a transmission attempt - transmission already attempted and failed, sufficient gas was provided",
		"executionID", request.Metadata.WorkflowExecutionID)

	return &wt.TransmissionState{
		Status:      wt.TransmissionStateFatal,
		Transmitter: transmissionInfo.Transmitter.String(),
		Err:         errors.New("submitted transaction failed"),
	}, nil
}

func (ts *targetStrategy) TransmitReport(ctx context.Context, report []byte, reportContext []byte, signatures [][]byte, request capabilities.CapabilityRequest) (string, error) {
	txID, err := uuid.NewUUID() // NOTE: CW expects us to generate an ID, rather than return one
	if err != nil {
		return "", err
	}

	r, err := getRequest(request)
	if err != nil {
		return txID.String(), fmt.Errorf("invalid capability request: %w", err)
	}

	blockhash, err := ts.client.LatestBlockhash(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get latest blockhash: %w", err)
	}

	tx, err := ts.newTransaction(r, blockhash.Value.Blockhash)
	if err != nil {
		return "", fmt.Errorf("failed to create solana tx: %w", err)
	}

	txSize, err := chainwriter.CalculateTxSize(tx)
	if err != nil {
		return "", fmt.Errorf("failed calculate tx size: %w", err)
	}

	if txSize > chainwriter.MaxSolanaTxSize {
		return "", fmt.Errorf("transaction size:%d exceeds solana max tx size:%d", txSize, chainwriter.MaxSolanaTxSize)
	}

	transactionID := txID.String()
	if err := ts.txm.Enqueue(ctx, ts.forwarder, tx, &transactionID, blockhash.Value.LastValidBlockHeight); err != nil {
		return "", fmt.Errorf("failed to submit transaction: %w", err)
	}

	return txID.String(), nil
}

type Config struct {
	Address string
}

type Acc struct {
	Address    string
	IsWritable bool
}

type Inputs struct {
	SignedReport      ocr3types.SignedReport
	RemainingAccounts []Acc
}

type targetRequest struct {
	Metadata capabilities.RequestMetadata
	Config   Config
	Receiver solana.PublicKey
	Inputs   Inputs
}

func (ts *targetRequest) toPayload() []byte {
	var ret []byte

	report := ts.Inputs.SignedReport

	// 1. data_size ret[0]
	ret[0] = byte(len(report.Signatures))

	// 2. add N signatures
	for _, sig := range report.Signatures {
		ret = append(ret, sig...)
	}

	// 3. add raw report
	ret = append(ret, report.Report...)

	// 4. add context
	ret = append(ret, report.Context...)

	return ret
}

var (
	remainingAccounts = "remaining_accounts"
)

func getRequest(rawRequest capabilities.CapabilityRequest) (*targetRequest, error) {
	r := &targetRequest{}
	r.Metadata = rawRequest.Metadata

	if rawRequest.Config == nil {
		return r, errors.New("missing config field")
	}

	if err := rawRequest.Config.UnwrapTo(&r.Config); err != nil {
		return r, err
	}

	receiver, err := solana.PublicKeyFromBase58(r.Config.Address)
	if err != nil {
		return r, fmt.Errorf("'%v' is not a valid public key :%w", r.Config.Address, err)
	}
	r.Receiver = receiver

	if rawRequest.Inputs == nil {
		return r, errors.New("missing inputs field")
	}

	// required field of target's config in the workflow spec
	signedReport, ok := rawRequest.Inputs.Underlying[wt.KeySignedReport]
	if !ok {
		return r, fmt.Errorf("missing required field %s", wt.KeySignedReport)
	}

	if err := signedReport.UnwrapTo(&r.Inputs.SignedReport); err != nil {
		return r, err
	}

	remainingAccs, ok := rawRequest.Inputs.Underlying[remainingAccounts]
	if !ok {
		return r, fmt.Errorf("missing required field %s", remainingAccounts)
	}

	if err := remainingAccs.UnwrapTo(r.Inputs.RemainingAccounts); err != nil {
		return r, err
	}

	for _, acc := range r.Inputs.RemainingAccounts {
		_, err := solana.PublicKeyFromBase58(acc.Address)
		if err != nil {
			return r, fmt.Errorf("failed parse public key from remaining account %v err:%w", acc, err)
		}
	}

	return r, nil
}

func (ts *targetStrategy) newTransaction(r *targetRequest, blockHash solana.Hash) (*solana.Transaction, error) {
	executionState, err := ts.deriveExecutionState(r)
	if err != nil {
		return nil, err
	}

	inst := ks_forwarder.NewReportInstruction(
		r.toPayload(),
		ts.accounts.state,
		ts.accounts.oraclesConfig,
		ts.accounts.transmitter,
		ts.accounts.forwarderAuthority,
		executionState,
		r.Receiver,
		solana.SystemProgramID,
	)

	lookup := make(map[solana.PublicKey]solana.PublicKeySlice)
	maps.Copy(lookup, ts.lookupTable)

	for _, acc := range r.Inputs.RemainingAccounts {
		key, err := solana.PublicKeyFromBase58(acc.Address)
		if err != nil {
			return nil, fmt.Errorf("failed parse remaining account key: %w", err)
		}

		inst.AccountMetaSlice = append(inst.AccountMetaSlice, &solana.AccountMeta{
			PublicKey:  key,
			IsWritable: acc.IsWritable,
		})

		lookup[ts.accounts.lookupTable] = append(lookup[ts.accounts.lookupTable], key)

	}
	tx, err := inst.ValidateAndBuild()
	if err != nil {
		return nil, fmt.Errorf("failed build and validate report instruction: %w", err)
	}

	return solana.NewTransaction([]solana.Instruction{tx}, blockHash,
		solana.TransactionPayer(ts.accounts.transmitter),
		solana.TransactionAddressTables(lookup),
	)
}

// Wrapper around the ChainWriter to get the transaction status
func (ts *targetStrategy) GetTransactionStatus(ctx context.Context, transactionID string) (commontypes.TransactionStatus, error) {
	return ts.txm.GetTransactionStatus(ctx, transactionID)
}

func (ts *targetStrategy) deriveExecutionState(r *targetRequest) (solana.PublicKey, error) {
	transmissionID, err := extractTransmissionID(r.Receiver, r.Inputs.SignedReport.Report)
	if err != nil {
		return solana.PublicKey{}, err
	}

	seeds := [][]byte{
		[]byte("execution_state"),
		ts.accounts.state.Bytes(),
		transmissionID[:],
	}

	ret, _, err := solana.FindProgramAddress(seeds, ts.accounts.forwarderID)

	return ret, err
}

var (
	reportIDOffset    = 107
	reporIDSize       = 2
	executionIDOffset = 1
	executionIDSize   = 32
)

func extractTransmissionID(receiver solana.PublicKey, rawReport []byte) ([32]byte, error) {
	var data []byte

	if len(rawReport) <= reportIDOffset+reporIDSize {
		return [32]byte{}, fmt.Errorf("invalid len of raw report: %d", len(rawReport))
	}

	// 1. add receiver
	data = append(data, receiver.Bytes()...)

	// 2. add executionID
	executionID := rawReport[executionIDOffset : executionIDOffset+executionIDSize]
	data = append(data, executionID...)

	// 3. add reportID
	reportID := rawReport[executionIDOffset : executionIDOffset+executionIDSize]
	data = append(data, reportID...)

	return sha3.Sum256(data), nil
}
