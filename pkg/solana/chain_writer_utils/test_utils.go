package chainwriterutils

import (
	"bytes"
	"encoding/base64"
	"strconv"
	"testing"

	bin "github.com/gagliardetto/binary"
	solana "github.com/gagliardetto/solana-go"
	addresslookuptable "github.com/gagliardetto/solana-go/programs/address-lookup-table"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	ccip_offramp_v0_1_1 "github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/v0_1_1/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/state"

	clientmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/mocks"
)

func GetRandomPubKey(t *testing.T) solana.PublicKey {
	t.Helper()
	privKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	return privKey.PublicKey()
}

func CreateTestPubKeys(t *testing.T, num int) solana.PublicKeySlice {
	t.Helper()

	addresses := make([]solana.PublicKey, num)
	for i := 0; i < num; i++ {
		addresses[i] = GetRandomPubKey(t)
	}
	return addresses
}

type TokenTransferAccounts struct {
	OfframpPoolSigner   solana.PublicKey
	UserTokenAccount    solana.PublicKey
	PerChainTokenConfig solana.PublicKey
	PoolChainConfig     solana.PublicKey
	PoolKeys            []solana.PublicKey
	Mint                solana.PublicKey
}

// Note: Other than the static token transfer stage required for token indices, these stages are implementation details on-chain
// It's ok if they drift from the on-chain version if any steps are added/removed. This is just to mock out an example of different stages for account derivation
func MockExecuteAccountDerivation(t *testing.T, rw *clientmocks.ReaderWriter, offrampStr string, userMessagingAccounts []solana.PublicKey, ttAccounts []TokenTransferAccounts, logicReceiver solana.PublicKey, lookupTables []solana.PublicKey) {
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

func mockMainAccountListStage(t *testing.T, rw *clientmocks.ReaderWriter, offrampStr string, userMessagingAccounts []solana.PublicKey, logicReceiver solana.PublicKey, ttAccounts []TokenTransferAccounts) {
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
		userMessagingCCIPMetas := ConvertToCCIPAccountMetas(userMessagingMetas)
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

func mockRetrieveLUTStage(t *testing.T, rw *clientmocks.ReaderWriter, offrampStr string, ttAccounts []TokenTransferAccounts) {
	if len(ttAccounts) == 0 {
		return
	}
	askAgain := []ccip_offramp_v0_1_1.CcipAccountMeta{{Pubkey: GetRandomPubKey(t)}}
	// Lookup table stage does not return accounts or lookup tables to save. Just processes accounts to ask again with.
	log := buildEncodedResponse(t, offrampStr, []ccip_offramp_v0_1_1.CcipAccountMeta{}, askAgain, nil, "RetrieveTokenLookupTables", "TokenTransferStaticAccounts/0/0")
	rw.On("SimulateTx", mock.Anything, mock.Anything, &rpc.SimulateTransactionOpts{SigVerify: false, ReplaceRecentBlockhash: true}).Return(&rpc.SimulateTransactionResult{Logs: []string{log}}, nil).Once()
}

func mockTokenTransferStages(t *testing.T, rw *clientmocks.ReaderWriter, offrampStr string, ttAccounts []TokenTransferAccounts, lookupTables []solana.PublicKey) {
	for i, ttAccount := range ttAccounts {
		toSave := []ccip_offramp_v0_1_1.CcipAccountMeta{
			{
				Pubkey: ttAccount.OfframpPoolSigner,
			},
			{
				Pubkey:     ttAccount.UserTokenAccount,
				IsWritable: true,
			},
			{
				Pubkey: ttAccount.PerChainTokenConfig,
			},
			{
				Pubkey:     ttAccount.PoolChainConfig,
				IsWritable: true,
			},
		}
		for _, poolKey := range ttAccount.PoolKeys {
			toSave = append(toSave, ccip_offramp_v0_1_1.CcipAccountMeta{
				Pubkey:     poolKey,
				IsWritable: true,
			})
		}
		var askAgain []ccip_offramp_v0_1_1.CcipAccountMeta
		nextStage := ""
		if i < len(ttAccounts)-1 {
			nextStage = "TokenTransferStaticAccounts/" + strconv.Itoa(i+1) + "/0"
			askAgain = []ccip_offramp_v0_1_1.CcipAccountMeta{{Pubkey: ttAccounts[i+1].Mint}}
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

func MockFetchLookupTableAddresses(t *testing.T, rw *clientmocks.ReaderWriter, lookupTablePubkey solana.PublicKey, storedPubkeys []solana.PublicKey) {
	var lookupTablePubkeySlice solana.PublicKeySlice
	lookupTablePubkeySlice.Append(storedPubkeys...)
	lookupTableState := addresslookuptable.AddressLookupTableState{
		Addresses: lookupTablePubkeySlice,
	}
	lookupTableStateBytes := mustBorshEncodeStruct(t, lookupTableState)
	rw.On("GetAccountInfoWithOpts", mock.Anything, lookupTablePubkey, mock.Anything).Return(&rpc.GetAccountInfoResult{
		RPCContext: rpc.RPCContext{},
		Value:      &rpc.Account{Data: rpc.DataBytesOrJSONFromBytes(lookupTableStateBytes)},
	}, nil)
}

func mustBorshEncodeStruct(t *testing.T, data interface{}) []byte {
	buf := new(bytes.Buffer)
	err := bin.NewBorshEncoder(buf).Encode(data)
	require.NoError(t, err)
	return buf.Bytes()
}

func MustFindPdaProgramAddress(t *testing.T, seeds [][]byte, programID solana.PublicKey) solana.PublicKey {
	t.Helper()
	pda, _, err := solana.FindProgramAddress(seeds, programID)
	require.NoError(t, err)
	return pda
}

func MockDataAccountLookupTable(t *testing.T, rw *clientmocks.ReaderWriter, pda solana.PublicKey) solana.PublicKey {
	t.Helper()
	lookupTablePubkey := GetRandomPubKey(t)
	dataAccount := DataAccount{
		Version:              1,
		Administrator:        GetRandomPubKey(t),
		PendingAdministrator: GetRandomPubKey(t),
		LookupTable:          lookupTablePubkey,
	}
	dataAccountBytes := mustBorshEncodeStruct(t, dataAccount)
	// codec will expect discriminator
	dataAccountBytes = append([]byte{220, 119, 44, 40, 237, 41, 223, 7}, dataAccountBytes...)
	rw.On("GetAccountInfoWithOpts", mock.Anything, pda, mock.Anything).Return(&rpc.GetAccountInfoResult{
		RPCContext: rpc.RPCContext{},
		Value:      &rpc.Account{Data: rpc.DataBytesOrJSONFromBytes(dataAccountBytes)},
	}, nil)
	return lookupTablePubkey
}
