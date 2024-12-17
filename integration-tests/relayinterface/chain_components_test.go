/*
Package relayinterface contains the interface tests for chain components.
*/
package relayinterface

import (
	"context"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/ws"
	"github.com/gagliardetto/solana-go/text"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	commontestutils "github.com/smartcontractkit/chainlink-common/pkg/loop/testutils"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	. "github.com/smartcontractkit/chainlink-common/pkg/types/interfacetests" //nolint common practice to import test mods with .
	"github.com/smartcontractkit/chainlink-common/pkg/types/query/primitives"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	contract "github.com/smartcontractkit/chainlink-solana/contracts/generated/contract_reader_interface"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/solclient"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/utils"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainreader"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

func TestChainComponents(t *testing.T) {
	t.Parallel()
	it := &SolanaChainComponentsInterfaceTester[*testing.T]{Helper: &helper{}}
	it.Init(t)

	it.DisableTests([]string{
		// disable tests that set values
		ContractReaderGetLatestValueBasedOnConfidenceLevel,
		// disable anything returning a struct or requiring input params for now
		ContractReaderGetLatestValueAsValuesDotValue,
		ContractReaderGetLatestValue,
		ContractReaderGetLatestValueWithModifiersUsingOwnMapstrctureOverrides,
		// events not yet supported
		ContractReaderGetLatestValueGetsLatestForEvent,
		ContractReaderGetLatestValueBasedOnConfidenceLevelForEvent,
		ContractReaderGetLatestValueReturnsNotFoundWhenNotTriggeredForEvent,
		ContractReaderGetLatestValueWithFilteringForEvent,
		// disable anything in batch relating to input params or structs for now
		ContractReaderBatchGetLatestValue,
		ContractReaderBatchGetLatestValueWithModifiersOwnMapstructureOverride,
		ContractReaderBatchGetLatestValueDifferentParamsResultsRetainOrder,
		ContractReaderBatchGetLatestValueDifferentParamsResultsRetainOrderMultipleContracts,
		ContractReaderBatchGetLatestValueSetsErrorsProperly,
		// query key not implemented yet
		ContractReaderQueryKeyNotFound,
		ContractReaderQueryKeyReturnsData,
		ContractReaderQueryKeyReturnsDataAsValuesDotValue,
		ContractReaderQueryKeyCanFilterWithValueComparator,
		ContractReaderQueryKeyCanLimitResultsWithCursor,
		ContractReaderQueryKeysReturnsDataTwoEventTypes,
		ContractReaderQueryKeysNotFound,
		ContractReaderQueryKeysReturnsData,
		ContractReaderQueryKeysReturnsDataAsValuesDotValue,
		ContractReaderQueryKeysCanFilterWithValueComparator,
		ContractReaderQueryKeysCanLimitResultsWithCursor,
	})

	RunChainComponentsSolanaTests(t, it)
	RunChainComponentsInLoopSolanaTests(t, commontestutils.WrapContractReaderTesterForLoop(it))
}

func RunChainComponentsSolanaTests[T TestingT[T]](t T, it *SolanaChainComponentsInterfaceTester[T]) {
	RunContractReaderSolanaTests(t, it)
	// Add ChainWriter tests here
}

func RunChainComponentsInLoopSolanaTests[T TestingT[T]](t T, it ChainComponentsInterfaceTester[T]) {
	RunContractReaderInLoopTests(t, it)
	// Add ChainWriter tests here
}

func RunContractReaderSolanaTests[T TestingT[T]](t T, it *SolanaChainComponentsInterfaceTester[T]) {
	RunContractReaderInterfaceTests(t, it, false)

	testCases := []Testcase[T]{}

	RunTests(t, it, testCases)
}

func RunContractReaderInLoopTests[T TestingT[T]](t T, it ChainComponentsInterfaceTester[T]) {
	RunContractReaderInterfaceTests(t, it, false)

	testCases := []Testcase[T]{}

	RunTests(t, it, testCases)
}

type SolanaChainComponentsInterfaceTesterHelper[T TestingT[T]] interface {
	Init(t T)
	RPCClient() *chainreader.RPCClientWrapper
	Context(t T) context.Context
	Logger(t T) logger.Logger
	GetJSONEncodedIDL(t T) []byte
	CreateAccount(t T, value uint64) solana.PublicKey
}

type SolanaChainComponentsInterfaceTester[T TestingT[T]] struct {
	TestSelectionSupport
	Helper              SolanaChainComponentsInterfaceTesterHelper[T]
	cr                  *chainreader.SolanaChainReaderService
	chainReaderConfig   config.ChainReader
	accountPubKey       solana.PublicKey
	secondAccountPubKey solana.PublicKey
}

func (it *SolanaChainComponentsInterfaceTester[T]) Setup(t T) {
	t.Cleanup(func() {})

	it.chainReaderConfig = config.ChainReader{
		Namespaces: map[string]config.ChainReaderMethods{
			AnyContractName: {
				Methods: map[string]config.ChainDataReader{
					MethodReturningUint64: {
						AnchorIDL: string(it.Helper.GetJSONEncodedIDL(t)),
						Encoding:  config.EncodingTypeBorsh,
						Procedure: config.ChainReaderProcedure{
							IDLAccount: "DataAccount",
							OutputModifications: codec.ModifiersConfig{
								&codec.PropertyExtractorConfig{FieldName: "U64Value"},
							},
						},
					},
					MethodReturningUint64Slice: {
						AnchorIDL: string(it.Helper.GetJSONEncodedIDL(t)),
						Encoding:  config.EncodingTypeBorsh,
						Procedure: config.ChainReaderProcedure{
							IDLAccount: "DataAccount",
							OutputModifications: codec.ModifiersConfig{
								&codec.PropertyExtractorConfig{FieldName: "U64Slice"},
							},
						},
					},
				},
			},
			AnySecondContractName: {
				Methods: map[string]config.ChainDataReader{
					MethodReturningUint64: {
						AnchorIDL: string(it.Helper.GetJSONEncodedIDL(t)),
						Encoding:  config.EncodingTypeBorsh,
						Procedure: config.ChainReaderProcedure{
							IDLAccount: "DataAccount",
							OutputModifications: codec.ModifiersConfig{
								&codec.PropertyExtractorConfig{FieldName: "U64Value"},
							},
						},
					},
				},
			},
		},
	}

	it.accountPubKey = it.Helper.CreateAccount(t, AnyValueToReadWithoutAnArgument)
	it.secondAccountPubKey = it.Helper.CreateAccount(t, AnyDifferentValueToReadWithoutAnArgument)
}

func (it *SolanaChainComponentsInterfaceTester[T]) Name() string {
	return ""
}

func (it *SolanaChainComponentsInterfaceTester[T]) GetAccountBytes(i int) []byte {
	return nil
}

func (it *SolanaChainComponentsInterfaceTester[T]) GetAccountString(i int) string {
	return ""
}

func (it *SolanaChainComponentsInterfaceTester[T]) GetContractReader(t T) types.ContractReader {
	ctx := it.Helper.Context(t)
	if it.cr != nil {
		return it.cr
	}

	svc, err := chainreader.NewChainReaderService(it.Helper.Logger(t), it.Helper.RPCClient(), it.chainReaderConfig)

	require.NoError(t, err)
	require.NoError(t, svc.Start(ctx))

	it.cr = svc

	return svc
}

func (it *SolanaChainComponentsInterfaceTester[T]) GetContractWriter(t T) types.ContractWriter {
	return nil
}

func (it *SolanaChainComponentsInterfaceTester[T]) GetBindings(t T) []types.BoundContract {
	// at the moment, use only a single account address for everything
	return []types.BoundContract{
		{Name: AnyContractName, Address: it.accountPubKey.String()},
		{Name: AnySecondContractName, Address: it.secondAccountPubKey.String()},
	}
}

func (it *SolanaChainComponentsInterfaceTester[T]) DirtyContracts() {}

func (it *SolanaChainComponentsInterfaceTester[T]) MaxWaitTimeForEvents() time.Duration {
	return time.Second
}

func (it *SolanaChainComponentsInterfaceTester[T]) GenerateBlocksTillConfidenceLevel(t T, contractName, readName string, confidenceLevel primitives.ConfidenceLevel) {

}

func (it *SolanaChainComponentsInterfaceTester[T]) Init(t T) {
	it.Helper.Init(t)
}

type helper struct {
	programID solana.PublicKey
	rpcURL    string
	wsURL     string
	rpcClient *rpc.Client
	wsClient  *ws.Client
	idlBts    []byte
	nonce     uint64
}

func (h *helper) Init(t *testing.T) {
	t.Helper()

	privateKey, err := solana.PrivateKeyFromBase58(solclient.DefaultPrivateKeysSolValidator[1])
	require.NoError(t, err)

	h.rpcURL, h.wsURL = setupTestValidator(t, privateKey.PublicKey().String())
	h.wsClient, err = ws.Connect(tests.Context(t), h.wsURL)
	h.rpcClient = rpc.New(h.rpcURL)

	require.NoError(t, err)

	client.FundTestAccounts(t, []solana.PublicKey{privateKey.PublicKey()}, h.rpcURL)

	pubkey, err := solana.PublicKeyFromBase58(programPubKey)
	require.NoError(t, err)

	contract.SetProgramID(pubkey)
	h.programID = pubkey
}

func (h *helper) RPCClient() *chainreader.RPCClientWrapper {
	return &chainreader.RPCClientWrapper{Client: h.rpcClient}
}

func (h *helper) Context(t *testing.T) context.Context {
	return tests.Context(t)
}

func (h *helper) Logger(t *testing.T) logger.Logger {
	return logger.Test(t)
}

func (h *helper) GetJSONEncodedIDL(t *testing.T) []byte {
	t.Helper()

	if h.idlBts != nil {
		return h.idlBts
	}

	soPath := filepath.Join(utils.IDLDir, "contract_reader_interface.json")

	_, err := os.Stat(soPath)
	if err != nil {
		t.Log(err.Error())
		t.FailNow()
	}

	bts, err := os.ReadFile(soPath)
	require.NoError(t, err)

	h.idlBts = bts

	return h.idlBts
}

func (h *helper) CreateAccount(t *testing.T, value uint64) solana.PublicKey {
	t.Helper()

	h.nonce++

	bts := make([]byte, 8)
	binary.LittleEndian.PutUint64(bts, h.nonce*value)

	pubKey, _, err := solana.FindProgramAddress([][]byte{[]byte("data"), bts}, h.programID)
	require.NoError(t, err)

	// Getting the default localnet private key
	privateKey, err := solana.PrivateKeyFromBase58(solclient.DefaultPrivateKeysSolValidator[1])
	require.NoError(t, err)

	h.runInitialize(t, value, pubKey, func(key solana.PublicKey) *solana.PrivateKey {
		return &privateKey
	}, privateKey.PublicKey())

	return pubKey
}

func (h *helper) runInitialize(
	t *testing.T,
	value uint64,
	data solana.PublicKey,
	signerFunc func(key solana.PublicKey) *solana.PrivateKey,
	payer solana.PublicKey,
) {
	t.Helper()

	inst, err := contract.NewInitializeInstruction(h.nonce*value, value, data, payer, solana.SystemProgramID).ValidateAndBuild()
	require.NoError(t, err)

	h.sendInstruction(t, inst, signerFunc, payer)
}

func (h *helper) sendInstruction(
	t *testing.T,
	inst *contract.Instruction,
	signerFunc func(key solana.PublicKey) *solana.PrivateKey,
	payer solana.PublicKey,
) {
	t.Helper()

	ctx := tests.Context(t)

	recent, err := h.rpcClient.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
	require.NoError(t, err)

	tx, err := solana.NewTransaction(
		[]solana.Instruction{
			inst,
		},
		recent.Value.Blockhash,
		solana.TransactionPayer(payer),
	)
	require.NoError(t, err)

	_, err = tx.EncodeTree(text.NewTreeEncoder(io.Discard, "Initialize"))
	require.NoError(t, err)

	_, err = tx.Sign(signerFunc)
	require.NoError(t, err)

	sig, err := h.rpcClient.SendTransactionWithOpts(
		ctx, tx,
		rpc.TransactionOpts{
			PreflightCommitment: rpc.CommitmentConfirmed,
		},
	)
	require.NoError(t, err)

	h.waitForTX(t, sig, rpc.CommitmentFinalized)
}

func (h *helper) waitForTX(t *testing.T, sig solana.Signature, commitment rpc.CommitmentType) {
	t.Helper()

	sub, err := h.wsClient.SignatureSubscribe(
		sig,
		commitment,
	)
	require.NoError(t, err)

	defer sub.Unsubscribe()

	res, err := sub.Recv()
	require.NoError(t, err)

	if res.Value.Err != nil {
		t.Logf("transaction confirmation failed: %v", res.Value.Err)
		t.FailNow()
	}
}

const programPubKey = "6AfuXF6HapDUhQfE4nQG9C1SGtA1YjP3icaJyRfU4RyE"

// upgradeAuthority is admin solana.PrivateKey as string
func setupTestValidator(t *testing.T, upgradeAuthority string) (string, string) {
	t.Helper()

	soPath := filepath.Join(utils.ContractsDir, "contract_reader_interface.so")

	_, err := os.Stat(soPath)
	if err != nil {
		t.Log(err.Error())
		t.FailNow()
	}

	flags := []string{
		"--warp-slot", "42",
		"--upgradeable-program",
		programPubKey,
		soPath,
		upgradeAuthority,
	}

	return client.SetupLocalSolNodeWithFlags(t, flags...)
}
