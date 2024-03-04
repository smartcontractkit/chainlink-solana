package targets_test

import (
	"context"
	"testing"
	"time"

	sdk "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/smartcontractkit/chainlink-common/pkg/capabilities"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/values"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/capabilities/targets"
	clientmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	mocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/mocks"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func ptr[T any](t T) *T {
	return &t
}

func TestSolanaWrite(t *testing.T) {
	lggr := logger.Test(t)
	cc := config.Chain{}
	cc.SetDefaults()
	cc.ChainWriter = &config.ChainWriter{
		FromAddress:           sdk.MustPublicKeyFromBase58("SysvarS1otHashes111111111111111111111111111"),
		ForwarderProgramID:    sdk.MustPublicKeyFromBase58("SysvarC1ock11111111111111111111111111111111"),
		ForwarderStateAccount: sdk.MustPublicKeyFromBase58("Vote111111111111111111111111111111111111111"),
	}
	tomlConfig := solana.TOMLConfig{
		ChainID: ptr("devnet"),
		Enabled: ptr(true),
		Chain:   cc,
	}

	chain := mocks.NewChain(t)
	chain.On("ID").Return(*tomlConfig.ChainID)
	chain.On("Config").Return(&tomlConfig)
	txManager := mocks.NewTxManager(t)
	chain.On("TxManager").Return(txManager)
	client := clientmocks.NewReaderWriter(t)
	client.On("LatestBlockhash").Return(
		&rpc.GetLatestBlockhashResult{
			RPCContext: rpc.RPCContext{},
			Value: &rpc.LatestBlockhashResult{
				Blockhash:            [32]byte{},
				LastValidBlockHeight: 0,
			},
		},
		nil,
	)
	chain.On("Reader").Return(client, nil)

	capability, err := targets.NewSolanaWrite(chain, lggr)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	config, err := values.NewMap(map[string]any{
		// "chain_id": ,
		// "receiver_program_id": sdk.MustPublicKeyFromBase58("11111111111111111111111111111111"),
		// "abi":    "receive(bytes report)",
		"params": []any{"$(report)"},
		"accounts": []any{
			map[string]any{"public_key": "A9QnpgfhCkmiBSjgBuWk76Wo3HxzxvDopUq9x6UUMmjn", "is_writable": true, "is_signer": true},
		},
	})
	require.NoError(t, err)

	inputs, err := values.NewMap(map[string]any{
		"report": []byte{1, 2, 3},
	})
	require.NoError(t, err)

	req := capabilities.CapabilityRequest{
		Metadata: capabilities.RequestMetadata{
			WorkflowID: "hello",
		},
		Config: config,
		Inputs: inputs,
	}

	txManager.On("Enqueue", mock.Anything, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		// TODO: tx asserts
		req := args.Get(1).(*sdk.Transaction)
		msg := req.Message
		ix := msg.Instructions[0]
		require.Equal(t, len(ix.Accounts), 4+1) // 4 required by forwarder, 1 passed through from reqConfig
		// TODO: assert on ix.Data
	})

	ch := make(chan capabilities.CapabilityResponse)

	err = capability.Execute(ctx, ch, req)
	require.NoError(t, err)

	response := <-ch
	require.Nil(t, response.Err)
}
