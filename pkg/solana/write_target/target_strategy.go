package writetarget

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/google/uuid"
	"github.com/smartcontractkit/chainlink-common/pkg/capabilities"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
	wt "github.com/smartcontractkit/chainlink-framework/capabilities/writetarget"
)

var (
	_ wt.TargetStrategy = &targetStrategy{}
)

type targetStrategy struct {
	cw        commontypes.ContractWriter
	cr        commontypes.ContractReader
	forwarder string

	lggr logger.Logger
}

func newTargetStrategy(cw commontypes.ContractWriter,
	cr commontypes.ContractReader,
	forwarder string,
	lggr logger.Logger,
) wt.TargetStrategy {
	return &targetStrategy{
		cw:        cw,
		cr:        cr,
		forwarder: forwarder,
		lggr:      lggr,
	}
}

// QueryTransmissionState defines how the report should be queried
// via ChainReader, and how resulting errors should be classified.
func (ts *targetStrategy) QueryTransmissionState(ctx context.Context, reportID uint16, request capabilities.CapabilityRequest) (*wt.TransmissionState, error) {
	// TODO
	return nil, nil
}

// TransmitReport constructs the tx to transmit the report, and defines
// any specific handling for sending the report via ChainWriter.
func (ts *targetStrategy) TransmitReport(ctx context.Context, report []byte, reportContext []byte, signatures [][]byte, request capabilities.CapabilityRequest) (string, error) {
	txID, err := uuid.NewUUID() // NOTE: CW expects us to generate an ID, rather than return one
	if err != nil {
		return "", err
	}

	req := struct {
		Data []byte
	}{
		// TODO create request
	}

	if req.Data == nil {
		req.Data = make([]byte, 0)
	}

	ts.lggr.Debugw("Transaction raw report", "report", hex.EncodeToString(req.Data))

	if err := ts.cw.SubmitTransaction(ctx, "forwarder", "report", req, txID.String(), ts.forwarder, nil, nil); err != nil {
		return txID.String(), fmt.Errorf("failed to submit transaction: %w", err)
	}

	return txID.String(), nil

}

// Wrapper around the ChainWriter to get the transaction status
func (ts *targetStrategy) GetTransactionStatus(ctx context.Context, transactionID string) (commontypes.TransactionStatus, error) {
	// TODO
	return commontypes.TransactionStatus(0), nil
}
