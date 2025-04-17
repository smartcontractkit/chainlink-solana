package chainwriter_test

import (
	"context"
	"encoding/binary"
	"math"
	"testing"

	ag_binary "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	ccipsolana "github.com/smartcontractkit/chainlink-ccip/chains/solana"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/ccip_common"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip/pkg/types/ccipocr3"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainwriter"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	clientmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/mocks"
	txmutils "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/utils"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/utils"
)

type ReportPreTransform struct {
	ReportContext [2][32]byte
	Report        []byte
	Info          ccipocr3.ExecuteReportInfo
}

func Test_CCIPExecuteArgsTransform(t *testing.T) {
	ctx := t.Context()

	// mock client
	rw := clientmocks.NewReaderWriter(t)
	mc := *client.NewMultiClient(func(context.Context) (client.ReaderWriter, error) {
		return rw, nil
	})

	logicReceiver := utils.GetRandomPubKey(t)
	tokenReceiver := utils.GetRandomPubKey(t)
	offrampAddress := utils.GetRandomPubKey(t)
	destTokenAddr1 := utils.GetRandomPubKey(t)
	destTokenAddr2 := utils.GetRandomPubKey(t)
	poolKeys := chainwriter.CreateTestPubKeys(t, 7)
	tokenAdminRegistryAddr := poolKeys[1]
	poolProgram := poolKeys[2]
	tokenProgram := poolKeys[6]
	sourceChainSelector := ccipocr3.ChainSelector(1)
	feeQuoterAddr := utils.GetRandomPubKey(t)

	sourceChainSelBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(sourceChainSelBytes, uint64(sourceChainSelector))

	offrampPoolsSigner, _, err := solana.FindProgramAddress([][]byte{[]byte("external_token_pools_signer"), poolProgram.Bytes()}, offrampAddress)
	require.NoError(t, err)

	userTokenAccount1, _, err := solana.FindProgramAddress([][]byte{tokenReceiver.Bytes(), tokenProgram.Bytes(), destTokenAddr1.Bytes()}, solana.SPLAssociatedTokenAccountProgramID)
	require.NoError(t, err)
	perChainTokenConfig1, _, err := solana.FindProgramAddress([][]byte{[]byte("per_chain_per_token_config"), sourceChainSelBytes, destTokenAddr1.Bytes()}, feeQuoterAddr)
	require.NoError(t, err)
	poolChainConfig1, _, err := solana.FindProgramAddress([][]byte{[]byte("ccip_tokenpool_chainconfig"), sourceChainSelBytes, destTokenAddr1.Bytes()}, poolProgram)
	require.NoError(t, err)

	userTokenAccount2, _, err := solana.FindProgramAddress([][]byte{tokenReceiver.Bytes(), tokenProgram.Bytes(), destTokenAddr2.Bytes()}, solana.SPLAssociatedTokenAccountProgramID)
	require.NoError(t, err)
	perChainTokenConfig2, _, err := solana.FindProgramAddress([][]byte{[]byte("per_chain_per_token_config"), sourceChainSelBytes, destTokenAddr2.Bytes()}, feeQuoterAddr)
	require.NoError(t, err)
	poolChainConfig2, _, err := solana.FindProgramAddress([][]byte{[]byte("ccip_tokenpool_chainconfig"), sourceChainSelBytes, destTokenAddr2.Bytes()}, poolProgram)
	require.NoError(t, err)

	tableMap := make(map[string]map[string][]*solana.AccountMeta)
	tableMap["PoolLookupTable"] = make(map[string][]*solana.AccountMeta)
	lookupTablePubkey := utils.GetRandomPubKey(t)

	poolKeysMeta := make([]*solana.AccountMeta, 0, len(poolKeys))
	for _, poolKey := range poolKeys {
		poolKeysMeta = append(poolKeysMeta, &solana.AccountMeta{PublicKey: poolKey})
	}
	tableMap["PoolLookupTable"][lookupTablePubkey.String()] = poolKeysMeta

	externalExecutionSigner, _, err := solana.FindProgramAddress([][]byte{[]byte("external_execution_config"), logicReceiver.Bytes()}, offrampAddress)
	require.NoError(t, err)
	userMessagingAccounts := chainwriter.CreateTestPubKeys(t, 3) // arbitrary number of user accounts

	args := ccipsolana.SVMExecCallArgs{
		Info: ccipocr3.ExecuteReportInfo{
			AbstractReports: []ccipocr3.ExecutePluginReportSingleChain{{
				Messages: []ccipocr3.Message{{
					Receiver: logicReceiver.Bytes(),
					Header:   ccipocr3.RampMessageHeader{SourceChainSelector: sourceChainSelector},
					TokenAmounts: []ccipocr3.RampTokenAmount{
						{
							DestTokenAddress: destTokenAddr1.Bytes(),
						},
						{
							DestTokenAddress: destTokenAddr2.Bytes(),
						},
					}},
				},
			}},
		},
		ExtraData: ccipsolana.ExtraDataDecoded{
			ExtraArgsDecoded: map[string]any{
				"computeUnits":            uint32(500),
				"accounts":                userMessagingAccounts,
				"accountIsWritableBitmap": uint64(1),
				"tokenReceiver":           tokenReceiver,
			},
			DestExecDataDecoded: []map[string]any{
				{"destGasAmount": uint32(500)},
			},
		},
	}

	requiredMessagingAccountsLen := 2
	nonPoolTTAccountsLen := 4

	t.Run("CCIPExecute ArgsTransform includes token indexes and sets the corresponding IsWritable flag", func(t *testing.T) {
		mockFetchFeeQuoterAddress(t, rw, feeQuoterAddr, offrampAddress)
		// second address in pool lookup table is expected to be the token admin registry address needed to fetch the WritableIndexes
		mockWritableIndexes(t, rw, tokenAdminRegistryAddr)
		mandatoryAccounts := chainwriter.CreateTestPubKeys(t, chainwriter.MandatoryExecuteAccounts)
		// Accounts list contains other accounts before token addresses
		accounts := make([]*solana.AccountMeta, 0, len(mandatoryAccounts)+len(userMessagingAccounts))
		for _, acc := range mandatoryAccounts {
			accounts = append(accounts, &solana.AccountMeta{PublicKey: acc})
		}

		transformedArgs, newAccounts, options, err := chainwriter.CCIPExecuteArgsTransform(ctx, mc, args, accounts, tableMap, offrampAddress.String())
		require.NoError(t, err)
		verifyTxOpts(t, options, true)

		typedArgs, ok := transformedArgs.(ccipsolana.SVMExecCallArgs)
		require.True(t, ok)
		require.NotNil(t, typedArgs.TokenIndexes)
		require.Len(t, typedArgs.TokenIndexes, 2)
		// mandatory accounts + required messaging accounts + arbitrary user messaging accounts + nonPoolTTAccountsLen for TokenAmounts[0]+ pool keys + nonPoolTTAccountsLen for TokenAmounts[1] + pool keys
		require.Len(t, newAccounts, len(mandatoryAccounts)+requiredMessagingAccountsLen+len(userMessagingAccounts)+nonPoolTTAccountsLen+len(poolKeys)+nonPoolTTAccountsLen+len(poolKeys))
		// Token indexes are relative to the remaining accounts which exclude the mandatory accounts at the beginning
		remainingAccounts := newAccounts[chainwriter.MandatoryExecuteAccounts:]
		require.Len(t, remainingAccounts, requiredMessagingAccountsLen+len(userMessagingAccounts)+nonPoolTTAccountsLen+len(poolKeys)+nonPoolTTAccountsLen+len(poolKeys))
		// logic receiver is the first account in remaining accounts
		require.Equal(t, logicReceiver, remainingAccounts[0].PublicKey)
		// external execution signer is the second account in remaining accounts
		require.Equal(t, externalExecutionSigner, remainingAccounts[1].PublicKey)
		for i, tokenIdx := range typedArgs.TokenIndexes {
			startIdx := tokenIdx
			var endIdx uint8
			if i < len(typedArgs.TokenIndexes)-1 {
				endIdx = typedArgs.TokenIndexes[i+1]
			} else {
				endIdx = uint8(len(remainingAccounts))
			}
			tokenAccounts := remainingAccounts[startIdx:endIdx]
			require.Len(t, tokenAccounts, nonPoolTTAccountsLen+len(poolKeys)) // offramp pools signer + user token account + per chain token config + pool chain config + 7 pool keys
			if i == 0 {
				require.Equal(t, &solana.AccountMeta{PublicKey: offrampPoolsSigner, IsWritable: false, IsSigner: false}, tokenAccounts[0])
				require.Equal(t, &solana.AccountMeta{PublicKey: userTokenAccount1, IsWritable: true}, tokenAccounts[1])
				require.Equal(t, &solana.AccountMeta{PublicKey: perChainTokenConfig1}, tokenAccounts[2])
				require.Equal(t, &solana.AccountMeta{PublicKey: poolChainConfig1, IsWritable: true}, tokenAccounts[3])
			} else {
				require.Equal(t, &solana.AccountMeta{PublicKey: offrampPoolsSigner, IsWritable: false, IsSigner: false}, tokenAccounts[0])
				require.Equal(t, &solana.AccountMeta{PublicKey: userTokenAccount2, IsWritable: true}, tokenAccounts[1])
				require.Equal(t, &solana.AccountMeta{PublicKey: perChainTokenConfig2}, tokenAccounts[2])
				require.Equal(t, &solana.AccountMeta{PublicKey: poolChainConfig2, IsWritable: true}, tokenAccounts[3])
			}
			// Pool lookup accounts should have the proper write flags set for token accounts
			for j := 3; j < len(tokenAccounts); j++ {
				require.True(t, tokenAccounts[j].IsWritable)
			}
		}
		// Token addresses shifted by logic receiver + external execution signer + user messaging accounts since token index is relative to remaining accounts
		require.Equal(t, uint8(requiredMessagingAccountsLen+len(userMessagingAccounts)), typedArgs.TokenIndexes[0])
		// Token addresses shifted by logic receiver + external execution signer + user messaging accounts + the previous token accounts
		require.Equal(t, uint8(requiredMessagingAccountsLen+len(userMessagingAccounts)+nonPoolTTAccountsLen+len(poolKeys)), typedArgs.TokenIndexes[1])
	})

	t.Run("CCIPExecute ArgsTransform ignores user messaging accounts if logic receiver is empty", func(t *testing.T) {
		mockFetchFeeQuoterAddress(t, rw, feeQuoterAddr, offrampAddress)
		// second address in pool lookup table is expected to be the token admin registry address needed to fetch the WritableIndexes
		mockWritableIndexes(t, rw, tokenAdminRegistryAddr)
		missingLogicReceiverArgs := ccipsolana.SVMExecCallArgs{
			Info: ccipocr3.ExecuteReportInfo{
				AbstractReports: []ccipocr3.ExecutePluginReportSingleChain{{
					Messages: []ccipocr3.Message{{
						Header: ccipocr3.RampMessageHeader{SourceChainSelector: sourceChainSelector},
						TokenAmounts: []ccipocr3.RampTokenAmount{
							{
								DestTokenAddress: destTokenAddr1.Bytes(),
							},
						}},
					},
				}},
			},
			ExtraData: ccipsolana.ExtraDataDecoded{
				ExtraArgsDecoded: map[string]any{
					"computeUnits":            uint32(500),
					"accounts":                userMessagingAccounts,
					"accountIsWritableBitmap": uint64(1),
					"tokenReceiver":           tokenReceiver,
				},
				DestExecDataDecoded: []map[string]any{
					{"destGasAmount": uint32(500)},
				},
			},
		}

		mandatoryAccounts := chainwriter.CreateTestPubKeys(t, chainwriter.MandatoryExecuteAccounts)
		// Accounts list contains other accounts before token addresses
		accounts := make([]*solana.AccountMeta, 0, len(mandatoryAccounts))
		for _, acc := range mandatoryAccounts {
			accounts = append(accounts, &solana.AccountMeta{PublicKey: acc})
		}

		transformedArgs, newAccounts, options, err := chainwriter.CCIPExecuteArgsTransform(ctx, mc, missingLogicReceiverArgs, accounts, tableMap, offrampAddress.String())
		require.NoError(t, err)
		verifyTxOpts(t, options, true)

		typedArgs, ok := transformedArgs.(ccipsolana.SVMExecCallArgs)
		require.True(t, ok)
		require.NotNil(t, typedArgs.TokenIndexes)
		require.Len(t, typedArgs.TokenIndexes, 1)
		require.Equal(t, uint8(0), typedArgs.TokenIndexes[0]) // Token index is 0 because no user messaging accounts precede token transfer accounts
		// mandatory accounts + 4 token accounts for TokenAmounts[0] + 7 pool keys
		require.Len(t, newAccounts, len(mandatoryAccounts)+nonPoolTTAccountsLen+len(poolKeys))
	})

	t.Run("CCIPExecute ArgsTransform ignores token transfer related errors if accounts not required", func(t *testing.T) {
		mandatoryAccounts := chainwriter.CreateTestPubKeys(t, chainwriter.MandatoryExecuteAccounts)
		// Accounts list contains other accounts before token addresses
		accounts := make([]*solana.AccountMeta, 0, len(mandatoryAccounts))
		for _, acc := range mandatoryAccounts {
			accounts = append(accounts, &solana.AccountMeta{PublicKey: acc})
		}
		messagingOnlyArgs := ccipsolana.SVMExecCallArgs{
			Info: ccipocr3.ExecuteReportInfo{
				AbstractReports: []ccipocr3.ExecutePluginReportSingleChain{{
					Messages: []ccipocr3.Message{
						{
							Receiver: logicReceiver.Bytes(),
							Header:   ccipocr3.RampMessageHeader{SourceChainSelector: sourceChainSelector},
						},
					},
				}},
			},
			ExtraData: ccipsolana.ExtraDataDecoded{
				ExtraArgsDecoded: map[string]any{
					"computeUnits":            uint32(500),
					"accounts":                userMessagingAccounts,
					"accountIsWritableBitmap": uint64(1),
				},
				DestExecDataDecoded: []map[string]any{
					{"destGasAmount": uint32(500)},
				},
			},
		}
		t.Run("CCIPExecute ArgsTransform ignores missing pool lookup table error", func(t *testing.T) {
			transformedArgs, newAccounts, options, err := chainwriter.CCIPExecuteArgsTransform(ctx, mc, messagingOnlyArgs, accounts, nil, offrampAddress.String())
			require.NoError(t, err)
			verifyTxOpts(t, options, true)

			typedArgs, ok := transformedArgs.(ccipsolana.SVMExecCallArgs)
			require.True(t, ok)
			require.NotNil(t, typedArgs.TokenIndexes)
			require.Len(t, typedArgs.TokenIndexes, 0)
			// mandatory accounts + 2 requiredMessagingAccountsLen + 3 for user messaging accounts
			require.Len(t, newAccounts, len(mandatoryAccounts)+requiredMessagingAccountsLen+len(userMessagingAccounts))
		})
		t.Run("CCIPExecute ArgsTransform ignores missing token receiver error", func(t *testing.T) {
			transformedArgs, newAccounts, options, err := chainwriter.CCIPExecuteArgsTransform(ctx, mc, messagingOnlyArgs, accounts, tableMap, offrampAddress.String())
			require.NoError(t, err)
			verifyTxOpts(t, options, true)

			typedArgs, ok := transformedArgs.(ccipsolana.SVMExecCallArgs)
			require.True(t, ok)
			require.NotNil(t, typedArgs.TokenIndexes)
			require.Len(t, typedArgs.TokenIndexes, 0)
			// mandatory accounts + 2 requiredMessagingAccountsLen + 3 for user messaging accounts
			require.Len(t, newAccounts, len(mandatoryAccounts)+requiredMessagingAccountsLen+len(userMessagingAccounts))
		})
	})

	t.Run("CCIPExecute ArgsTransform failed if token transfer accounts are required and the token receiver is empty", func(t *testing.T) {
		mockFetchFeeQuoterAddress(t, rw, feeQuoterAddr, offrampAddress)
		// second address in pool lookup table is expected to be the token admin registry address needed to fetch the WritableIndexes
		mockWritableIndexes(t, rw, tokenAdminRegistryAddr)
		missingTokenReceiverArgs := ccipsolana.SVMExecCallArgs{
			Info: ccipocr3.ExecuteReportInfo{
				AbstractReports: []ccipocr3.ExecutePluginReportSingleChain{{
					Messages: []ccipocr3.Message{{
						Receiver: logicReceiver.Bytes(),
						Header:   ccipocr3.RampMessageHeader{SourceChainSelector: sourceChainSelector},
						TokenAmounts: []ccipocr3.RampTokenAmount{
							{
								DestTokenAddress: destTokenAddr1.Bytes(),
							},
						}},
					},
				}},
			},
			ExtraData: ccipsolana.ExtraDataDecoded{
				ExtraArgsDecoded: map[string]any{
					"computeUnits":            uint32(500),
					"accounts":                userMessagingAccounts,
					"accountIsWritableBitmap": uint64(1),
				},
				DestExecDataDecoded: []map[string]any{
					{"destGasAmount": uint32(500)},
				},
			},
		}

		mandatoryAccounts := chainwriter.CreateTestPubKeys(t, chainwriter.MandatoryExecuteAccounts)
		// Accounts list contains other accounts before token addresses
		accounts := make([]*solana.AccountMeta, 0, len(mandatoryAccounts))
		for _, acc := range mandatoryAccounts {
			accounts = append(accounts, &solana.AccountMeta{PublicKey: acc})
		}

		_, _, _, err := chainwriter.CCIPExecuteArgsTransform(ctx, mc, missingTokenReceiverArgs, accounts, tableMap, offrampAddress.String())
		require.Error(t, err)
	})

	t.Run("CCIPExecute ArgsTransform does not include any remaining accounts if both logic and token receivers are missing", func(t *testing.T) {
		missingBothReceiverArgs := ccipsolana.SVMExecCallArgs{
			Info: ccipocr3.ExecuteReportInfo{
				AbstractReports: []ccipocr3.ExecutePluginReportSingleChain{{
					Messages: []ccipocr3.Message{
						{
							Header: ccipocr3.RampMessageHeader{SourceChainSelector: sourceChainSelector},
						},
					},
				}},
			},
			ExtraData: ccipsolana.ExtraDataDecoded{
				ExtraArgsDecoded: map[string]any{
					"computeUnits":            uint32(500),
					"accounts":                userMessagingAccounts,
					"accountIsWritableBitmap": uint64(1),
				},
				DestExecDataDecoded: []map[string]any{
					{"destGasAmount": uint32(500)},
				},
			},
		}
		mandatoryAccounts := chainwriter.CreateTestPubKeys(t, chainwriter.MandatoryExecuteAccounts)
		// Accounts list contains other accounts before token addresses
		accounts := make([]*solana.AccountMeta, 0, len(mandatoryAccounts))
		for _, acc := range mandatoryAccounts {
			accounts = append(accounts, &solana.AccountMeta{PublicKey: acc})
		}

		transformedArgs, newAccounts, options, err := chainwriter.CCIPExecuteArgsTransform(ctx, mc, missingBothReceiverArgs, accounts, tableMap, offrampAddress.String())
		require.NoError(t, err)
		verifyTxOpts(t, options, true)
		typedArgs, ok := transformedArgs.(ccipsolana.SVMExecCallArgs)
		require.True(t, ok)
		require.NotNil(t, typedArgs.TokenIndexes)
		require.Len(t, typedArgs.TokenIndexes, 0)
		// no extra accounts are added so new accounts should equal mandatory accounts
		require.Len(t, newAccounts, len(mandatoryAccounts))
	})

	t.Run("CCIPExecute ArgsTransform fails if token transfer accounts is required and lookup table not found", func(t *testing.T) {
		mandatoryAccounts := chainwriter.CreateTestPubKeys(t, chainwriter.MandatoryExecuteAccounts)
		// Accounts list contains other accounts before token addresses
		accounts := make([]*solana.AccountMeta, 0, len(mandatoryAccounts))
		for _, acc := range mandatoryAccounts {
			accounts = append(accounts, &solana.AccountMeta{PublicKey: acc})
		}
		_, _, _, err := chainwriter.CCIPExecuteArgsTransform(ctx, mc, args, accounts, nil, offrampAddress.String())
		require.ErrorContains(t, err, "failed to find PoolLookupTable in table map")
	})

	t.Run("CCIPExecute ArgsTransform does not get args that conform to ReportPreTransform", func(t *testing.T) {
		mandatoryAccounts := chainwriter.CreateTestPubKeys(t, chainwriter.MandatoryExecuteAccounts)
		// Accounts list contains other accounts before token addresses
		accounts := make([]*solana.AccountMeta, 0, len(mandatoryAccounts))
		for _, acc := range mandatoryAccounts {
			accounts = append(accounts, &solana.AccountMeta{PublicKey: acc})
		}
		args := struct {
			ReportContext [2][32]uint8
			Info          ccipocr3.ExecuteReportInfo
			ExtraData     ccipsolana.ExtraDataDecoded
		}{
			ReportContext: [2][32]uint8{},
			Info: ccipocr3.ExecuteReportInfo{
				AbstractReports: []ccipocr3.ExecutePluginReportSingleChain{{
					Messages: []ccipocr3.Message{{}},
				}},
			},
			ExtraData: ccipsolana.ExtraDataDecoded{
				ExtraArgsDecoded: map[string]any{
					"computeUnits": uint32(500),
				},
				DestExecDataDecoded: []map[string]any{
					{"destGasAmount": uint32(500)},
				},
			},
		}
		transformedArgs, newAccounts, options, err := chainwriter.CCIPExecuteArgsTransform(ctx, mc, args, accounts, nil, offrampAddress.String())
		require.NoError(t, err)

		verifyTxOpts(t, options, true)
		_, ok := transformedArgs.(ccipsolana.SVMExecCallArgs)
		require.True(t, ok)
		require.Len(t, newAccounts, len(accounts))
	})

	t.Run("CCIPExecute ArgsTransform fails with empty Info", func(t *testing.T) {
		accounts := []*solana.AccountMeta{{PublicKey: utils.GetRandomPubKey(t)}}

		args := struct {
			ReportContext [2][32]uint8
			Report        []uint8
			Info          ccipocr3.ExecuteReportInfo
		}{
			ReportContext: [2][32]uint8{},
			Report:        []uint8{},
			Info:          ccipocr3.ExecuteReportInfo{},
		}
		_, _, _, err := chainwriter.CCIPExecuteArgsTransform(ctx, mc, args, accounts, nil, offrampAddress.String())
		require.Contains(t, err.Error(), "computeUnits not found in ExtraData")
	})
}

func Test_CCIPCommitAccountTransform(t *testing.T) {
	ctx := t.Context()

	// mock client
	rw := clientmocks.NewReaderWriter(t)
	mc := *client.NewMultiClient(func(context.Context) (client.ReaderWriter, error) {
		return rw, nil
	})

	key1 := utils.GetRandomPubKey(t)
	key2 := utils.GetRandomPubKey(t)
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
		_, newAccounts, options, err := chainwriter.CCIPCommitAccountTransform(ctx, mc, args, accounts, nil, "")
		verifyTxOpts(t, options, false)
		require.NoError(t, err)
		require.Len(t, newAccounts, len(accounts))
	})
	t.Run("CCIPCommit ArgsTransform removes last account if token and gas prices do not exist", func(t *testing.T) {
		args := struct {
			Info ccipocr3.CommitReportInfo
		}{
			Info: ccipocr3.CommitReportInfo{},
		}
		accounts := []*solana.AccountMeta{{PublicKey: key1}, {PublicKey: key2}}
		_, newAccounts, _, err := chainwriter.CCIPCommitAccountTransform(ctx, mc, args, accounts, nil, "")
		require.NoError(t, err)
		require.Len(t, newAccounts, 1)
	})
}

func verifyTxOpts(t *testing.T, options []txmutils.SetTxConfig, exec bool) {
	// TODO: re-enable when the SanitizeFailure issue is fixed
	// expectedLen := 1
	// if exec {
	// 	expectedLen = 2
	// }
	// require.Len(t, options, expectedLen)

	// txConfig := &txmutils.TxConfig{}
	// options[0](txConfig)
	// require.Equal(t, !exec, txConfig.EstimateComputeUnitLimit)

	// if exec {
	// 	options[1](txConfig)
	// 	require.Equal(t, chainwriter.StaticCuOverhead+1000, txConfig.ComputeUnitLimit)
	// }
}

func mockWritableIndexes(t *testing.T, rw *clientmocks.ReaderWriter, tokenAdminRegistryAddr solana.PublicKey) {
	lookupTablePubkey := utils.GetRandomPubKey(t)
	tokenAdminRegistry := ccip_common.TokenAdminRegistry{
		Version:              1,
		Administrator:        utils.GetRandomPubKey(t),
		PendingAdministrator: utils.GetRandomPubKey(t),
		LookupTable:          lookupTablePubkey,
		// set all accounts as writable
		WritableIndexes: [2]ag_binary.Uint128{{Endianness: ag_binary.LE, Lo: math.MaxUint64, Hi: math.MaxUint64}},
	}
	registryBytes := mustBorshEncodeStruct(t, tokenAdminRegistry)
	rw.On("GetAccountInfoWithOpts", mock.Anything, tokenAdminRegistryAddr, mock.Anything).Return(&rpc.GetAccountInfoResult{
		RPCContext: rpc.RPCContext{},
		Value:      &rpc.Account{Data: rpc.DataBytesOrJSONFromBytes(registryBytes)},
	}, nil).Once()
}

func mockFetchFeeQuoterAddress(t *testing.T, rw *clientmocks.ReaderWriter, feeQuoterAddr, offrampAddr solana.PublicKey) {
	pda, _, err := solana.FindProgramAddress([][]byte{[]byte("reference_addresses")}, offrampAddr)
	require.NoError(t, err)
	referenceAddresses := ccip_offramp.ReferenceAddresses{
		Version:            1,
		Router:             solana.PublicKey{},
		FeeQuoter:          feeQuoterAddr,
		OfframpLookupTable: solana.PublicKey{},
	}
	referenceAddressesBytes := mustBorshEncodeStruct(t, referenceAddresses)
	rw.On("GetAccountInfoWithOpts", mock.Anything, pda, mock.Anything).Return(&rpc.GetAccountInfoResult{
		RPCContext: rpc.RPCContext{},
		Value:      &rpc.Account{Data: rpc.DataBytesOrJSONFromBytes(referenceAddressesBytes)},
	}, nil).Once()
}
