package monitoring

import (
	"context"
	"fmt"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
	pkgSolana "github.com/smartcontractkit/chainlink-solana/pkg/solana"
)

// NewStateAccountSource builds a source of []Account each with a pkgSolana.State instance
func NewStateAccountSource(
	client *rpc.Client,
	accounts []solana.PublicKey,
	log relayMonitoring.Logger,
	commitment rpc.CommitmentType,
) relayMonitoring.Source {
	return &stateAccountSource{
		client,
		accounts,
		log,
		commitment,
	}
}

type stateAccountSource struct {
	client     *rpc.Client
	accounts   []solana.PublicKey
	log        relayMonitoring.Logger
	commitment rpc.CommitmentType
}

func (s *stateAccountSource) GetType() string {
	return "state"
}

func (s *stateAccountSource) Fetch(ctx context.Context) (interface{}, error) {
	if len(s.accounts) == 0 {
		return nil, relayMonitoring.ErrNoUpdate
	}
	result, err := s.client.GetMultipleAccountsWithOpts(
		ctx,
		s.accounts,
		&rpc.GetMultipleAccountsOpts{
			Encoding:   solana.EncodingBase64,
			Commitment: s.commitment,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch accounts", "error", err)
	}
	if len(s.accounts) != len(result.Value) {
		return nil, fmt.Errorf("received insufficient accounts from the RPC, expected %d but got %d", len(s.accounts), len(result.Value))
	}
	output := make([]Account, len(s.accounts))
	slot := result.RPCContext.Context.Slot
	for i, value := range result.Value {
		state := pkgSolana.State{}
		if err := bin.NewBinDecoder(value.Data.GetBinary()).Decode(&state); err != nil {
			return nil, fmt.Errorf("failed to decode state account (address='%s') from protobuf: %w", s.accounts[i], err)
		}
		output[i] = Account{
			slot,
			s.accounts[i],
			value.Lamports,
			value.Owner,
			state,
			value.Executable,
			value.RentEpoch,
		}
	}

	return output, nil
}
