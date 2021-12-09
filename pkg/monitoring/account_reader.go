package monitoring

import (
	"context"
	"fmt"
	"time"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	pkgSolana "github.com/smartcontractkit/chainlink-solana/pkg/solana"
)

const solanaAccountReadTimeout = 5 * time.Second

// AccountReader is a wrapper on top of *rpc.Client
type AccountReader interface {
	Read(ctx context.Context, account solana.PublicKey) (interface{}, error)
}

type TransmissionEnvelope struct {
	Answer      pkgSolana.Answer
	BlockNumber uint64
}

func NewTransmissionReader(client *rpc.Client) AccountReader {
	return &trReader{client}
}

type trReader struct {
	client *rpc.Client
}

func (t *trReader) Read(ctx context.Context, transmissionsAccount solana.PublicKey) (interface{}, error) {
	ctx, cancel := context.WithTimeout(ctx, solanaAccountReadTimeout)
	defer cancel()
	answer, blockNum, err := pkgSolana.GetLatestTransmission(ctx, t.client, transmissionsAccount)
	return TransmissionEnvelope{answer, blockNum}, err
}

func NewStateReader(client *rpc.Client) AccountReader {
	return &stReader{client}
}

type stReader struct {
	client *rpc.Client
}

type StateEnvelope struct {
	State       pkgSolana.State
	BlockNumber uint64
}

func (s *stReader) Read(ctx context.Context, stateAccount solana.PublicKey) (interface{}, error) {
	ctx, cancel := context.WithTimeout(ctx, solanaAccountReadTimeout)
	defer cancel()
	res, err := s.client.GetAccountInfo(ctx, stateAccount)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch state account: %w", err)
	}

	state := pkgSolana.State{}
	if err := bin.NewBinDecoder(res.Value.Data.GetBinary()).Decode(state); err != nil {
		return nil, fmt.Errorf("failed to decode state account contents: %w", err)
	}

	blockNum := res.RPCContext.Context.Slot
	return StateEnvelope{state, blockNum}, nil
}
