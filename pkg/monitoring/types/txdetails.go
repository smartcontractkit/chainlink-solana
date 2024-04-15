package types

import (
	"errors"
	"fmt"

	solanaGo "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/libocr/offchainreporting2/types"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
)

var (
	TxDetailsType = "txdetails"
)

// TxDetails expands on TxResults and contains additional detail on a set of tx signatures specific to solana
type TxDetails struct {
	Count int // total signatures processed

	// TODO: PerOperator categorizes TxResults based on sender/operator
	// PerOperator map[string]commonMonitoring.TxResults

	// observation counts within each report
	ObsLatest  uint8   // number of observations in latest included report/tx
	ObsAll     []uint8 // observations across all seen txs/reports from operators
	ObsSuccess []uint8 // observations included in successful reports
	ObsFailed  []uint8 // observations included in failed reports

	// TODO: implement - parse fee using shared logic from fee/computebudget.go
	// FeeAvg
	// FeeSuccessAvg
	// FeeFailedAvg
}

type ParsedTx struct {
	Err interface{}
	Fee uint64

	Sender   solanaGo.PublicKey
	Operator string // human readable name associated to public key

	// report information - only supports single report per tx
	ObservationCount uint8
}

// ParseTxResult parses the GetTransaction RPC response
func ParseTxResult(txResult *rpc.GetTransactionResult, nodes map[solanaGo.PublicKey]string, programAddr solanaGo.PublicKey) (ParsedTx, error) {
	if txResult == nil {
		return ParsedTx{}, fmt.Errorf("txResult is nil")
	}
	if txResult.Meta == nil {
		return ParsedTx{}, fmt.Errorf("txResult.Meta is nil")
	}
	if txResult.Transaction == nil {
		return ParsedTx{}, fmt.Errorf("txResult.Transaction is nil")
	}
	// get original tx
	tx, err := txResult.Transaction.GetTransaction()
	if err != nil {
		return ParsedTx{}, fmt.Errorf("GetTransaction: %w", err)
	}

	details, err := ParseTx(tx, nodes, programAddr)
	if err != nil {
		return ParsedTx{}, fmt.Errorf("ParseTx: %w", err)
	}

	// append more details from tx meta
	details.Err = txResult.Meta.Err
	details.Fee = txResult.Meta.Fee
	return details, nil
}

// ParseTx parses a solana transaction
func ParseTx(tx *solanaGo.Transaction, nodes map[solanaGo.PublicKey]string, programAddr solanaGo.PublicKey) (ParsedTx, error) {
	if tx == nil {
		return ParsedTx{}, fmt.Errorf("tx is nil")
	}
	if nodes == nil {
		return ParsedTx{}, fmt.Errorf("nodes is nil")
	}

	// determine sender
	// if more than 1 tx signature, then it is not a data feed report tx from a CL node -> ignore
	if len(tx.Signatures) != 1 || len(tx.Message.AccountKeys) == 0 {
		return ParsedTx{}, fmt.Errorf("invalid number of signatures")
	}
	// from docs: https://solana.com/docs/rpc/json-structures#transactions
	// A list of base-58 encoded signatures applied to the transaction.
	// The list is always of length message.header.numRequiredSignatures and not empty.
	// The signature at index i corresponds to the public key at index i in message.accountKeys.
	sender := tx.Message.AccountKeys[0]

	// if sender matches a known node/operator + sending to feed account, parse transmit tx data
	// node transmit calls the fallback which is difficult to filter for the function call
	if _, ok := nodes[sender]; !ok {
		return ParsedTx{}, fmt.Errorf("unknown public key: %s", sender)
	}

	var obsCount uint8
	var totalErr error
	for _, instruction := range tx.Message.Instructions {
		// protect against invalid index
		if int(instruction.ProgramIDIndex) >= len(tx.Message.AccountKeys) {
			continue
		}

		// find OCR2 transmit instruction at specified program address
		if tx.Message.AccountKeys[instruction.ProgramIDIndex] == programAddr {
			// parse report from tx data (see solana/transmitter.go)
			start := solana.StoreNonceLen + solana.ReportContextLen
			end := start + int(solana.ReportLen)
			report := types.Report(instruction.Data[start:end])
			count, err := solana.ReportCodec{}.ObserversCountFromReport(report)
			if err != nil {
				totalErr = errors.Join(totalErr, fmt.Errorf("%w (%+v)", err, instruction))
				continue
			}
			obsCount = count
		}

		// find compute budget program instruction
		if tx.Message.AccountKeys[instruction.ProgramIDIndex] == solanaGo.MustPublicKeyFromBase58(fees.COMPUTE_BUDGET_PROGRAM) {
			// TODO: parsing fee calculation
		}
	}

	return ParsedTx{
		Sender:           sender,
		Operator:         nodes[sender],
		ObservationCount: obsCount,
	}, totalErr
}
