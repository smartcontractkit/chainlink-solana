package monitoring

import (
	"context"
	"fmt"
	"sync"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/pkg/errors"
	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
	pkgSolana "github.com/smartcontractkit/chainlink-solana/pkg/solana"
	"go.uber.org/multierr"
)

// NewTransmissionAccountSource builds a source of []Account each with a pkgSolana.Transmission instance
func NewTransmissionAccountSource(
	client *rpc.Client,
	accounts []solana.PublicKey,
	log relayMonitoring.Logger,
	commitment rpc.CommitmentType,
) relayMonitoring.Source {
	return &transmissionAccountSource{
		client,
		accounts,
		log,
		commitment,
	}
}

type transmissionAccountSource struct {
	client     *rpc.Client
	accounts   []solana.PublicKey
	log        relayMonitoring.Logger
	commitment rpc.CommitmentType
}

func (t *transmissionAccountSource) GetType() string {
	return "transmission"
}

func (t *transmissionAccountSource) Fetch(ctx context.Context) (interface{}, error) {
	if len(t.accounts) == 0 {
		return nil, relayMonitoring.ErrNoUpdate
	}
	headers, accounts, err := t.getTransmissionsHeaders(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch headers for transmissions accounts: %w", err)
	}

	transmissions := make([]pkgSolana.Transmission, len(headers))
	var transmissionsErr error
	transmissionMu := &sync.Mutex{}
	wg := &sync.WaitGroup{}
	wg.Add(len(headers))
	for i, header := range headers {
		go func(i int) {
			defer wg.Done()
			transmission, err := t.getLatestTransmissionByHeader(ctx, t.accounts[i], header)
			transmissionMu.Lock()
			defer transmissionMu.Unlock()
			if err != nil {
				transmissionsErr = multierr.Combine(transmissionsErr, err)
			} else {
				transmissions[i] = transmission
			}
		}(i)
	}
	wg.Wait()
	if transmissionsErr != nil {
		return nil, fmt.Errorf("failed to fetch latest transmissions: %w", err)
	}
	output := make([]Account, len(t.accounts))
	for i := 0; i < len(t.accounts); i++ {
		accounts[i].Data = Transmission{
			Header:             headers[i],
			LatestTransmission: transmissions[i],
		}
		output[i] = accounts[i]
	}
	return output, nil
}

// Helpers

func (t *transmissionAccountSource) getTransmissionsHeaders(
	ctx context.Context,
) (
	[]pkgSolana.TransmissionsHeader,
	[]Account,
	error,
) {
	headerStart := pkgSolana.AccountDiscriminatorLen // skip account discriminator
	headerLen := pkgSolana.TransmissionsHeaderLen
	result, err := t.client.GetMultipleAccountsWithOpts(
		ctx,
		t.accounts,
		&rpc.GetMultipleAccountsOpts{
			Encoding:   solana.EncodingBase64,
			Commitment: t.commitment,
			DataSlice: &rpc.DataSlice{
				Offset: &headerStart,
				Length: &headerLen,
			},
		},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch headers of transmission accounts: %w", err)
	}
	slot := result.RPCContext.Context.Slot
	accounts := make([]Account, len(t.accounts))
	for i, value := range result.Value {
		accounts[i] = Account{
			slot,
			t.accounts[i],
			value.Lamports,
			value.Owner,
			nil,
			value.Executable,
			value.RentEpoch,
		}
	}
	headers := make([]pkgSolana.TransmissionsHeader, len(t.accounts))
	for i, value := range result.Value {
		header := pkgSolana.TransmissionsHeader{}
		if err = bin.NewBinDecoder(value.Data.GetBinary()).Decode(&header); err != nil {
			return nil, nil, fmt.Errorf("failed to decode header for account '%s': %w", t.accounts[i], err)
		}
		headers[i] = header
	}
	return headers, accounts, nil
}

func (t *transmissionAccountSource) getLatestTransmissionByHeader(
	ctx context.Context,
	account solana.PublicKey,
	header pkgSolana.TransmissionsHeader,
) (
	pkgSolana.Transmission,
	error,
) {
	transmission := pkgSolana.Transmission{}
	if header.Version != 2 {
		return transmission, fmt.Errorf("can't parse feed version %v", header.Version)
	}
	cursor := header.LiveCursor
	liveLength := header.LiveLength
	if cursor == 0 { // handle array wrap
		cursor = liveLength
	}
	cursor-- // cursor indicates index for new answer, latest answer is in previous index
	// setup transmissionLen
	transmissionLen := pkgSolana.TransmissionLen
	var transmissionOffset = pkgSolana.AccountDiscriminatorLen + pkgSolana.TransmissionsHeaderMaxSize + (uint64(cursor) * transmissionLen)
	result, err := t.client.GetAccountInfoWithOpts(
		ctx,
		account,
		&rpc.GetAccountInfoOpts{
			Encoding:   solana.EncodingBase64,
			Commitment: t.commitment,
			DataSlice: &rpc.DataSlice{
				Offset: &transmissionOffset,
				Length: &transmissionLen,
			},
		},
	)
	if err != nil {
		return transmission, fmt.Errorf("failed to fetch transmission slice: %w", err)
	}
	// check for nil pointers
	if result == nil || result.Value == nil || result.Value.Data == nil {
		return transmission, errors.New("nil pointer returned in received")
	}
	// parse tranmission
	if err := bin.NewBinDecoder(result.Value.Data.GetBinary()).Decode(&transmission); err != nil {
		return transmission, errors.Wrap(err, "failed to decode transmission")
	}
	return transmission, nil
}
