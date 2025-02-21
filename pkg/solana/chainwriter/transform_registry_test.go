package chainwriter_test

import (
	"context"
	"testing"

	"github.com/gagliardetto/solana-go"
	ccipconsts "github.com/smartcontractkit/chainlink-ccip/pkg/consts"
	"github.com/smartcontractkit/chainlink-ccip/pkg/types/ccipocr3"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/testutils"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainwriter"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	clientmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/mocks"
)

func Test_CCIPExecuteArgsTransform(t *testing.T) {
	ctx := tests.Context(t)
	offrampAddress := chainwriter.GetRandomPubKey(t)
	routerAddress := chainwriter.GetRandomPubKey(t)

	// simplified CCIP Config - only IDLs are required for CCIPExecute ArgsTransform
	ccipCWConfig := chainwriter.ChainWriterConfig{
		Programs: map[string]chainwriter.ProgramConfig{
			ccipconsts.ContractNameOffRamp: {
				IDL: ccipOfframpIDL,
			},
			// Requires only the IDL for the CCIPArgsTransform to fetch the TokenAdminRegistry
			ccipconsts.ContractNameRouter: {
				IDL: ccipRouterIDL,
			},
		},
	}
	// mock client
	rw := clientmocks.NewReaderWriter(t)
	mc := *client.NewMultiClient(func(context.Context) (client.ReaderWriter, error) {
		return rw, nil
	})
	// initialize chain writer
	cw, err := chainwriter.NewSolanaChainWriterService(testutils.NewNullLogger(), mc, nil, nil, ccipCWConfig)
	require.NoError(t, err)

	destTokenAddr := chainwriter.GetRandomPubKey(t)
	poolKeys := []solana.PublicKey{destTokenAddr}
	poolKeys = append(poolKeys, chainwriter.CreateTestPubKeys(t, 1)...)

	args := chainwriter.ReportPreTransform{
		Info: ccipocr3.ExecuteReportInfo{
			AbstractReports: []ccipocr3.ExecutePluginReportSingleChain{{
				Messages: []ccipocr3.Message{{
					TokenAmounts: []ccipocr3.RampTokenAmount{{
						DestTokenAddress: ccipocr3.UnknownAddress(destTokenAddr.Bytes()),
					}},
				}},
			}},
		},
	}

	accounts := []*solana.AccountMeta{{PublicKey: poolKeys[0]}, {PublicKey: poolKeys[1]}}

	t.Run("CCIPExecute ArgsTransform includes token indexes", func(t *testing.T) {
		pda, _, err := solana.FindProgramAddress([][]byte{[]byte("token_admin_registry"), destTokenAddr.Bytes()}, routerAddress)
		require.NoError(t, err)

		lookupTable := mockTokenAdminRegistryLookupTable(t, rw, pda)
		mockFetchRouterAddress(t, rw, routerAddress, offrampAddress)
		mockFetchLookupTableAddresses(t, rw, lookupTable, poolKeys)
		transformedArgs, newAccounts, err := chainwriter.CCIPExecuteArgsTransform(ctx, cw, args, accounts, offrampAddress.String())
		require.NoError(t, err)
		// Accounts should be unchanged
		require.Len(t, newAccounts, 2)
		typedArgs, ok := transformedArgs.(chainwriter.ReportPostTransform)
		require.True(t, ok)
		require.NotNil(t, typedArgs.TokenIndexes)
		require.Len(t, typedArgs.TokenIndexes, 1)
	})
}

func Test_CCIPCommitAccountTransform(t *testing.T) {
	ctx := tests.Context(t)
	offrampAddress := chainwriter.GetRandomPubKey(t)
	key1 := chainwriter.GetRandomPubKey(t)
	key2 := chainwriter.GetRandomPubKey(t)
	t.Run("CCIPCommit ArgsTransform does not affect accounts if token prices exist", func(t *testing.T) {
		args := struct {
			Info ccipocr3.CommitReportInfo
		}{
			Info: ccipocr3.CommitReportInfo{
				TokenPrices: []ccipocr3.TokenPrice{{TokenID: ccipocr3.UnknownEncodedAddress(key1.String())}},
			},
		}
		accounts := []*solana.AccountMeta{{PublicKey: key1}, {PublicKey: key2}}
		_, newAccounts, err := chainwriter.CCIPCommitAccountTransform(ctx, nil, args, accounts, offrampAddress.String())
		require.NoError(t, err)
		require.Len(t, newAccounts, 2)
	})
	t.Run("CCIPCommit ArgsTransform removes last account if token and gas prices do not exist", func(t *testing.T) {
		args := struct {
			Info ccipocr3.CommitReportInfo
		}{
			Info: ccipocr3.CommitReportInfo{},
		}
		accounts := []*solana.AccountMeta{{PublicKey: key1}, {PublicKey: key2}}
		_, newAccounts, err := chainwriter.CCIPCommitAccountTransform(ctx, nil, args, accounts, offrampAddress.String())
		require.NoError(t, err)
		require.Len(t, newAccounts, 1)
	})
}
