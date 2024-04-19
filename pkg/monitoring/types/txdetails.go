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

	ReportObservationMetric = "sol_report_observations"
	TxFeeMetric             = "sol_tx_fee"
	ComputeUnitPriceMetric  = "sol_tx_compute_unit_price"

	TxDetailsMetrics = []string{
		ReportObservationMetric,
		TxFeeMetric,
		ComputeUnitPriceMetric,
	}
)

type TxDetails struct {
	Err  interface{}
	Fee  uint64
	Slot uint64

	Sender solanaGo.PublicKey

	// report tx information - only supports single report per tx
	ObservationCount uint8
	ComputeUnitPrice fees.ComputeUnitPrice
}

func (td TxDetails) Empty() bool {
	return td.Fee == 0 &&
		td.Slot == 0 &&
		td.Sender == solanaGo.PublicKey{} &&
		td.ObservationCount == 0
}

// MakeTxDetails casts an interface to []TxDetails
func MakeTxDetails(in interface{}) ([]TxDetails, error) {
	out, ok := (in).([]TxDetails)
	if !ok {
		return nil, fmt.Errorf("Unable to make type []TxDetails from %T", in)
	}
	return out, nil
}

// ParseTxResult parses the GetTransaction RPC response
func ParseTxResult(txResult *rpc.GetTransactionResult, programAddr solanaGo.PublicKey) (TxDetails, error) {
	if txResult == nil {
		return TxDetails{}, fmt.Errorf("txResult is nil")
	}
	if txResult.Meta == nil {
		return TxDetails{}, fmt.Errorf("txResult.Meta is nil")
	}
	if txResult.Transaction == nil {
		return TxDetails{}, fmt.Errorf("txResult.Transaction is nil")
	}

	// get original tx
	tx, err := txResult.Transaction.GetTransaction()
	if err != nil {
		return TxDetails{}, fmt.Errorf("GetTransaction: %w", err)
	}

	details, err := ParseTx(tx, programAddr)
	if err != nil {
		return TxDetails{}, fmt.Errorf("ParseTx: %w", err)
	}

	// append more details from tx meta
	details.Err = txResult.Meta.Err
	details.Fee = txResult.Meta.Fee
	details.Slot = txResult.Slot
	return details, nil
}

// ParseTx parses a solana transaction
func ParseTx(tx *solanaGo.Transaction, programAddr solanaGo.PublicKey) (TxDetails, error) {
	if tx == nil {
		return TxDetails{}, fmt.Errorf("tx is nil")
	}

	// determine sender
	// if more than 1 tx signature, then it is not a data feed report tx from a CL node -> ignore
	if len(tx.Signatures) != 1 || len(tx.Message.AccountKeys) == 0 {
		return TxDetails{}, fmt.Errorf("invalid number of signatures")
	}
	// from docs: https://solana.com/docs/rpc/json-structures#transactions
	// A list of base-58 encoded signatures applied to the transaction.
	// The list is always of length message.header.numRequiredSignatures and not empty.
	// The signature at index i corresponds to the public key at index i in message.accountKeys.
	sender := tx.Message.AccountKeys[0]

	// CL node DF transactions should only have a compute unit price + ocr2 instruction
	if len(tx.Message.Instructions) != 2 {
		return TxDetails{}, fmt.Errorf("not a node transaction")
	}

	var totalErr error
	var foundTransmit bool
	var foundFee bool
	txDetails := TxDetails{Sender: sender}
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

			// handle invalid length
			if len(instruction.Data) < (solana.StoreNonceLen + solana.ReportContextLen + int(solana.ReportLen)) {
				totalErr = errors.Join(totalErr, fmt.Errorf("transmit: invalid instruction length (%+v)", instruction))
				continue
			}

			report := types.Report(instruction.Data[start:end])
			var err error
			txDetails.ObservationCount, err = solana.ReportCodec{}.ObserversCountFromReport(report)
			if err != nil {
				totalErr = errors.Join(totalErr, fmt.Errorf("%w (%+v)", err, instruction))
				continue
			}
			foundTransmit = true
			continue
		}

		// find compute budget program instruction
		if tx.Message.AccountKeys[instruction.ProgramIDIndex] == solanaGo.MustPublicKeyFromBase58(fees.COMPUTE_BUDGET_PROGRAM) {
			// parsing compute unit price
			var err error
			txDetails.ComputeUnitPrice, err = fees.ParseComputeUnitPrice(instruction.Data)
			if err != nil {
				totalErr = errors.Join(totalErr, fmt.Errorf("computeUnitPrice: %w (%+v)", err, instruction))
				continue
			}
			foundFee = true
			continue
		}
	}
	if totalErr != nil {
		return TxDetails{}, totalErr
	}

	// if missing either instruction, return error
	if !foundTransmit || !foundFee {
		return TxDetails{}, fmt.Errorf("unable to parse both Transmit and Fee instructions")
	}

	return txDetails, nil
}
