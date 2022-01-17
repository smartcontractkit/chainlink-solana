package monitoring

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	pkgSolana "github.com/smartcontractkit/chainlink-solana/pkg/solana"
)

// ChainReader is a wrapper on top of the chain-specific RCP client.
type ChainReader interface {
	Read(ctx context.Context, address []byte) (interface{}, error)
}

type TransmissionEnvelope struct {
	Answer      pkgSolana.Answer
	BlockNumber uint64
}

func NewTransmissionReader(client *rpc.Client) ChainReader {
	return &trReader{client}
}

type trReader struct {
	client *rpc.Client
}

func (t *trReader) Read(ctx context.Context, transmissionsAccountRaw []byte) (interface{}, error) {
	transmissionsAccount := solana.PublicKeyFromBytes(transmissionsAccountRaw)
	answer, blockNum, err := pkgSolana.GetLatestTransmission(ctx, t.client, transmissionsAccount)
	return TransmissionEnvelope{answer, blockNum}, err
}

func NewStateReader(client *rpc.Client) ChainReader {
	return &stReader{client}
}

type stReader struct {
	client *rpc.Client
}

type StateEnvelope struct {
	State       pkgSolana.State
	BlockNumber uint64
}

func (s *stReader) Read(ctx context.Context, stateAccountRaw []byte) (interface{}, error) {
	stateAccount := solana.PublicKeyFromBytes(stateAccountRaw)
	state, blockNum, err := pkgSolana.GetState(ctx, s.client, stateAccount)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch state : %w", err)
	}
	return StateEnvelope{state, blockNum}, nil
}
