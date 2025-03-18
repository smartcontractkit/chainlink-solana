package chainwriter_test

import (
	"context"
	"encoding/binary"
	"math"
	"testing"

	ag_binary "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip/pkg/types/ccipocr3"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainwriter"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	clientmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/mocks"
)

type ReportPreTransform struct {
	ReportContext  [2][32]byte
	Report         []byte
	Info           ccipocr3.ExecuteReportInfo
	AbstractReport ccip_offramp.ExecutionReportSingleChain
}

func Test_CCIPExecuteArgsTransform(t *testing.T) {
	ctx := tests.Context(t)

	// mock client
	rw := clientmocks.NewReaderWriter(t)
	mc := *client.NewMultiClient(func(context.Context) (client.ReaderWriter, error) {
		return rw, nil
	})

	receiver := chainwriter.GetRandomPubKey(t)
	offrampAddress := chainwriter.GetRandomPubKey(t)
	destTokenAddr1 := chainwriter.GetRandomPubKey(t)
	destTokenAddr2 := chainwriter.GetRandomPubKey(t)
	poolKeys := chainwriter.CreateTestPubKeys(t, 7)
	tokenAdminRegistryAddr := poolKeys[1]
	poolProgram := poolKeys[2]
	tokenProgram := poolKeys[6]
	sourceChainSelector := ccipocr3.ChainSelector(1)
	feeQuoterAddr := chainwriter.GetRandomPubKey(t)
	mockFetchFeeQuoterAddress(t, rw, feeQuoterAddr, offrampAddress)

	sourceChainSelBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(sourceChainSelBytes, uint64(sourceChainSelector))

	userTokenAccount1, _, err := solana.FindProgramAddress([][]byte{receiver.Bytes(), tokenProgram.Bytes(), destTokenAddr1.Bytes()}, solana.SPLAssociatedTokenAccountProgramID)
	require.NoError(t, err)
	perChainTokenConfig1, _, err := solana.FindProgramAddress([][]byte{[]byte("per_chain_per_token_config"), sourceChainSelBytes, destTokenAddr1.Bytes()}, feeQuoterAddr)
	require.NoError(t, err)
	poolChainConfig1, _, err := solana.FindProgramAddress([][]byte{[]byte("ccip_tokenpool_chainconfig"), sourceChainSelBytes, destTokenAddr1.Bytes()}, poolProgram)
	require.NoError(t, err)

	userTokenAccount2, _, err := solana.FindProgramAddress([][]byte{receiver.Bytes(), tokenProgram.Bytes(), destTokenAddr2.Bytes()}, solana.SPLAssociatedTokenAccountProgramID)
	require.NoError(t, err)
	perChainTokenConfig2, _, err := solana.FindProgramAddress([][]byte{[]byte("per_chain_per_token_config"), sourceChainSelBytes, destTokenAddr2.Bytes()}, feeQuoterAddr)
	require.NoError(t, err)
	poolChainConfig2, _, err := solana.FindProgramAddress([][]byte{[]byte("ccip_tokenpool_chainconfig"), sourceChainSelBytes, destTokenAddr2.Bytes()}, poolProgram)
	require.NoError(t, err)

	args := ReportPreTransform{
		Info: ccipocr3.ExecuteReportInfo{
			AbstractReports: []ccipocr3.ExecutePluginReportSingleChain{{
				Messages: []ccipocr3.Message{{
					Receiver: receiver.Bytes(),
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
	}

	t.Run("CCIPExecute ArgsTransform includes token indexes and sets the corresponding IsWritable flag", func(t *testing.T) {
		mandatoryAccounts := chainwriter.CreateTestPubKeys(t, chainwriter.MandatoryExecuteAccounts)
		userAccounts := chainwriter.CreateTestPubKeys(t, 4) // arbitrary number of user accounts
		// Accounts list contains other accounts before token addresses
		accounts := make([]*solana.AccountMeta, 0, len(mandatoryAccounts)+len(userAccounts))
		for _, acc := range mandatoryAccounts {
			accounts = append(accounts, &solana.AccountMeta{PublicKey: acc})
		}
		for _, acc := range userAccounts {
			accounts = append(accounts, &solana.AccountMeta{PublicKey: acc})
		}

		tableMap := make(map[string]map[string][]*solana.AccountMeta)
		tableMap["PoolLookupTable"] = make(map[string][]*solana.AccountMeta)
		lookupTablePubkey := chainwriter.GetRandomPubKey(t)

		poolKeysMeta := make([]*solana.AccountMeta, 0, len(poolKeys))
		// second address in pool lookup table is expected to be the token admin registry address needed to fetch the WritableIndexes
		mockWritableIndexes(t, rw, tokenAdminRegistryAddr)
		for _, poolKey := range poolKeys {
			poolKeysMeta = append(poolKeysMeta, &solana.AccountMeta{PublicKey: poolKey})
		}
		tableMap["PoolLookupTable"][lookupTablePubkey.String()] = poolKeysMeta

		transformedArgs, newAccounts, err := chainwriter.CCIPExecuteArgsTransform(ctx, mc, args, accounts, tableMap, offrampAddress.String())
		require.NoError(t, err)
		typedArgs, ok := transformedArgs.(chainwriter.ReportPostTransform)
		require.True(t, ok)
		require.NotNil(t, typedArgs.TokenIndexes)
		require.Len(t, typedArgs.TokenIndexes, 2)
		// mandatory accounts + user accounts + 3 token accounts for TokenAmounts[0] + 7 pool keys + 3 token accounts for TokenAmounts[1] + 7 pool keys
		require.Len(t, newAccounts, len(mandatoryAccounts)+len(userAccounts)+3+len(poolKeys)+3+len(poolKeys))
		// Token indexes are relative to the remaining accounts which exclude the mandatory accounts at the beginning
		remainingAccounts := newAccounts[chainwriter.MandatoryExecuteAccounts:]
		require.Len(t, remainingAccounts, len(userAccounts)+3+len(poolKeys)+3+len(poolKeys))
		for i, tokenIdx := range typedArgs.TokenIndexes {
			startIdx := tokenIdx
			var endIdx uint8
			if i < len(typedArgs.TokenIndexes)-1 {
				endIdx = typedArgs.TokenIndexes[i+1]
			} else {
				endIdx = uint8(len(remainingAccounts))
			}
			tokenAccounts := remainingAccounts[startIdx:endIdx]
			require.Len(t, tokenAccounts, 3+len(poolKeys)) // user token account + per chain token config + pool chain config + 7 pool keys
			if i == 0 {
				require.Equal(t, &solana.AccountMeta{PublicKey: userTokenAccount1, IsWritable: true}, tokenAccounts[0])
				require.Equal(t, &solana.AccountMeta{PublicKey: perChainTokenConfig1}, tokenAccounts[1])
				require.Equal(t, &solana.AccountMeta{PublicKey: poolChainConfig1, IsWritable: true}, tokenAccounts[2])
			} else {
				require.Equal(t, &solana.AccountMeta{PublicKey: userTokenAccount2, IsWritable: true}, tokenAccounts[0])
				require.Equal(t, &solana.AccountMeta{PublicKey: perChainTokenConfig2}, tokenAccounts[1])
				require.Equal(t, &solana.AccountMeta{PublicKey: poolChainConfig2, IsWritable: true}, tokenAccounts[2])
			}
			// Pool lookup accounts should have the proper write flags set for token accounts
			for j := 3; j < len(tokenAccounts); j++ {
				require.True(t, tokenAccounts[j].IsWritable)
			}
		}
		// Token addresses shifted by userAccounts since token index is relative to remaining accounts which include user accounts at the beginning
		require.Equal(t, uint8(len(userAccounts)), typedArgs.TokenIndexes[0])
		// Token addresses shifted by user accounts + the previous token accounts
		require.Equal(t, uint8(len(userAccounts)+10), typedArgs.TokenIndexes[1])
	})

	t.Run("CCIPExecute ArgsTransform includes empty token indexes if lookup table not found", func(t *testing.T) {
		accounts := []*solana.AccountMeta{{PublicKey: chainwriter.GetRandomPubKey(t)}}
		transformedArgs, newAccounts, err := chainwriter.CCIPExecuteArgsTransform(ctx, mc, args, accounts, nil, offrampAddress.String())
		require.NoError(t, err)
		// Accounts should be unchanged
		require.Len(t, newAccounts, len(accounts))
		typedArgs, ok := transformedArgs.(chainwriter.ReportPostTransform)
		require.True(t, ok)
		require.NotNil(t, typedArgs.TokenIndexes)
		require.Len(t, typedArgs.TokenIndexes, 0)
	})

	t.Run("CCIPExecute ArgsTransform does not get args that conform to ReportPreTransform", func(t *testing.T) {
		accounts := []*solana.AccountMeta{{PublicKey: chainwriter.GetRandomPubKey(t)}}
		args := struct {
			ReportContext [2][32]uint8
			Report        []uint8
		}{
			ReportContext: [2][32]uint8{},
			Report:        []uint8{},
		}
		transformedArgs, newAccounts, err := chainwriter.CCIPExecuteArgsTransform(ctx, mc, args, accounts, nil, offrampAddress.String())
		require.NoError(t, err)
		_, ok := transformedArgs.(chainwriter.ReportPostTransform)
		require.True(t, ok)
		require.Len(t, newAccounts, len(accounts))
	})
}

func Test_CCIPCommitAccountTransform(t *testing.T) {
	ctx := tests.Context(t)

	// mock client
	rw := clientmocks.NewReaderWriter(t)
	mc := *client.NewMultiClient(func(context.Context) (client.ReaderWriter, error) {
		return rw, nil
	})

	key1 := chainwriter.GetRandomPubKey(t)
	key2 := chainwriter.GetRandomPubKey(t)
	t.Run("CCIPCommit ArgsTransform does not affect accounts if token prices exist", func(t *testing.T) {
		args := struct {
			Info ccipocr3.CommitReportInfo
		}{
			Info: ccipocr3.CommitReportInfo{
				PriceUpdates: ccipocr3.PriceUpdates{TokenPriceUpdates: []ccipocr3.TokenPrice{{TokenID: ccipocr3.UnknownEncodedAddress(key1.String())}}},
			},
		}
		accounts := []*solana.AccountMeta{{PublicKey: key1}, {PublicKey: key2}}
		_, newAccounts, err := chainwriter.CCIPCommitAccountTransform(ctx, mc, args, accounts, nil, "")
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
		_, newAccounts, err := chainwriter.CCIPCommitAccountTransform(ctx, mc, args, accounts, nil, "")
		require.NoError(t, err)
		require.Len(t, newAccounts, 1)
	})
}

func mockWritableIndexes(t *testing.T, rw *clientmocks.ReaderWriter, tokenAdminRegistryAddr solana.PublicKey) {
	lookupTablePubkey := chainwriter.GetRandomPubKey(t)
	tokenAdminRegistry := ccip_router.TokenAdminRegistry{
		Version:              1,
		Administrator:        chainwriter.GetRandomPubKey(t),
		PendingAdministrator: chainwriter.GetRandomPubKey(t),
		LookupTable:          lookupTablePubkey,
		// set all accounts as writable
		WritableIndexes: [2]ag_binary.Uint128{{Endianness: ag_binary.LE, Lo: math.MaxUint64, Hi: math.MaxUint64}},
	}
	registryBytes := mustBorshEncodeStruct(t, tokenAdminRegistry)
	rw.On("GetAccountInfoWithOpts", mock.Anything, tokenAdminRegistryAddr, mock.Anything).Return(&rpc.GetAccountInfoResult{
		RPCContext: rpc.RPCContext{},
		Value:      &rpc.Account{Data: rpc.DataBytesOrJSONFromBytes(registryBytes)},
	}, nil)
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
	}, nil)
}
