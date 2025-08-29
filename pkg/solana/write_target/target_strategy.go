package writetarget

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"

	"encoding/binary"

	"github.com/gagliardetto/solana-go"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/smartcontractkit/chainlink-common/pkg/capabilities"
	ocr3types "github.com/smartcontractkit/chainlink-common/pkg/capabilities/consensus/ocr3/types"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
	wt "github.com/smartcontractkit/chainlink-framework/capabilities/writetarget"
	ks_forwarder "github.com/smartcontractkit/chainlink-solana/contracts/generated/keystone_forwarder"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainwriter"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"

	"github.com/ethereum/go-ethereum/crypto"
	sol_binary "github.com/gagliardetto/binary"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/utils"
	txmutils "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/utils"
)

var (
	_ wt.TargetStrategy = &targetStrategy{}
)

type targetStrategy struct {
	client client.Reader
	txm    Txm

	accounts    accounts
	lookupTable map[solana.PublicKey]solana.PublicKeySlice
	lggr        logger.Logger
}

// All known in advance accounts
type accounts struct {
	forwarderState     solana.PublicKey
	forwarderProgramID solana.PublicKey
	transmitter        solana.PublicKey
	lookupTable        solana.PublicKey
}

type Txm interface {
	Enqueue(ctx context.Context, accountID string, tx *solana.Transaction, txID *string, txLastValidBlockHeight uint64, txCfgs ...txmutils.SetTxConfig) error
	GetTransactionStatus(ctx context.Context, transactionID string) (commontypes.TransactionStatus, error)
}

func newTargetStrategy(client client.Reader, txm Txm, cfg config.Workflow, lggr logger.Logger) (wt.TargetStrategy, error) {
	lookup := make(map[solana.PublicKey]solana.PublicKeySlice)

	accs, err := accountsFromConfig(cfg)
	if err != nil {
		return nil, err
	}

	lookup[accs.lookupTable] = solana.PublicKeySlice{
		accs.forwarderState,
		accs.forwarderProgramID,
		accs.transmitter,
		solana.SystemProgramID,
	}

	ks_forwarder.SetProgramID(accs.forwarderProgramID)

	return &targetStrategy{
		client:      client,
		txm:         txm,
		accounts:    accs,
		lggr:        lggr,
		lookupTable: lookup,
	}, nil
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
	if err != nil && !errors.Is(err, rpc.ErrNotFound) {
		return nil, fmt.Errorf("failed to get transmission state latest value: %w", err)
	}

	if errors.Is(err, rpc.ErrNotFound) || acc.Value == nil {
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

	err = sol_binary.UnmarshalBorsh(&transmissionInfo, acc.Bytes())
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

	verifySignature(ts.lggr, r)

	blockhash, err := ts.client.LatestBlockhash(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get latest blockhash: %w", err)
	}

	configPDA, err := ts.getOracleConfigPDA(ctx, r.Metadata.WorkflowDonID, r.Metadata.WorkflowDonConfigVersion)
	if err != nil {
		return "", fmt.Errorf("failed to get oracle config PDA: %w", err)
	}

	tx, err := ts.newTransaction(r, configPDA, blockhash.Value.Blockhash)
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
	options := []utils.SetTxConfig{txmutils.SetEstimateComputeUnitLimit(false), txmutils.SetComputeUnitLimit(500_000)}
	if err := ts.txm.Enqueue(ctx, ts.accounts.forwarderProgramID.String(), tx, &transactionID, blockhash.Value.LastValidBlockHeight, options...); err != nil {
		return "", fmt.Errorf("failed to submit transaction: %w", err)
	}

	return txID.String(), nil
}

func verifySignature(lggr logger.Logger, r *targetRequest) {
	lggr.Debug("verify signature")
	var raw []byte
	raw = append(raw, r.Inputs.SignedReport.Report...)
	raw = append(raw, r.Inputs.SignedReport.Context...)
	h := sha256.Sum256(raw)
	for _, sig := range r.Inputs.SignedReport.Signatures {
		rb := new(big.Int).SetBytes(sig[:32])
		s := new(big.Int).SetBytes(sig[32:])
		v := sig[64]
		if !crypto.ValidateSignatureValues(v, rb, s, true) {
			lggr.Debug("validate signature failed")
		}

		pubKey, err := crypto.SigToPub(h[:], sig)
		if err != nil {
			lggr.Errorf("recovered pubkey failed: %w", err)
			return
		}
		addr := crypto.PubkeyToAddress(*pubKey)
		lggr.Debugf("signer public address %x", addr.Bytes())
	}

}

// Wrapper around the ChainWriter to get the fee esimate
func (ts *targetStrategy) GetEstimateFee(ctx context.Context, report []byte, reportContext []byte, signatures [][]byte, request capabilities.CapabilityRequest) (commontypes.EstimateFee, error) {
	return commontypes.EstimateFee{}, errors.New("unimplemented")
}

// GetTransactionFee retrieves the actual transaction fee in native currency from the transaction receipt.
// This method should be implemented by chain-specific services and handle the conversion of gas units to native currency.
func (ts *targetStrategy) GetTransactionFee(ctx context.Context, transactionID string) (decimal.Decimal, error) {
	return decimal.Decimal{}, errors.New("unimplemented")
}

type Config struct {
	Address string `mapstructure:"address"`
}

type Acc struct {
	Address    string
	IsWritable bool
}

type Inputs struct {
	SignedReport      ocr3types.SignedReport
	RemainingAccounts solana.AccountMetaSlice
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
	ret = append(ret, byte(len(report.Signatures)))

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
	remainingAccountsKey = "remaining_accounts"
)

func getRequest(rawRequest capabilities.CapabilityRequest) (*targetRequest, error) {
	r := &targetRequest{}
	r.Metadata = rawRequest.Metadata

	if rawRequest.Config == nil {
		return r, errors.New("missing config field")
	}

	if err := rawRequest.Config.UnwrapTo(&r.Config); err != nil {
		return r, fmt.Errorf("failed to unwrap config field: %w", err)
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
		return r, fmt.Errorf("failed to unwrap SignedReport: %w", err)
	}

	remaings, ok := rawRequest.Inputs.Underlying[remainingAccountsKey]
	if !ok {
		return r, fmt.Errorf("missing required field %s", remainingAccountsKey)
	}

	if err := remaings.UnwrapTo(&r.Inputs.RemainingAccounts); err != nil {
		return r, err
	}

	return r, nil
}

// auto-detects base-10, base-8, and base-16 only
func validateBytes16(s string) bool {
	n, ok := new(big.Int).SetString(s, 0)
	if !ok {
		return false
	}
	return n.BitLen() <= 128
}

func (ts *targetStrategy) newTransaction(r *targetRequest, oracleConfigPDA solana.PublicKey, blockHash solana.Hash) (*solana.Transaction, error) {
	executionState, err := ts.deriveExecutionState(r)
	if err != nil {
		return nil, err
	}

	authority, err := deriveForwarderAuthority(ts.accounts.forwarderState, r.Receiver, ts.accounts.forwarderProgramID)
	if err != nil {
		return nil, err
	}
	inst := ks_forwarder.NewReportInstruction(
		r.toPayload(),
		ts.accounts.forwarderState,
		oracleConfigPDA,
		ts.accounts.transmitter,
		authority,
		executionState,
		r.Receiver,
		solana.SystemProgramID,
	)

	// append remainings except for forwarderState + Authority
	inst.AccountMetaSlice = append(inst.AccountMetaSlice, r.Inputs.RemainingAccounts[2:]...)
	tx, err := inst.ValidateAndBuild()
	if err != nil {
		return nil, fmt.Errorf("failed build and validate report instruction: %w", err)
	}

	return solana.NewTransaction([]solana.Instruction{tx}, blockHash,
		solana.TransactionPayer(ts.accounts.transmitter),
	)
}

func (ts *targetStrategy) getOracleConfigPDA(ctx context.Context, workflowDonID, configVersion uint32) (solana.PublicKey, error) {
	oracleConfigPDA := getConfigPDA(ts.accounts.forwarderState, workflowDonID, configVersion, ts.accounts.forwarderProgramID)

	oracleConfigAccount, err := ts.client.GetAccountInfoWithOpts(ctx, oracleConfigPDA, &rpc.GetAccountInfoOpts{Commitment: rpc.CommitmentProcessed})
	if err != nil {
		return oracleConfigPDA, fmt.Errorf("error fetching cache state account %v; err: %w", oracleConfigPDA, err)
	}

	if oracleConfigAccount.Value == nil {
		return oracleConfigPDA, fmt.Errorf("cache state account does not exist %v", oracleConfigPDA)
	}

	return oracleConfigPDA, err

}

func getConfigPDA(statePubkey solana.PublicKey, donID uint32, configVersion uint32, programID solana.PublicKey) solana.PublicKey {
	configID := getConfigID(donID, configVersion)
	reqIDBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(reqIDBytes, configID)

	seeds := [][]byte{
		[]byte("config"),
		statePubkey.Bytes(),
		reqIDBytes,
	}

	addr, _, _ := solana.FindProgramAddress(seeds, programID)
	return addr
}

func getConfigID(donID uint32, configVersion uint32) uint64 {
	return (uint64(donID) << 32) | uint64(configVersion)
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
		ts.accounts.forwarderState.Bytes(),
		transmissionID[:],
	}

	ret, _, err := solana.FindProgramAddress(seeds, ts.accounts.forwarderProgramID)

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
	reportID := rawReport[reportIDOffset : reportIDOffset+reporIDSize]
	data = append(data, reportID...)

	return sha256.Sum256(data), nil
}

func deriveForwarderAuthority(forwarderState solana.PublicKey, receiverProgram solana.PublicKey, forwarderProgram solana.PublicKey) (solana.PublicKey, error) {
	seeds := [][]byte{
		[]byte("forwarder"),
		forwarderState[:],
		receiverProgram[:],
	}
	ret, _, err := solana.FindProgramAddress(seeds, forwarderProgram)

	return ret, err
}

func createReportHash(dataID []byte, forwarderAuthority []byte, workflowOwner []byte, workflowName []byte) [32]byte {
	var data []byte
	data = append(data, dataID...)
	data = append(data, forwarderAuthority...)
	data = append(data, workflowOwner...)
	data = append(data, workflowName...)

	return sha256.Sum256(data)
}

func accountsFromConfig(cfg config.Workflow) (accounts, error) {
	var ret accounts
	var err error
	ret.forwarderProgramID, err = solana.PublicKeyFromBase58(cfg.ForwarderAddress())
	if err != nil {
		return ret, err
	}

	ret.forwarderState, err = solana.PublicKeyFromBase58(cfg.ForwarderState())
	if err != nil {
		return ret, err
	}

	ret.transmitter, err = solana.PublicKeyFromBase58(cfg.FromAddress())
	if err != nil {
		return ret, err
	}

	return ret, nil
}
