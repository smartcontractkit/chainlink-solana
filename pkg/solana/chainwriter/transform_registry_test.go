package chainwriter_test

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/smartcontractkit/chainlink-ccip/pkg/types/ccipocr3"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainwriter"
	txmutils "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/utils"
)

type ReportPreTransform struct {
	ReportContext [2][32]byte
	Report        []byte
	Info          ccipocr3.ExecuteReportInfo
}

func Test_CCIPExecuteArgsTransform(t *testing.T) {
	ctx := tests.Context(t)

	destTokenAddr := chainwriter.GetRandomPubKey(t)
	poolKeys := []solana.PublicKey{destTokenAddr}
	poolKeys = append(poolKeys, chainwriter.CreateTestPubKeys(t, 1)...)

	args := chainwriter.SVMExecCallArgs{
		Info: ccipocr3.ExecuteReportInfo{
			AbstractReports: []ccipocr3.ExecutePluginReportSingleChain{{
				Messages: []ccipocr3.Message{{
					TokenAmounts: []ccipocr3.RampTokenAmount{{
						DestTokenAddress: ccipocr3.UnknownAddress(destTokenAddr.Bytes()),
					}},
				}},
			}},
		},
		ExtraData: chainwriter.ExtraDataDecoded{
			ExtraArgsDecoded: map[string]any{
				"computeUnits": uint32(500),
			},
			DestExecDataDecoded: []map[string]any{
				{"destGasAmount": uint32(500)},
			},
		},
	}

	accounts := []*solana.AccountMeta{{PublicKey: poolKeys[0]}, {PublicKey: poolKeys[1]}}

	t.Run("CCIPExecute ArgsTransform includes token indexes and computeUnits", func(t *testing.T) {
		tableMap := make(map[string]map[string][]*solana.AccountMeta)
		tableMap["PoolLookupTable"] = make(map[string][]*solana.AccountMeta)
		lookupTablePubkey := chainwriter.GetRandomPubKey(t)

		poolKeysMeta := make([]*solana.AccountMeta, 0, 2)
		for _, poolKey := range poolKeys {
			poolKeysMeta = append(poolKeysMeta, &solana.AccountMeta{PublicKey: poolKey})
		}
		tableMap["PoolLookupTable"][lookupTablePubkey.String()] = poolKeysMeta

		transformedArgs, newAccounts, options, err := chainwriter.CCIPExecuteTransform(ctx, args, accounts, tableMap)
		require.NoError(t, err)

		verifyTxOpts(t, options, true)

		// Accounts should be unchanged
		require.Len(t, newAccounts, 2)
		typedArgs, ok := transformedArgs.(chainwriter.SVMExecCallArgs)
		require.True(t, ok)
		require.NotNil(t, typedArgs.TokenIndexes)
		require.Len(t, typedArgs.TokenIndexes, 1)
	})

	t.Run("CCIPExecute ArgsTransform includes empty token indexes if lookup table not found", func(t *testing.T) {
		transformedArgs, newAccounts, options, err := chainwriter.CCIPExecuteTransform(ctx, args, accounts, nil)
		require.NoError(t, err)
		verifyTxOpts(t, options, true)

		// Accounts should be unchanged
		require.Len(t, newAccounts, 2)
		typedArgs, ok := transformedArgs.(chainwriter.SVMExecCallArgs)
		require.True(t, ok)
		require.NotNil(t, typedArgs.TokenIndexes)
		require.Len(t, typedArgs.TokenIndexes, 0)
	})

	t.Run("CCIPExecute ArgsTransform does not get args that conform to ReportPreTransform", func(t *testing.T) {
		args := struct {
			ReportContext [2][32]uint8
			Info          ccipocr3.ExecuteReportInfo
			ExtraData     chainwriter.ExtraDataDecoded
		}{
			ReportContext: [2][32]uint8{},
			Info: ccipocr3.ExecuteReportInfo{
				AbstractReports: []ccipocr3.ExecutePluginReportSingleChain{{
					Messages: []ccipocr3.Message{{}},
				}},
			},
			ExtraData: chainwriter.ExtraDataDecoded{
				ExtraArgsDecoded: map[string]any{
					"computeUnits": uint32(500),
				},
				DestExecDataDecoded: []map[string]any{
					{"destGasAmount": uint32(500)},
				},
			},
		}
		transformedArgs, newAccounts, options, err := chainwriter.CCIPExecuteTransform(ctx, args, accounts, nil)
		require.NoError(t, err)

		verifyTxOpts(t, options, true)
		_, ok := transformedArgs.(chainwriter.SVMExecCallArgs)
		require.True(t, ok)
		require.Len(t, newAccounts, 2)
	})

	t.Run("CCIPExecute ArgsTransform fails with empty Info", func(t *testing.T) {
		args := struct {
			ReportContext [2][32]uint8
			Report        []uint8
			Info          ccipocr3.ExecuteReportInfo
		}{
			ReportContext: [2][32]uint8{},
			Report:        []uint8{},
			Info:          ccipocr3.ExecuteReportInfo{},
		}
		_, _, _, err := chainwriter.CCIPExecuteTransform(ctx, args, accounts, nil)
		require.Contains(t, err.Error(), "computeUnits not found in ExtraData")
	})
}

func Test_CCIPCommitAccountTransform(t *testing.T) {
	ctx := tests.Context(t)
	key1 := chainwriter.GetRandomPubKey(t)
	key2 := chainwriter.GetRandomPubKey(t)
	t.Run("CCIPCommit ArgsTransform does not affect accounts if token prices exist", func(t *testing.T) {
		args := struct {
			Info ccipocr3.CommitReportInfo
		}{
			Info: ccipocr3.CommitReportInfo{
				PriceUpdates: ccipocr3.PriceUpdates{
					TokenPriceUpdates: []ccipocr3.TokenPrice{
						{TokenID: ccipocr3.UnknownEncodedAddress(key1.String())},
					},
				},
			},
		}
		accounts := []*solana.AccountMeta{{PublicKey: key1}, {PublicKey: key2}}
		_, newAccounts, options, err := chainwriter.CCIPCommitTransform(ctx, args, accounts, nil)
		verifyTxOpts(t, options, false)
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
		_, newAccounts, options, err := chainwriter.CCIPCommitTransform(ctx, args, accounts, nil)
		verifyTxOpts(t, options, false)

		require.NoError(t, err)
		require.Len(t, newAccounts, 1)
	})
}

func verifyTxOpts(t *testing.T, options []txmutils.SetTxConfig, exec bool) {
	expectedLen := 1
	if exec {
		expectedLen = 2
	}
	require.Len(t, options, expectedLen)

	txConfig := &txmutils.TxConfig{}
	options[0](txConfig)
	require.Equal(t, !exec, txConfig.EstimateComputeUnitLimit)

	if exec {
		options[1](txConfig)
		require.Equal(t, chainwriter.StaticCuOverhead+1000, txConfig.ComputeUnitLimit)
	}
}
