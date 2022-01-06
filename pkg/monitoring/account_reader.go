package monitoring

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	pkgSolana "github.com/smartcontractkit/chainlink-solana/pkg/solana"
)

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
	state, blockNum, err := pkgSolana.GetState(ctx, s.client, stateAccount)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch state : %w", err)
	}
	return StateEnvelope{state, blockNum}, nil
}
