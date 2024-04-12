package types

import (
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

var (
	TxDetailsType = "txdetails"
)

// TxDetails expands on TxResults and contains additional detail on a set of tx signatures specific to solana
type TxDetails struct {
	count int // total signatures processed

	// TODO: PerOperator categorizes TxResults based on sender/operator
	// PerOperator map[string]commonMonitoring.TxResults

	// observation counts within each report
	obsLatest     int // number of observations in latest included report/tx
	obsAvg        int // average number of observations across all seen txs/reports from operators
	obsSuccessAvg int // average number of observations included in successful reports
	obsFailedAvg  int // average number of observations included in failed reports

	// TODO: implement - parse fee using shared logic from fee/computebudget.go
	// feeAvg
	// feeSuccessAvg
	// feeFailedAvg
}

type ParsedTx struct {
	Err interface{}
	Fee uint64

	Sender   solana.PublicKey
	Operator string // human readable name associated to public key

	// report information
	ObservationCount int
}

func ParseTx(txResult *rpc.GetTransactionResult, nodes map[solana.PublicKey]string) (ParsedTx, error) {
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
	if tx == nil {
		return ParsedTx{}, fmt.Errorf("GetTransaction returned nil")
	}

	// determine sender
	// if more than 1 tx signature, then it is not a data feed report tx from a CL node -> ignore
	if len(tx.Signatures) != 1 || len(tx.Message.AccountKeys) == 0 {
		return ParsedTx{}, nil
	}
	// from docs: https://solana.com/docs/rpc/json-structures#transactions
	// A list of base-58 encoded signatures applied to the transaction.
	// The list is always of length message.header.numRequiredSignatures and not empty.
	// The signature at index i corresponds to the public key at index i in message.accountKeys.
	sender := tx.Message.AccountKeys[0]

	// if sender matches a known node/operator + sending to feed account, parse transmit tx data
	// node transmit calls the fallback which is difficult to filter
	if _, ok := nodes[sender]; !ok {
		return ParsedTx{}, nil
	}

	var found bool // found transmit instruction
	for _, instruction := range tx.Message.Instructions {
		// protect against invalid index
		if int(instruction.ProgramIDIndex) >= len(tx.Message.AccountKeys) {
			continue
		}

		// find OCR2 transmit instruction
		// TODO: fix hardcoding OCR2 program ID - SolanaFeedConfig.ContractAddress
		if !found && // only can find one transmit instruction
			tx.Message.AccountKeys[instruction.ProgramIDIndex].String() == "cjg3oHmg9uuPsP8D6g29NWvhySJkdYdAo9D25PRbKXJ" {
			found = true

			// TODO: parse report
		}

		// TODO: parsing fee calculation
		// ComputeBudget111111111111111111111111111111
	}

	return ParsedTx{
		Err:      txResult.Meta.Err,
		Fee:      txResult.Meta.Fee,
		Sender:   sender,
		Operator: nodes[sender],
	}, nil
}
