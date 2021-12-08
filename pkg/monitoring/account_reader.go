package monitoring

import (
	"context"

	bin "github.com/gagliardetto/binary"
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
	res, err := s.client.GetAccountInfo(ctx, stateAccount)
	if err != nil {
		return nil, err
	}

	state := pkgSolana.State{}
	if err := bin.NewBinDecoder(res.Value.Data.GetBinary()).Decode(state); err != nil {
		return nil, err
	}

	blockNum := res.RPCContext.Context.Slot
	return StateEnvelope{state, blockNum}, nil
}
