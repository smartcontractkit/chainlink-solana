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

	Sender solana.PublicKey

	// report information
	ObservationCount int
}

func ParseTx(txResult *rpc.GetTransactionResult) (ParsedTx, error) {
	out := ParsedTx{}
	if txResult == nil {
		return out, fmt.Errorf("txResult is nil")
	}
	if txResult.Meta == nil {
		return out, fmt.Errorf("txResult.Meta is nil")
	}

	out.Err = txResult.Meta.Err
	out.Fee = txResult.Meta.Fee

	// determine sender

	// find OCR2 transmit instruction

	return out, nil
}
