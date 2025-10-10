package chainwriter_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"math/big"
	"strconv"
	"testing"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/types/ccipocr3"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	ccipsolana "github.com/smartcontractkit/chainlink-ccip/chains/solana"
	ccip_offramp_v0_1_1 "github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/v0_1_1/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/state"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainwriter"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	clientmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/mocks"
	txmutils "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/utils"
)

type ReportPreTransform struct {
	ReportContext [2][32]byte
	Report        []byte
	Info          ccipocr3.ExecuteReportInfo
}

type tokenTransferAccounts struct {
	offrampPoolSigner   solana.PublicKey
	userTokenAccount    solana.PublicKey
	perChainTokenConfig solana.PublicKey
	poolChainConfig     solana.PublicKey
	poolKeys            []solana.PublicKey
	mint                solana.PublicKey
}

func Test_CCIPExecuteArgsTransform(t *testing.T) {
	ctx := t.Context()
	lggr := logger.Test(t)

	// mock client
	rw := clientmocks.NewReaderWriter(t)
	mc := *client.NewMultiClient(func(context.Context) (client.ReaderWriter, error) {
		return rw, nil
	})

	fromAddress := GetRandomPubKey(t)
	logicReceiver := GetRandomPubKey(t)
	tokenReceiver := GetRandomPubKey(t)
	offrampAddress := GetRandomPubKey(t)
	destTokenAddr1 := GetRandomPubKey(t)
	destTokenAddr2 := GetRandomPubKey(t)
	sourcePoolAddr1 := GetRandomPubKey(t)
	sourcePoolAddr2 := GetRandomPubKey(t)
	poolKeys1 := CreateTestPubKeys(t, 7)
	poolProgram1 := poolKeys1[2]
	tokenProgram1 := poolKeys1[6]

	poolKeys2 := CreateTestPubKeys(t, 7)
	poolProgram2 := poolKeys2[2]
	tokenProgram2 := poolKeys2[6]
	sourceChainSelector := ccipocr3.ChainSelector(1)
	feeQuoterAddr := GetRandomPubKey(t)

	sourceChainSelBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(sourceChainSelBytes, uint64(sourceChainSelector))

	offrampPoolsSigner1, _, pdaErr := state.FindExternalTokenPoolsSignerPDA(poolProgram1, offrampAddress)
	require.NoError(t, pdaErr)
	userTokenAccount1, _, pdaErr := solana.FindProgramAddress([][]byte{tokenReceiver.Bytes(), tokenProgram1.Bytes(), destTokenAddr1.Bytes()}, solana.SPLAssociatedTokenAccountProgramID)
	require.NoError(t, pdaErr)
	perChainTokenConfig1, _, pdaErr := solana.FindProgramAddress([][]byte{[]byte("per_chain_per_token_config"), sourceChainSelBytes, destTokenAddr1.Bytes()}, feeQuoterAddr)
	require.NoError(t, pdaErr)
	poolChainConfig1, _, pdaErr := solana.FindProgramAddress([][]byte{[]byte("ccip_tokenpool_chainconfig"), sourceChainSelBytes, destTokenAddr1.Bytes()}, poolProgram1)
	require.NoError(t, pdaErr)

	offrampPoolsSigner2, _, pdaErr := state.FindExternalTokenPoolsSignerPDA(poolProgram2, offrampAddress)
	require.NoError(t, pdaErr)
	userTokenAccount2, _, pdaErr := solana.FindProgramAddress([][]byte{tokenReceiver.Bytes(), tokenProgram2.Bytes(), destTokenAddr2.Bytes()}, solana.SPLAssociatedTokenAccountProgramID)
	require.NoError(t, pdaErr)
	perChainTokenConfig2, _, pdaErr := solana.FindProgramAddress([][]byte{[]byte("per_chain_per_token_config"), sourceChainSelBytes, destTokenAddr2.Bytes()}, feeQuoterAddr)
	require.NoError(t, pdaErr)
	poolChainConfig2, _, pdaErr := solana.FindProgramAddress([][]byte{[]byte("ccip_tokenpool_chainconfig"), sourceChainSelBytes, destTokenAddr2.Bytes()}, poolProgram2)
	require.NoError(t, pdaErr)

	userMessagingAccounts := CreateTestPubKeys(t, 3) // arbitrary number of user accounts

	staticCUOverhead := uint32(150_000)
	userCU := uint32(500)
	destGasAmount := uint32(500)

	requiredMessagingAccountsLen := 2
	nonPoolTTAccountsLen := 4

	ttAccount1 := tokenTransferAccounts{
		offrampPoolSigner:   offrampPoolsSigner1,
		userTokenAccount:    userTokenAccount1,
		perChainTokenConfig: perChainTokenConfig1,
		poolChainConfig:     poolChainConfig1,
		poolKeys:            poolKeys1,
		mint:                destTokenAddr1,
	}
	ttAccount2 := tokenTransferAccounts{
		offrampPoolSigner:   offrampPoolsSigner2,
		userTokenAccount:    userTokenAccount2,
		perChainTokenConfig: perChainTokenConfig2,
		poolChainConfig:     poolChainConfig2,
		poolKeys:            poolKeys2,
		mint:                destTokenAddr2,
	}

	lookupTablePubkey1 := GetRandomPubKey(t)
	lookupTablePubkey2 := GetRandomPubKey(t)
	mockFetchLookupTableAddresses(t, rw, lookupTablePubkey1, poolKeys1)
	mockFetchLookupTableAddresses(t, rw, lookupTablePubkey2, poolKeys2)

	externalExecutionConfig, _, pdaErr := state.FindExternalExecutionConfigPDA(logicReceiver, offrampAddress)
	require.NoError(t, pdaErr)

	var merkleRoot ccipocr3.Bytes32

	args := ccipsolana.SVMExecCallArgs{
		Info: ccipocr3.ExecuteReportInfo{
			AbstractReports: []ccipocr3.ExecutePluginReportSingleChain{{
				Messages: []ccipocr3.Message{{
					Receiver: logicReceiver.Bytes(),
					Header:   ccipocr3.RampMessageHeader{SourceChainSelector: sourceChainSelector},
					TokenAmounts: []ccipocr3.RampTokenAmount{
						{
							DestTokenAddress:  destTokenAddr1.Bytes(),
							Amount:            ccipocr3.NewBigInt(big.NewInt(1)),
							SourcePoolAddress: sourcePoolAddr1.Bytes(),
						},
						{
							DestTokenAddress:  destTokenAddr2.Bytes(),
							Amount:            ccipocr3.NewBigInt(big.NewInt(2)),
							SourcePoolAddress: sourcePoolAddr2.Bytes(),
						},
					}},
				},
				OffchainTokenData: [][][]byte{{}},
			}},
			MerkleRoots: []ccipocr3.MerkleRootChain{{MerkleRoot: merkleRoot}},
		},
		ExtraData: ccipsolana.ExtraDataDecoded{
			ExtraArgsDecoded: map[string]any{
				"computeUnits":            userCU,
				"accounts":                userMessagingAccounts,
				"accountIsWritableBitmap": uint64(1),
				"tokenReceiver":           tokenReceiver,
			},
			DestExecDataDecoded: []map[string]any{
				{"destGasAmount": destGasAmount},
				{"destGasAmount": destGasAmount},
			},
		},
	}

	mandatoryExecuteAccountsLen := cap(ccip_offramp_v0_1_1.NewExecuteInstructionBuilder().AccountMetaSlice)

	t.Run("ArgsTransform includes token indexes and sets the corresponding IsWritable flag", func(t *testing.T) {
		// Mock the account derivation simulations
		mockExecuteAccountDerivation(t, rw, offrampAddress.String(), userMessagingAccounts, []tokenTransferAccounts{ttAccount1, ttAccount2}, logicReceiver, []solana.PublicKey{lookupTablePubkey1, lookupTablePubkey2})

		staticLookupTableMaps := make(map[solana.PublicKey]solana.PublicKeySlice)
		transformedArgs, newAccounts, lookupTableMap, options, err := chainwriter.CCIPExecuteArgsTransformV2(ctx, mc, lggr, args, nil, staticLookupTableMaps, nil, fromAddress, offrampAddress.String(), staticCUOverhead, []txmutils.SetTxConfig{}, "")
		require.NoError(t, err)
		require.Len(t, lookupTableMap, 2) // contains pool lookup table for the both token transfers
		verifyTxOpts(t, options, true, staticCUOverhead, userCU, destGasAmount*2)

		typedArgs, ok := transformedArgs.(ccipsolana.SVMExecCallArgs)
		require.True(t, ok)
		require.NotNil(t, typedArgs.TokenIndexes)
		require.Len(t, typedArgs.TokenIndexes, 2)
		// mandatory accounts + required messaging accounts + arbitrary user messaging accounts + nonPoolTTAccountsLen for TokenAmounts[0]+ pool keys + nonPoolTTAccountsLen for TokenAmounts[1] + pool keys
		require.Len(t, newAccounts, mandatoryExecuteAccountsLen+requiredMessagingAccountsLen+len(userMessagingAccounts)+nonPoolTTAccountsLen+len(poolKeys1)+nonPoolTTAccountsLen+len(poolKeys2))
		// Token indexes are relative to the remaining accounts which exclude the mandatory accounts at the beginning
		remainingAccounts := newAccounts[mandatoryExecuteAccountsLen:]
		require.Len(t, remainingAccounts, requiredMessagingAccountsLen+len(userMessagingAccounts)+nonPoolTTAccountsLen+len(poolKeys1)+nonPoolTTAccountsLen+len(poolKeys2))
		// logic receiver is the first account in remaining accounts
		require.Equal(t, logicReceiver, remainingAccounts[0].PublicKey)
		// external execution signer is the second account in remaining accounts
		require.Equal(t, externalExecutionConfig, remainingAccounts[1].PublicKey)
		for i, tokenIdx := range typedArgs.TokenIndexes {
			startIdx := tokenIdx
			var endIdx uint8
			if i < len(typedArgs.TokenIndexes)-1 {
				endIdx = typedArgs.TokenIndexes[i+1]
			} else {
				endIdx = uint8(len(remainingAccounts))
			}
			tokenAccounts := remainingAccounts[startIdx:endIdx]
			if i == 0 {
				require.Len(t, tokenAccounts, nonPoolTTAccountsLen+len(poolKeys1)) // offramp pools signer + user token account + per chain token config + pool chain config + 7 pool keys
				require.Equal(t, &solana.AccountMeta{PublicKey: offrampPoolsSigner1, IsWritable: false, IsSigner: false}, tokenAccounts[0])
				require.Equal(t, &solana.AccountMeta{PublicKey: userTokenAccount1, IsWritable: true}, tokenAccounts[1])
				require.Equal(t, &solana.AccountMeta{PublicKey: perChainTokenConfig1}, tokenAccounts[2])
				require.Equal(t, &solana.AccountMeta{PublicKey: poolChainConfig1, IsWritable: true}, tokenAccounts[3])
			} else {
				require.Len(t, tokenAccounts, nonPoolTTAccountsLen+len(poolKeys2)) // offramp pools signer + user token account + per chain token config + pool chain config + 7 pool keys
				require.Equal(t, &solana.AccountMeta{PublicKey: offrampPoolsSigner2, IsWritable: false, IsSigner: false}, tokenAccounts[0])
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
		require.Equal(t, uint8(requiredMessagingAccountsLen+len(userMessagingAccounts)+nonPoolTTAccountsLen+len(poolKeys1)), typedArgs.TokenIndexes[1])
	})

	t.Run("ArgsTransform ignores user messaging accounts if logic receiver is empty", func(t *testing.T) {
		// Mock the account derivation simulations
		mockExecuteAccountDerivation(t, rw, offrampAddress.String(), userMessagingAccounts, []tokenTransferAccounts{ttAccount1}, solana.PublicKey{}, []solana.PublicKey{lookupTablePubkey1})

		missingLogicReceiverArgs := ccipsolana.SVMExecCallArgs{
			Info: ccipocr3.ExecuteReportInfo{
				AbstractReports: []ccipocr3.ExecutePluginReportSingleChain{{
					Messages: []ccipocr3.Message{{
						Header: ccipocr3.RampMessageHeader{SourceChainSelector: sourceChainSelector},
						TokenAmounts: []ccipocr3.RampTokenAmount{
							{
								DestTokenAddress:  destTokenAddr1.Bytes(),
								SourcePoolAddress: sourcePoolAddr1.Bytes(),
								Amount:            ccipocr3.NewBigInt(big.NewInt(1)),
							},
						}},
					},
					OffchainTokenData: [][][]byte{{}},
				}},
				MerkleRoots: []ccipocr3.MerkleRootChain{{MerkleRoot: merkleRoot}},
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

		staticLookupTableMaps := make(map[solana.PublicKey]solana.PublicKeySlice)
		transformedArgs, newAccounts, lookupTableMap, options, err := chainwriter.CCIPExecuteArgsTransformV2(ctx, mc, lggr, missingLogicReceiverArgs, nil, staticLookupTableMaps, nil, fromAddress, offrampAddress.String(), staticCUOverhead, []txmutils.SetTxConfig{}, "")
		require.NoError(t, err)
		verifyTxOpts(t, options, true, staticCUOverhead, userCU, destGasAmount)

		typedArgs, ok := transformedArgs.(ccipsolana.SVMExecCallArgs)
		require.True(t, ok)
		require.NotNil(t, typedArgs.TokenIndexes)
		require.Len(t, typedArgs.TokenIndexes, 1)
		require.Equal(t, uint8(0), typedArgs.TokenIndexes[0]) // Token index is 0 because no user messaging accounts precede token transfer accounts
		// mandatory accounts + 4 token accounts for TokenAmounts[0] + 7 pool keys
		require.Len(t, newAccounts, mandatoryExecuteAccountsLen+nonPoolTTAccountsLen+len(poolKeys1))
		require.Len(t, lookupTableMap, 1) // contains pool lookup table for the single token transfer
	})

	t.Run("ArgsTransform ignores token transfer related errors if accounts not required", func(t *testing.T) {
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
				MerkleRoots: []ccipocr3.MerkleRootChain{{MerkleRoot: merkleRoot}},
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

		// Mock the account derivation simulations
		mockExecuteAccountDerivation(t, rw, offrampAddress.String(), userMessagingAccounts, nil, logicReceiver, nil)
		transformedArgs, newAccounts, lookupTableMap, options, err := chainwriter.CCIPExecuteArgsTransformV2(ctx, mc, lggr, messagingOnlyArgs, nil, nil, nil, fromAddress, offrampAddress.String(), staticCUOverhead, []txmutils.SetTxConfig{}, "")
		require.NoError(t, err)
		verifyTxOpts(t, options, true, staticCUOverhead, userCU, destGasAmount)

		typedArgs, ok := transformedArgs.(ccipsolana.SVMExecCallArgs)
		require.True(t, ok)
		require.NotNil(t, typedArgs.TokenIndexes)
		require.Len(t, typedArgs.TokenIndexes, 0)
		// mandatory accounts + 2 requiredMessagingAccountsLen + 3 for user messaging accounts
		require.Len(t, newAccounts, mandatoryExecuteAccountsLen+requiredMessagingAccountsLen+len(userMessagingAccounts))
		require.Len(t, lookupTableMap, 0) // no lookup tables returned if there are no token transfers
	})

	t.Run("ArgsTransform failed if token transfer accounts are required and the token receiver is empty", func(t *testing.T) {
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
				MerkleRoots: []ccipocr3.MerkleRootChain{{MerkleRoot: merkleRoot}},
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

		_, _, _, _, err := chainwriter.CCIPExecuteArgsTransformV2(ctx, mc, lggr, missingTokenReceiverArgs, nil, nil, nil, fromAddress, offrampAddress.String(), 0, []txmutils.SetTxConfig{}, "")
		require.Error(t, err)
	})

	t.Run("ArgsTransform does not include any remaining accounts if both logic and token receivers are missing", func(t *testing.T) {
		// Mock the account derivation simulations
		mockExecuteAccountDerivation(t, rw, offrampAddress.String(), userMessagingAccounts, nil, solana.PublicKey{}, nil)
		missingBothReceiverArgs := ccipsolana.SVMExecCallArgs{
			Info: ccipocr3.ExecuteReportInfo{
				AbstractReports: []ccipocr3.ExecutePluginReportSingleChain{{
					Messages: []ccipocr3.Message{
						{
							Header: ccipocr3.RampMessageHeader{SourceChainSelector: sourceChainSelector},
						},
					},
				}},
				MerkleRoots: []ccipocr3.MerkleRootChain{{MerkleRoot: merkleRoot}},
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

		transformedArgs, newAccounts, lookupTableMap, options, err := chainwriter.CCIPExecuteArgsTransformV2(ctx, mc, lggr, missingBothReceiverArgs, nil, nil, nil, fromAddress, offrampAddress.String(), staticCUOverhead, []txmutils.SetTxConfig{}, "")
		require.NoError(t, err)
		verifyTxOpts(t, options, true, staticCUOverhead, userCU, destGasAmount)
		typedArgs, ok := transformedArgs.(ccipsolana.SVMExecCallArgs)
		require.True(t, ok)
		require.NotNil(t, typedArgs.TokenIndexes)
		require.Len(t, typedArgs.TokenIndexes, 0)
		// no extra accounts are added so new accounts should equal mandatory accounts
		require.Len(t, newAccounts, mandatoryExecuteAccountsLen)
		require.Len(t, lookupTableMap, 0) // no lookup tables returned if there are no token transfers
	})

	t.Run("ArgsTransform fails if token transfer accounts is required and lookup table not found", func(t *testing.T) {
		badLookupTable := GetRandomPubKey(t)
		// Mock the account derivation simulations
		mockExecuteAccountDerivation(t, rw, offrampAddress.String(), userMessagingAccounts, []tokenTransferAccounts{ttAccount1}, solana.PublicKey{}, []solana.PublicKey{badLookupTable})
		rw.On("GetAccountInfoWithOpts", mock.Anything, badLookupTable, mock.Anything).Return(nil, errors.New("failed to fetch lookup table")).Once()
		staticLookupTableMaps := make(map[solana.PublicKey]solana.PublicKeySlice)
		_, _, _, _, err := chainwriter.CCIPExecuteArgsTransformV2(ctx, mc, lggr, args, nil, staticLookupTableMaps, nil, fromAddress, offrampAddress.String(), 0, []txmutils.SetTxConfig{}, "")
		require.ErrorContains(t, err, "failed to fetch lookup table")
	})

	t.Run("ArgsTransform does not get args that conform to ReportPreTransform", func(t *testing.T) {
		// Mock the account derivation simulations
		mockExecuteAccountDerivation(t, rw, offrampAddress.String(), userMessagingAccounts, nil, solana.PublicKey{}, nil)
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
				MerkleRoots: []ccipocr3.MerkleRootChain{{MerkleRoot: merkleRoot}},
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
		transformedArgs, newAccounts, _, options, err := chainwriter.CCIPExecuteArgsTransformV2(ctx, mc, lggr, args, nil, nil, nil, fromAddress, offrampAddress.String(), staticCUOverhead, []txmutils.SetTxConfig{}, "")
		require.NoError(t, err)

		verifyTxOpts(t, options, true, staticCUOverhead, userCU, destGasAmount)
		_, ok := transformedArgs.(ccipsolana.SVMExecCallArgs)
		require.True(t, ok)
		require.Len(t, newAccounts, mandatoryExecuteAccountsLen)
	})

	t.Run("ArgsTransform fails with empty Info", func(t *testing.T) {
		args := struct {
			ReportContext [2][32]uint8
			Report        []uint8
			Info          ccipocr3.ExecuteReportInfo
		}{
			ReportContext: [2][32]uint8{},
			Report:        []uint8{},
			Info:          ccipocr3.ExecuteReportInfo{},
		}
		_, _, _, _, err := chainwriter.CCIPExecuteArgsTransformV2(ctx, mc, lggr, args, nil, nil, nil, solana.PublicKey{}, offrampAddress.String(), 0, []txmutils.SetTxConfig{}, "")
		require.Contains(t, err.Error(), "computeUnits not found in ExtraArgsDecoded")
	})

	t.Run("ArgsTransform fails with unexpected number of reports, messages, or merkle roots", func(t *testing.T) {
		emptyArgs := ccipsolana.SVMExecCallArgs{
			Info: ccipocr3.ExecuteReportInfo{
				AbstractReports: []ccipocr3.ExecutePluginReportSingleChain{},
				MerkleRoots:     []ccipocr3.MerkleRootChain{},
			},
			ExtraData: ccipsolana.ExtraDataDecoded{
				ExtraArgsDecoded: map[string]any{
					"computeUnits": uint32(500),
				},
			},
		}
		multiReport := emptyArgs
		report := ccipocr3.ExecutePluginReportSingleChain{
			Messages: []ccipocr3.Message{{Header: ccipocr3.RampMessageHeader{SourceChainSelector: sourceChainSelector}}},
		}
		multiReport.Info.AbstractReports = []ccipocr3.ExecutePluginReportSingleChain{report, report}
		_, _, _, _, err := chainwriter.CCIPExecuteArgsTransformV2(ctx, mc, lggr, multiReport, nil, nil, nil, solana.PublicKey{}, offrampAddress.String(), 0, []txmutils.SetTxConfig{}, "")
		require.Contains(t, err.Error(), "encountered unexpected number of reports")

		multiMessage := emptyArgs
		message := ccipocr3.Message{Header: ccipocr3.RampMessageHeader{SourceChainSelector: sourceChainSelector}}
		multiMessage.Info.AbstractReports = []ccipocr3.ExecutePluginReportSingleChain{report}
		multiMessage.Info.AbstractReports[0].Messages = []ccipocr3.Message{message, message}
		_, _, _, _, err = chainwriter.CCIPExecuteArgsTransformV2(ctx, mc, lggr, multiMessage, nil, nil, nil, solana.PublicKey{}, offrampAddress.String(), 0, []txmutils.SetTxConfig{}, "")
		require.Contains(t, err.Error(), "encountered unexpected number of messages")

		multiMerkleRoots := emptyArgs
		merkleRoot := ccipocr3.MerkleRootChain{MerkleRoot: [32]byte{}}
		multiMerkleRoots.Info.AbstractReports = []ccipocr3.ExecutePluginReportSingleChain{report}
		multiMerkleRoots.Info.AbstractReports[0].Messages = []ccipocr3.Message{message}
		multiMerkleRoots.Info.MerkleRoots = []ccipocr3.MerkleRootChain{merkleRoot, merkleRoot}
		_, _, _, _, err = chainwriter.CCIPExecuteArgsTransformV2(ctx, mc, lggr, multiMerkleRoots, nil, nil, nil, solana.PublicKey{}, offrampAddress.String(), 0, []txmutils.SetTxConfig{}, "")
		require.Contains(t, err.Error(), "encountered unexpected number of merkle roots")
	})

	t.Run("ArgsTransform fails if accounts exist at start", func(t *testing.T) {
		_, _, _, _, err := chainwriter.CCIPExecuteArgsTransformV2(ctx, mc, lggr, nil, solana.AccountMetaSlice{&solana.AccountMeta{PublicKey: solana.PublicKey{}}}, nil, nil, solana.PublicKey{}, offrampAddress.String(), 0, []txmutils.SetTxConfig{}, "")
		require.ErrorContains(t, err, "expect accounts to be empty at start of CCIPExecuteArgsTransformV2")
	})

	t.Run("ArgsTransform fails if unexpected number of OffchainTokenData encountered", func(t *testing.T) {
		missingTokenDataArgs := ccipsolana.SVMExecCallArgs{
			Info: ccipocr3.ExecuteReportInfo{
				AbstractReports: []ccipocr3.ExecutePluginReportSingleChain{{
					Messages: []ccipocr3.Message{{
						Header: ccipocr3.RampMessageHeader{SourceChainSelector: sourceChainSelector},
						TokenAmounts: []ccipocr3.RampTokenAmount{
							{
								DestTokenAddress:  destTokenAddr1.Bytes(),
								Amount:            ccipocr3.NewBigInt(big.NewInt(1)),
								SourcePoolAddress: sourcePoolAddr1.Bytes(),
							},
						}},
					},
				}},
				MerkleRoots: []ccipocr3.MerkleRootChain{{MerkleRoot: merkleRoot}},
			},
			ExtraData: ccipsolana.ExtraDataDecoded{
				ExtraArgsDecoded: map[string]any{
					"tokenReceiver": tokenReceiver,
					"computeUnits":  uint32(0),
				},
				DestExecDataDecoded: []map[string]any{
					{"destGasAmount": destGasAmount},
				},
			},
		}
		_, _, _, _, err := chainwriter.CCIPExecuteArgsTransformV2(ctx, mc, lggr, missingTokenDataArgs, nil, nil, nil, solana.PublicKey{}, offrampAddress.String(), 0, []txmutils.SetTxConfig{}, "")
		require.ErrorContains(t, err, "unexpected number of OffchainTokenData encountered. expect the same number as messages")
	})
}

func Test_CCIPCommitAccountTransform(t *testing.T) {
	ctx := t.Context()
	lggr := logger.Test(t)

	// mock client
	rw := clientmocks.NewReaderWriter(t)
	mc := *client.NewMultiClient(func(context.Context) (client.ReaderWriter, error) {
		return rw, nil
	})

	key1 := GetRandomPubKey(t)
	key2 := GetRandomPubKey(t)
	staticCUOverhead := uint32(150_000)
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
		_, newAccounts, _, options, err := chainwriter.CCIPCommitAccountTransform(ctx, mc, lggr, args, accounts, nil, nil, solana.PublicKey{}, "", staticCUOverhead, []txmutils.SetTxConfig{}, "")
		verifyTxOpts(t, options, false, staticCUOverhead, 0, 0)
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
		_, newAccounts, _, _, err := chainwriter.CCIPCommitAccountTransform(ctx, mc, lggr, args, accounts, nil, nil, solana.PublicKey{}, "", 0, []txmutils.SetTxConfig{}, "")
		require.NoError(t, err)
		require.Len(t, newAccounts, 1)
	})

	t.Run("CCIPCommit ArgsTransform no-ops if accounts list is empty", func(t *testing.T) {
		args := struct {
			Info ccipocr3.CommitReportInfo
		}{
			Info: ccipocr3.CommitReportInfo{},
		}
		_, newAccounts, _, _, err := chainwriter.CCIPCommitAccountTransform(ctx, mc, lggr, args, nil, nil, nil, solana.PublicKey{}, "", 0, []txmutils.SetTxConfig{}, "")
		require.NoError(t, err)
		require.Len(t, newAccounts, 0)
	})
}

func verifyTxOpts(t *testing.T, options []txmutils.SetTxConfig, exec bool, overhead, userCU, destGasAmounts uint32) {
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
		require.Equal(t, overhead+userCU+destGasAmounts, txConfig.ComputeUnitLimit)
	}
}

// Note: Other than the static token transfer stage required for token indices, these stages are implementation details on-chain
// It's ok if they drift from the on-chain version if any steps are added/removed. This is just to mock out an example of different stages for account derivation
func mockExecuteAccountDerivation(t *testing.T, rw *clientmocks.ReaderWriter, offrampStr string, userMessagingAccounts []solana.PublicKey, ttAccounts []tokenTransferAccounts, logicReceiver solana.PublicKey, lookupTables []solana.PublicKey) {
	recentBlockHash := solana.Hash{}
	rw.On("LatestBlockhash", mock.Anything).Return(&rpc.GetLatestBlockhashResult{Value: &rpc.LatestBlockhashResult{Blockhash: recentBlockHash, LastValidBlockHeight: uint64(100)}}, nil).Once()
	mockGatherBasicInfoStage(t, rw, offrampStr)
	mockMainAccountListStage(t, rw, offrampStr, userMessagingAccounts, logicReceiver, ttAccounts)
	mockRetrieveLUTStage(t, rw, offrampStr, ttAccounts)
	mockTokenTransferStages(t, rw, offrampStr, ttAccounts, lookupTables)
}

func mockGatherBasicInfoStage(t *testing.T, rw *clientmocks.ReaderWriter, offrampStr string) {
	basicAccountsLen := 3
	toSave := make([]ccip_offramp_v0_1_1.CcipAccountMeta, 0, basicAccountsLen)
	basicAccounts := CreateTestPubKeys(t, basicAccountsLen)
	for _, addr := range basicAccounts {
		toSave = append(toSave, ccip_offramp_v0_1_1.CcipAccountMeta{Pubkey: addr})
	}
	// Proper ask again accounts do not have to be returned since the follow up derivation call mock does not check them. Just can't return empty accounts
	askAgain := toSave[0:]
	log := buildEncodedResponse(t, offrampStr, toSave, askAgain, nil, "GatherBasicInfo", "BuildMainAccountList")
	rw.On("SimulateTx", mock.Anything, mock.Anything, &rpc.SimulateTransactionOpts{SigVerify: false, ReplaceRecentBlockhash: true}).Return(&rpc.SimulateTransactionResult{Logs: []string{log}}, nil).Once()
}

func mockMainAccountListStage(t *testing.T, rw *clientmocks.ReaderWriter, offrampStr string, userMessagingAccounts []solana.PublicKey, logicReceiver solana.PublicKey, ttAccounts []tokenTransferAccounts) {
	requiredAccounts := CreateTestPubKeys(t, 9)
	toSave := []ccip_offramp_v0_1_1.CcipAccountMeta{}
	for _, addr := range requiredAccounts {
		toSave = append(toSave, ccip_offramp_v0_1_1.CcipAccountMeta{Pubkey: addr})
	}

	if !logicReceiver.IsZero() {
		toSave = append(toSave, ccip_offramp_v0_1_1.CcipAccountMeta{
			Pubkey:     logicReceiver,
			IsSigner:   false,
			IsWritable: true,
		})
		offramp := solana.MustPublicKeyFromBase58(offrampStr)
		externalExecutionConfig, _, err := state.FindExternalExecutionConfigPDA(logicReceiver, offramp)
		require.NoError(t, err)
		toSave = append(toSave, ccip_offramp_v0_1_1.CcipAccountMeta{
			Pubkey:     externalExecutionConfig,
			IsSigner:   false,
			IsWritable: false,
		})
		userMessagingMetas := []*solana.AccountMeta{}
		for _, addr := range userMessagingAccounts {
			userMessagingMetas = append(userMessagingMetas, &solana.AccountMeta{PublicKey: addr})
		}
		userMessagingCCIPMetas := chainwriter.ConvertToCCIPAccountMetas(userMessagingMetas)
		toSave = append(toSave, userMessagingCCIPMetas...)
	}
	// Proper ask again accounts do not have to be returned since the follow up derivation call mock does not check them. Just can't return empty accounts
	askAgain := []ccip_offramp_v0_1_1.CcipAccountMeta{}
	nextStage := ""
	if len(ttAccounts) > 0 {
		askAgain = toSave[:1]
		nextStage = "RetrieveTokenLookupTables"
	}
	log := buildEncodedResponse(t, offrampStr, toSave, askAgain, nil, "BuildMainAccountList", nextStage)
	rw.On("SimulateTx", mock.Anything, mock.Anything, &rpc.SimulateTransactionOpts{SigVerify: false, ReplaceRecentBlockhash: true}).Return(&rpc.SimulateTransactionResult{Logs: []string{log}}, nil).Once()
}

func mockRetrieveLUTStage(t *testing.T, rw *clientmocks.ReaderWriter, offrampStr string, ttAccounts []tokenTransferAccounts) {
	if len(ttAccounts) == 0 {
		return
	}
	askAgain := []ccip_offramp_v0_1_1.CcipAccountMeta{{Pubkey: GetRandomPubKey(t)}}
	// Lookup table stage does not return accounts or lookup tables to save. Just processes accounts to ask again with.
	log := buildEncodedResponse(t, offrampStr, []ccip_offramp_v0_1_1.CcipAccountMeta{}, askAgain, nil, "RetrieveTokenLookupTables", "TokenTransferStaticAccounts/0/0")
	rw.On("SimulateTx", mock.Anything, mock.Anything, &rpc.SimulateTransactionOpts{SigVerify: false, ReplaceRecentBlockhash: true}).Return(&rpc.SimulateTransactionResult{Logs: []string{log}}, nil).Once()
}

func mockTokenTransferStages(t *testing.T, rw *clientmocks.ReaderWriter, offrampStr string, ttAccounts []tokenTransferAccounts, lookupTables []solana.PublicKey) {
	for i, ttAccount := range ttAccounts {
		toSave := []ccip_offramp_v0_1_1.CcipAccountMeta{
			{
				Pubkey: ttAccount.offrampPoolSigner,
			},
			{
				Pubkey:     ttAccount.userTokenAccount,
				IsWritable: true,
			},
			{
				Pubkey: ttAccount.perChainTokenConfig,
			},
			{
				Pubkey:     ttAccount.poolChainConfig,
				IsWritable: true,
			},
		}
		for _, poolKey := range ttAccount.poolKeys {
			toSave = append(toSave, ccip_offramp_v0_1_1.CcipAccountMeta{
				Pubkey:     poolKey,
				IsWritable: true,
			})
		}
		var askAgain []ccip_offramp_v0_1_1.CcipAccountMeta
		nextStage := ""
		if i < len(ttAccounts)-1 {
			nextStage = "TokenTransferStaticAccounts/" + strconv.Itoa(i+1) + "/0"
			askAgain = []ccip_offramp_v0_1_1.CcipAccountMeta{{Pubkey: ttAccounts[i+1].mint}}
		}
		log := buildEncodedResponse(t, offrampStr, toSave, askAgain, []solana.PublicKey{lookupTables[i]}, "TokenTransferStaticAccounts/"+strconv.Itoa(i)+"/0", nextStage)
		rw.On("SimulateTx", mock.Anything, mock.Anything, &rpc.SimulateTransactionOpts{SigVerify: false, ReplaceRecentBlockhash: true}).Return(&rpc.SimulateTransactionResult{Logs: []string{log}}, nil).Once()
	}
}

func buildEncodedResponse(t *testing.T, offramp string, toSave, askAgainWith []ccip_offramp_v0_1_1.CcipAccountMeta, lookupTables []solana.PublicKey, currentStage, nextStage string) string {
	response := ccip_offramp_v0_1_1.DeriveAccountsResponse{
		AccountsToSave:     toSave,
		AskAgainWith:       askAgainWith,
		CurrentStage:       currentStage,
		NextStage:          nextStage,
		LookUpTablesToSave: lookupTables,
	}
	buf := new(bytes.Buffer)
	err := response.MarshalWithEncoder(bin.NewBorshEncoder(buf))
	require.NoError(t, err)
	encodedRes := base64.StdEncoding.EncodeToString(buf.Bytes())

	return "Program return: " + offramp + " " + encodedRes
}
