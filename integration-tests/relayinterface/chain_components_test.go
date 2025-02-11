/*
Package relayinterface contains the interface tests for chain components.
*/
package relayinterface

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/ws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	commonconfig "github.com/smartcontractkit/chainlink-common/pkg/config"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	commontestutils "github.com/smartcontractkit/chainlink-common/pkg/loop/testutils"
	"github.com/smartcontractkit/chainlink-common/pkg/services/servicetest"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	. "github.com/smartcontractkit/chainlink-common/pkg/types/interfacetests" //nolint common practice to import test mods with .
	"github.com/smartcontractkit/chainlink-common/pkg/types/query/primitives"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	contractprimary "github.com/smartcontractkit/chainlink-solana/contracts/generated/contract_reader_interface"
	contractsecondary "github.com/smartcontractkit/chainlink-solana/contracts/generated/contract_reader_interface_secondary"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/solclient"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/utils"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainreader"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainwriter"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/txm"
	keyMocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/mocks"
	solanautils "github.com/smartcontractkit/chainlink-solana/pkg/solana/utils"
)

const (
	AnyContractNameWithSharedAddress1 = AnyContractName + "Shared1"
	AnyContractNameWithSharedAddress2 = AnyContractName + "Shared2"
	AnyContractNameWithSharedAddress3 = AnyContractName + "Shared3"
)

func TestChainComponents(t *testing.T) {
	t.Parallel()

	t.Run("RunChainComponentsSolanaTests", func(t *testing.T) {
		t.Parallel()
		helper := &helper{}
		helper.Init(t)
		it := &SolanaChainComponentsInterfaceTester[*testing.T]{Helper: helper, testContext: make(map[string]uint64), testContextMu: &sync.RWMutex{}, testIdx: &atomic.Uint64{}}
		DisableTests(it)
		it.Setup(t)
		RunChainComponentsSolanaTests(t, it)
	})

	t.Run("RunChainComponentsInLoopSolanaTests", func(t *testing.T) {
		t.Parallel()
		helper := &helper{}
		helper.Init(t)
		it := &SolanaChainComponentsInterfaceTester[*testing.T]{Helper: helper, testContext: make(map[string]uint64), testContextMu: &sync.RWMutex{}, testIdx: &atomic.Uint64{}}
		DisableTests(it)
		wrapped := commontestutils.WrapContractReaderTesterForLoop(it)
		wrapped.Setup(t)
		RunChainComponentsInLoopSolanaTests(t, wrapped)
	})
}

func DisableTests(it *SolanaChainComponentsInterfaceTester[*testing.T]) {
	it.DisableTests([]string{
		// solana is a no-op on confidence level
		ContractReaderGetLatestValueBasedOnConfidenceLevel,
		// disable failing tests
		ContractReaderBatchGetLatestValueSetsErrorsProperly,
		ContractReaderGetLatestValue,
		ContractReaderGetLatestValueAsValuesDotValue,
		ContractReaderBatchGetLatestValue,
		ContractReaderBatchGetLatestValueDifferentParamsResultsRetainOrder,
		ContractReaderBatchGetLatestValueDifferentParamsResultsRetainOrderMultipleContracts,

		// events not yet supported
		ContractReaderGetLatestValueGetsLatestForEvent,
		ContractReaderGetLatestValueBasedOnConfidenceLevelForEvent,
		ContractReaderGetLatestValueReturnsNotFoundWhenNotTriggeredForEvent,
		ContractReaderGetLatestValueWithFilteringForEvent,
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
}

func RunChainComponentsSolanaTests[T WrappedTestingT[T]](t T, it *SolanaChainComponentsInterfaceTester[T]) {
	testCases := Testcase[T]{
		Name: "Test address groups where first namespace shares address with second namespace",
		Test: func(t T) {
			ctx := tests.Context(t)
			cfg := it.buildContractReaderConfig(t)
			cfg.AddressShareGroups = [][]string{{AnyContractNameWithSharedAddress1, AnyContractNameWithSharedAddress2, AnyContractNameWithSharedAddress3}}
			cr := it.GetContractReaderWithCustomCfg(t, cfg)

			t.Run("Namespace is part of an address share group that doesn't have a registered address and provides no address during Bind", func(t T) {
				bound1 := []types.BoundContract{{
					Name: AnyContractNameWithSharedAddress1,
				}}
				require.Error(t, cr.Bind(ctx, bound1))
			})

			addressToBeShared := it.Helper.CreateAccount(t, *it, AnyContractName, AnyValueToReadWithoutAnArgument, CreateTestStruct(0, it)).String()
			t.Run("Namespace is part of an address share group that doesn't have a registered address and provides an address during Bind", func(t T) {
				bound1 := []types.BoundContract{{Name: AnyContractNameWithSharedAddress1, Address: addressToBeShared}}

				require.NoError(t, cr.Bind(ctx, bound1))

				var prim uint64
				require.NoError(t, cr.GetLatestValue(ctx, bound1[0].ReadIdentifier(MethodReturningUint64), primitives.Unconfirmed, nil, &prim))
				assert.Equal(t, AnyValueToReadWithoutAnArgument, prim)
			})

			t.Run("Namespace is part of an address share group that has a registered address and provides that same address during Bind", func(t T) {
				bound2 := []types.BoundContract{{
					Name:    AnyContractNameWithSharedAddress2,
					Address: addressToBeShared}}
				require.NoError(t, cr.Bind(ctx, bound2))

				var prim uint64
				require.NoError(t, cr.GetLatestValue(ctx, bound2[0].ReadIdentifier(MethodReturningUint64), primitives.Unconfirmed, nil, &prim))
				assert.Equal(t, AnyValueToReadWithoutAnArgument, prim)
				assert.Equal(t, addressToBeShared, bound2[0].Address)
			})

			t.Run("Namespace is part of an address share group that has a registered address and provides a wrong address during Bind", func(t T) {
				key, err := solana.NewRandomPrivateKey()
				require.NoError(t, err)

				bound2 := []types.BoundContract{{
					Name:    AnyContractNameWithSharedAddress2,
					Address: key.PublicKey().String()}}
				require.Error(t, cr.Bind(ctx, bound2))
			})

			t.Run("Namespace is part of an address share group that has a registered address and provides no address during Bind", func(t T) {
				bound3 := []types.BoundContract{{Name: AnyContractNameWithSharedAddress3}}
				require.NoError(t, cr.Bind(ctx, bound3))

				var prim uint64
				require.NoError(t, cr.GetLatestValue(ctx, bound3[0].ReadIdentifier(MethodReturningUint64), primitives.Unconfirmed, nil, &prim))
				assert.Equal(t, AnyValueToReadWithoutAnArgument, prim)
				assert.Equal(t, addressToBeShared, bound3[0].Address)

				// when run in a loop Bind address won't be set, so check if CR Method works without set address.
				prim = 0
				require.NoError(t, cr.GetLatestValue(ctx, types.BoundContract{
					Address: "",
					Name:    AnyContractNameWithSharedAddress3,
				}.ReadIdentifier(MethodReturningUint64), primitives.Unconfirmed, nil, &prim))
				assert.Equal(t, AnyValueToReadWithoutAnArgument, prim)
			})

			t.Run("Namespace is not part of an address share group that has a registered address and provides no address during Bind", func(t T) {
				require.Error(t, cr.Bind(ctx, []types.BoundContract{{Name: AnyContractName}}))
			})
		},
	}

	RunTests(t, it, []Testcase[T]{testCases})
	RunContractReaderTests(t, it)
	// Add ChainWriter tests here
}

func RunChainComponentsInLoopSolanaTests[T WrappedTestingT[T]](t T, it ChainComponentsInterfaceTester[T]) {
	RunContractReaderInLoopTests(t, it)
	// Add ChainWriter tests here
}

func RunContractReaderTests[T WrappedTestingT[T]](t T, it *SolanaChainComponentsInterfaceTester[T]) {
	RunContractReaderInterfaceTests(t, it, false, true)
}

// GetLatestValue method
const (
	ContractReaderGetLatestValueUsingMultiReader = "Get latest value using multi reader"
	ContractReaderGetLatestValueGetTokenPrices   = "Get latest value handles get token prices edge case"
)

type TimestampedUnixBig struct {
	Value     *big.Int `json:"value"`
	Timestamp uint32   `json:"timestamp"`
}

func RunContractReaderInLoopTests[T WrappedTestingT[T]](t T, it ChainComponentsInterfaceTester[T]) {
	//RunContractReaderInterfaceTests(t, it, false, true)
	testCases := []Testcase[T]{
		{
			Name: ContractReaderGetLatestValueUsingMultiReader,
			Test: func(t T) {
				cr := it.GetContractReader(t)
				bindings := it.GetBindings(t)
				ctx := tests.Context(t)

				bound := BindingsByName(bindings, AnyContractName)[0]

				require.NoError(t, cr.Bind(ctx, bindings))

				type MultiReadResult struct {
					A uint8
					B int16
					U string
					V bool
				}

				mRR := MultiReadResult{}
				require.NoError(t, cr.GetLatestValue(ctx, bound.ReadIdentifier(MultiRead), primitives.Unconfirmed, nil, &mRR))

				expectedMRR := MultiReadResult{A: 1, B: 2, U: "Hello", V: true}
				require.Equal(t, expectedMRR, mRR)
			},
		},
		{
			Name: ContractReaderGetLatestValueGetTokenPrices,
			Test: func(t T) {
				cr := it.GetContractReader(t)
				bindings := it.GetBindings(t)
				ctx := tests.Context(t)

				bound := BindingsByName(bindings, AnyContractName)[0]

				require.NoError(t, cr.Bind(ctx, bindings))

				type TimestampedUnixBig struct {
					Value     *big.Int `json:"value"`
					Timestamp uint32   `json:"timestamp"`
				}

				res := make([]TimestampedUnixBig, 2)

				byteTokens := make([][]byte, 0, 2)
				pubKey1, err := solana.PublicKeyFromBase58(GetTokenPricesPubKey1)
				require.NoError(t, err)
				pubKey2, err := solana.PublicKeyFromBase58(GetTokenPricesPubKey2)
				require.NoError(t, err)

				byteTokens = append(byteTokens, pubKey1.Bytes())
				byteTokens = append(byteTokens, pubKey2.Bytes())
				require.NoError(t, cr.GetLatestValue(ctx, bound.ReadIdentifier(GetTokenPrices), primitives.Unconfirmed, map[string]any{"tokens": byteTokens}, &res))
				require.Equal(t, "7048352069843304521481572571769838000081483315549204879493368331", res[0].Value.String())
				require.Equal(t, uint32(1700000001), res[0].Timestamp)
				require.Equal(t, "17980346130170174053328187512531209543631592085982266692926093439168", res[1].Value.String())
				require.Equal(t, uint32(1800000002), res[1].Timestamp)
			},
		},
	}
	RunTests(t, it, testCases)
}

type SolanaChainComponentsInterfaceTesterHelper[T WrappedTestingT[T]] interface {
	Init(t T)
	RPCClient() *chainreader.RPCClientWrapper
	Context(t T) context.Context
	Logger(t T) logger.Logger
	GetPrimaryIDL(t T) []byte
	GetSecondaryIDL(t T) []byte
	CreateAccount(t T, it SolanaChainComponentsInterfaceTester[T], contractName string, value uint64, testStruct TestStruct) solana.PublicKey
	TXM() *txm.TxManager
	SolanaClient() *client.Client
}

type WrappedTestingT[T any] interface {
	TestingT[T]
	Name() string
}

type SolanaChainComponentsInterfaceTester[T WrappedTestingT[T]] struct {
	TestSelectionSupport
	Helper        SolanaChainComponentsInterfaceTesterHelper[T]
	testContext   map[string]uint64
	testContextMu *sync.RWMutex
	testIdx       *atomic.Uint64
}

// ContractReaderConfig and ContractWriterConfig are created when GetContractReader and GetContractWriter are called, respectively,
// so that a test index can be injected as a PDA seed for each test
func (it *SolanaChainComponentsInterfaceTester[T]) Setup(t T) {
	t.Cleanup(func() {})
}

func (it *SolanaChainComponentsInterfaceTester[T]) Name() string {
	return ""
}

func (it *SolanaChainComponentsInterfaceTester[T]) GetAccountBytes(i int) []byte {
	pubKeyBytes := make([]byte, 32)
	binary.LittleEndian.PutUint64(pubKeyBytes, uint64(i))
	return solana.PublicKeyFromBytes(pubKeyBytes).Bytes()
}

func (it *SolanaChainComponentsInterfaceTester[T]) GetAccountString(i int) string {
	pubKeyBytes := make([]byte, 32)
	binary.LittleEndian.PutUint64(pubKeyBytes, uint64(i))
	return solana.PublicKeyFromBytes(pubKeyBytes).String()
}

func (it *SolanaChainComponentsInterfaceTester[T]) GetContractReader(t T) types.ContractReader {
	contractReaderConfig := it.buildContractReaderConfig(t)
	var events chainreader.EventsReader

	svc, err := chainreader.NewContractReaderService(
		it.Helper.Logger(t),
		it.Helper.RPCClient(),
		contractReaderConfig,
		events)

	require.NoError(t, err)
	servicetest.Run(t, svc)

	return svc
}

func (it *SolanaChainComponentsInterfaceTester[T]) GetContractReaderWithCustomCfg(t T, contractReaderConfig config.ContractReader) types.ContractReader {
	ctx := it.Helper.Context(t)
	var events chainreader.EventsReader

	svc, err := chainreader.NewContractReaderService(
		it.Helper.Logger(t),
		it.Helper.RPCClient(),
		contractReaderConfig,
		events)

	require.NoError(t, err)
	require.NoError(t, svc.Start(ctx))

	return svc
}

func (it *SolanaChainComponentsInterfaceTester[T]) GetContractWriter(t T) types.ContractWriter {
	chainWriterConfig := it.buildContractWriterConfig(t)
	cw, err := chainwriter.NewSolanaChainWriterService(it.Helper.Logger(t), it.Helper.SolanaClient(), *it.Helper.TXM(), nil, chainWriterConfig)
	require.NoError(t, err)

	servicetest.Run(t, cw)
	return cw
}

func (it *SolanaChainComponentsInterfaceTester[T]) getTestIdx(name string) uint64 {
	it.testContextMu.Lock()
	defer it.testContextMu.Unlock()
	idx, exists := it.testContext[name]
	if !exists {
		idx = it.testIdx.Add(1)    // new index is needed so increment the existing
		it.testContext[name] = idx // set new index in map
	}
	return idx
}

func (it *SolanaChainComponentsInterfaceTester[T]) GetBindings(t T) []types.BoundContract {
	// Create a new account with fresh state for each test
	testStruct := CreateTestStruct(0, it)
	return []types.BoundContract{
		{Name: AnyContractName, Address: it.Helper.CreateAccount(t, *it, AnyContractName, AnyValueToReadWithoutAnArgument, testStruct).String()},
		{Name: AnySecondContractName, Address: it.Helper.CreateAccount(t, *it, AnySecondContractName, AnyDifferentValueToReadWithoutAnArgument, testStruct).String()},
	}
}

func (it *SolanaChainComponentsInterfaceTester[T]) DirtyContracts() {}

func (it *SolanaChainComponentsInterfaceTester[T]) MaxWaitTimeForEvents() time.Duration {
	return time.Second
}

func (it *SolanaChainComponentsInterfaceTester[T]) GenerateBlocksTillConfidenceLevel(t T, contractName, readName string, confidenceLevel primitives.ConfidenceLevel) {

}

type helper struct {
	initOnce           sync.Once
	primaryProgramID   solana.PublicKey
	secondaryProgramID solana.PublicKey
	rpcURL             string
	wsURL              string
	rpcClient          *rpc.Client
	wsClient           *ws.Client
	primaryIdlBts      []byte
	secondaryIdlBts    []byte
	txm                txm.TxManager
	sc                 *client.Client
}

func (h *helper) Init(t *testing.T) {
	t.Helper()

	privateKey, err := solana.PrivateKeyFromBase58(solclient.DefaultPrivateKeysSolValidator[1])
	require.NoError(t, err)

	h.rpcURL, h.wsURL = utils.SetupTestValidatorWithAnchorPrograms(t, privateKey.PublicKey().String(), []string{"contract-reader-interface", "contract-reader-interface-secondary"})
	h.wsClient, err = ws.Connect(tests.Context(t), h.wsURL)
	h.rpcClient = rpc.New(h.rpcURL)

	require.NoError(t, err)

	utils.FundAccounts(t, []solana.PrivateKey{privateKey}, h.rpcClient)

	cfg := config.NewDefault()
	cfg.Chain.TxRetentionTimeout = commonconfig.MustNewDuration(10 * time.Minute)
	solanaClient, err := client.NewClient(h.rpcURL, cfg, 5*time.Second, nil)
	require.NoError(t, err)

	h.sc = solanaClient

	loader := solanautils.NewLoader[client.ReaderWriter](func(ctx context.Context) (client.ReaderWriter, error) { return solanaClient, nil })
	mkey := keyMocks.NewSimpleKeystore(t)
	mkey.On("Sign", mock.Anything, privateKey.PublicKey().String(), mock.Anything).Return(func(_ context.Context, _ string, data []byte) []byte {
		sig, _ := privateKey.Sign(data)
		return sig[:]
	}, nil)
	lggr := logger.Test(t)

	txm := txm.NewTxm("localnet", loader, nil, cfg, mkey, lggr)
	err = txm.Start(tests.Context(t))
	require.NoError(t, err)

	h.txm = txm

	primaryPubkey, err := solana.PublicKeyFromBase58(primaryProgramPubKey)
	require.NoError(t, err)
	contractprimary.SetProgramID(primaryPubkey)

	secondaryPubkey, err := solana.PublicKeyFromBase58(secondaryProgramPubKey)
	require.NoError(t, err)
	contractsecondary.SetProgramID(secondaryPubkey)

	h.primaryProgramID = primaryPubkey
	h.secondaryProgramID = secondaryPubkey
}

func (h *helper) RPCClient() *chainreader.RPCClientWrapper {
	return &chainreader.RPCClientWrapper{AccountReader: h.rpcClient}
}

func (h *helper) TXM() *txm.TxManager {
	return &h.txm
}

func (h *helper) SolanaClient() *client.Client {
	return h.sc
}

func (h *helper) Context(t *testing.T) context.Context {
	return tests.Context(t)
}

func (h *helper) Logger(t *testing.T) logger.Logger {
	return logger.Test(t)
}

func (h *helper) GetPrimaryIDL(t *testing.T) []byte {
	t.Helper()

	if h.primaryIdlBts != nil {
		return h.primaryIdlBts
	}

	bts := h.GetJSONEncodedIDL(t, "contract_reader_interface.json")
	h.primaryIdlBts = bts
	return h.primaryIdlBts
}

func (h *helper) GetSecondaryIDL(t *testing.T) []byte {
	t.Helper()

	if h.secondaryIdlBts != nil {
		return h.secondaryIdlBts
	}

	bts := h.GetJSONEncodedIDL(t, "contract_reader_interface_secondary.json")
	h.secondaryIdlBts = bts
	return h.secondaryIdlBts
}

func (h *helper) GetJSONEncodedIDL(t *testing.T, fileName string) []byte {
	t.Helper()

	soPath := filepath.Join(utils.IDLDir, fileName)

	_, err := os.Stat(soPath)
	if err != nil {
		t.Log(err.Error())
		t.FailNow()
	}

	bts, err := os.ReadFile(soPath)
	require.NoError(t, err)

	return bts
}

func (h *helper) CreateAccount(t *testing.T, it SolanaChainComponentsInterfaceTester[*testing.T], contractName string, value uint64, testStruct TestStruct) solana.PublicKey {
	t.Helper()

	var programID solana.PublicKey
	switch contractName {
	case AnyContractName:
		programID = h.primaryProgramID
		h.initOnce.Do(func() {
			cw := it.GetContractWriter(t)
			SubmitTransactionToCW(t, &it, cw, "initializeMultiRead", nil, types.BoundContract{Name: contractName, Address: programID.String()}, types.Finalized)
			SubmitTransactionToCW(t, &it, cw, "initializeTokenPrices", nil, types.BoundContract{Name: contractName, Address: programID.String()}, types.Finalized)
		})
	case AnySecondContractName:
		programID = h.secondaryProgramID
	}

	h.runInitialize(t, it, contractName, programID, value, testStruct)
	return programID
}

type InitializeArgs struct {
	TestIdx uint64
	Value   uint64
}

type StoreStructArgs struct {
	TestIdx uint64
	Data    TestStruct
}

func (h *helper) runInitialize(
	t *testing.T,
	it SolanaChainComponentsInterfaceTester[*testing.T],
	contractName string,
	programID solana.PublicKey,
	value uint64,
	testStruct TestStruct,
) {
	t.Helper()

	cw := it.GetContractWriter(t)

	// Fetch test index from map
	it.testContextMu.RLock()
	defer it.testContextMu.RUnlock()
	testIdx, exists := it.testContext[t.Name()]
	if !exists {
		return
	}

	initArgs := InitializeArgs{
		TestIdx: testIdx,
		Value:   value,
	}
	SubmitTransactionToCW(t, &it, cw, "initialize", initArgs, types.BoundContract{Name: contractName, Address: programID.String()}, types.Finalized)

	storeStructArgs := StoreStructArgs{
		TestIdx: testIdx,
		Data:    testStruct,
	}
	SubmitTransactionToCW(t, &it, cw, MethodSettingStruct, storeStructArgs, types.BoundContract{Name: contractName, Address: programID.String()}, types.Finalized)
}

const (
	MultiRead      = "MultiRead"
	GetTokenPrices = "GetTokenPrices"
)

func (it *SolanaChainComponentsInterfaceTester[T]) buildContractReaderConfig(t T) config.ContractReader {
	idx := it.getTestIdx(t.Name())
	pdaDataPrefix := []byte("data")
	pdaDataPrefix = binary.LittleEndian.AppendUint64(pdaDataPrefix, idx)
	pdaStructDataPrefix := []byte("struct_data")
	pdaStructDataPrefix = binary.LittleEndian.AppendUint64(pdaStructDataPrefix, idx)
	testStruct := CreateTestStruct(0, it)
	uint64ReadDef := config.ReadDefinition{
		ChainSpecificName: "DataAccount",
		ReadType:          config.Account,
		PDADefinition: codec.PDATypeDef{
			Prefix: pdaDataPrefix,
		},
		OutputModifications: commoncodec.ModifiersConfig{
			&commoncodec.PropertyExtractorConfig{FieldName: "U64Value"},
		},
	}
	basicContractDef := config.ChainContractReader{
		IDL: mustUnmarshalIDL(t, string(it.Helper.GetPrimaryIDL(t))),
		Reads: map[string]config.ReadDefinition{
			MethodReturningUint64: uint64ReadDef,
		},
	}
	return config.ContractReader{
		Namespaces: map[string]config.ChainContractReader{
			AnyContractName: {
				IDL: mustUnmarshalIDL(t, string(it.Helper.GetPrimaryIDL(t))),
				Reads: map[string]config.ReadDefinition{
					GetTokenPrices: {
						ChainSpecificName: "BillingTokenConfigWrapper",
						PDADefinition: codec.PDATypeDef{
							Prefix: []byte("fee_billing_token_config"),
							Seeds: []codec.PDASeed{
								{
									Name: "Tokens",
									Type: codec.IdlType{
										AsIdlTypeVec: &codec.IdlTypeVec{
											Vec: codec.IdlType{AsString: codec.IdlTypePublicKey},
										},
									},
								},
							},
						},
						OutputModifications: commoncodec.ModifiersConfig{
							&commoncodec.DropModifierConfig{
								Fields: []string{"Config"},
							},
							&commoncodec.HardCodeModifierConfig{
								OffChainValues: map[string]any{
									"Response": make([]TimestampedUnixBig, 1000),
								},
							},
							&commoncodec.PropertyExtractorConfig{FieldName: "Response"},
						},
						ReadType: config.Account,
					},
					MultiRead: {
						ChainSpecificName: "MultiRead1",
						PDADefinition: codec.PDATypeDef{
							Prefix: []byte("multi_read1"),
						},
						OutputModifications: commoncodec.ModifiersConfig{
							&commoncodec.HardCodeModifierConfig{
								OffChainValues: map[string]any{"U": "", "V": false},
							},
						},
						MultiReader: &config.MultiReader{Reads: []config.ReadDefinition{
							{
								ChainSpecificName: "MultiRead2",
								PDADefinition:     codec.PDATypeDef{Prefix: []byte("multi_read2")},
								ReadType:          config.Account,
							},
						}},
						ReadType: config.Account,
					},
					MethodReturningUint64: uint64ReadDef,
					MethodReturningUint64Slice: {
						ChainSpecificName: "DataAccount",
						ReadType:          config.Account,
						PDADefinition: codec.PDATypeDef{
							Prefix: pdaDataPrefix,
						},
						OutputModifications: commoncodec.ModifiersConfig{
							&commoncodec.PropertyExtractorConfig{FieldName: "U64Slice"},
						},
					},
					MethodSettingUint64: {
						ChainSpecificName: "DataAccount",
						ReadType:          config.Account,
						PDADefinition: codec.PDATypeDef{
							Prefix: pdaDataPrefix,
						},
						OutputModifications: commoncodec.ModifiersConfig{
							&commoncodec.PropertyExtractorConfig{FieldName: "U64Value"},
						},
					},
					MethodReturningSeenStruct: {
						ChainSpecificName: "TestStruct",
						ReadType:          config.Account,
						PDADefinition: codec.PDATypeDef{
							Prefix: pdaStructDataPrefix,
						},
						OutputModifications: commoncodec.ModifiersConfig{
							&commoncodec.HardCodeModifierConfig{
								OnChainValues: map[string]any{
									"DifferentField":              copy(make([]byte, 32), []byte(testStruct.DifferentField)),
									"NestedDynamicStruct.Inner.S": copy(make([]byte, 32), []byte(testStruct.NestedDynamicStruct.Inner.S)),
								},
								OffChainValues: map[string]any{
									"ExtraField":                  AnyExtraValue,
									"DifferentField":              testStruct.DifferentField,
									"NestedDynamicStruct.Inner.S": testStruct.NestedDynamicStruct.Inner.S,
								},
							},
							&commoncodec.AddressBytesToStringModifierConfig{
								Fields: []string{"AccountStruct.AccountStr"},
							},
						},
					},
					MethodTakingLatestParamsReturningTestStruct: {
						ChainSpecificName: "TestStruct",
						PDADefinition: codec.PDATypeDef{
							Prefix: pdaStructDataPrefix,
						},
						OutputModifications: commoncodec.ModifiersConfig{
							&commoncodec.HardCodeModifierConfig{
								OnChainValues: map[string]any{
									"DifferentField":              copy(make([]byte, 32), []byte(testStruct.DifferentField)),
									"NestedDynamicStruct.Inner.S": copy(make([]byte, 32), []byte(testStruct.NestedDynamicStruct.Inner.S)),
								},
								OffChainValues: map[string]any{
									"ExtraField":                  AnyExtraValue,
									"DifferentField":              testStruct.DifferentField,
									"NestedDynamicStruct.Inner.S": testStruct.NestedDynamicStruct.Inner.S,
								},
							},
							&commoncodec.AddressBytesToStringModifierConfig{
								Fields: []string{"AccountStruct.AccountStr"},
							},
						},
					},
				},
			},
			AnySecondContractName: {
				IDL: mustUnmarshalIDL(t, string(it.Helper.GetSecondaryIDL(t))),
				Reads: map[string]config.ReadDefinition{
					MethodReturningUint64: {
						ChainSpecificName: "Data",
						PDADefinition: codec.PDATypeDef{
							Prefix: pdaDataPrefix,
						},
						OutputModifications: commoncodec.ModifiersConfig{
							&commoncodec.PropertyExtractorConfig{FieldName: "U64Value"},
						},
					},
				},
			},
			// these are for testing shared address groups
			AnyContractNameWithSharedAddress1: basicContractDef,
			AnyContractNameWithSharedAddress2: basicContractDef,
			AnyContractNameWithSharedAddress3: basicContractDef,
		},
	}
}

const (
	GetTokenPricesPubKey1 = "57FUKrjY7Dywph1bqNGztvtTGWcXvk5VLNCfAXtk6jqK"
	GetTokenPricesPubKey2 = "47XyyAALxH7WeNT1DGWsPeA8veSVJaF8MHFMqBM5DkP6"
)

func (it *SolanaChainComponentsInterfaceTester[T]) buildContractWriterConfig(t T) chainwriter.ChainWriterConfig {
	idx := it.getTestIdx(t.Name())
	testIdx := binary.LittleEndian.AppendUint64([]byte{}, idx)
	fromAddress := solana.MustPrivateKeyFromBase58(solclient.DefaultPrivateKeysSolValidator[1]).PublicKey().String()
	testStruct := CreateTestStruct(0, it)
	pubKey1, err := solana.PublicKeyFromBase58(GetTokenPricesPubKey1)
	require.NoError(t, err)
	pubKey2, err := solana.PublicKeyFromBase58(GetTokenPricesPubKey2)
	require.NoError(t, err)

	return chainwriter.ChainWriterConfig{
		Programs: map[string]chainwriter.ProgramConfig{
			AnyContractName: {
				IDL: string(it.Helper.GetPrimaryIDL(t)),
				Methods: map[string]chainwriter.MethodConfig{
					"initialize": {
						FromAddress:        fromAddress,
						InputModifications: nil,
						ChainSpecificName:  "initialize",
						LookupTables:       chainwriter.LookupTables{},
						Accounts: []chainwriter.Lookup{
							chainwriter.AccountConstant{
								Name:       "Signer",
								Address:    fromAddress,
								IsSigner:   true,
								IsWritable: true,
							},
							chainwriter.PDALookups{
								Name: "Account",
								PublicKey: chainwriter.AccountConstant{
									Address: primaryProgramPubKey,
								},
								Seeds: []chainwriter.Seed{
									{Static: []byte("data")},
									{Static: testIdx},
								},
								IsWritable: true,
								IsSigner:   false,
							},
							chainwriter.AccountConstant{
								Name:       "SystemProgram",
								Address:    solana.SystemProgramID.String(),
								IsWritable: false,
								IsSigner:   false,
							},
						},
						DebugIDLocation: "",
					},
					"initializeMultiRead": {
						FromAddress:        fromAddress,
						InputModifications: nil,
						ChainSpecificName:  "initializemultiread",
						LookupTables:       chainwriter.LookupTables{},
						Accounts: []chainwriter.Lookup{
							chainwriter.AccountConstant{
								Name:       "Signer",
								Address:    fromAddress,
								IsSigner:   true,
								IsWritable: true,
							},
							chainwriter.PDALookups{
								Name: "MultiRead1",
								PublicKey: chainwriter.AccountConstant{
									Name:    "ProgramID",
									Address: primaryProgramPubKey,
								},
								Seeds: []chainwriter.Seed{
									{Static: []byte("multi_read1")},
								},
								IsWritable: true,
								IsSigner:   false,
							},
							chainwriter.PDALookups{
								Name: "MultiRead2",
								PublicKey: chainwriter.AccountConstant{
									Name:    "ProgramID",
									Address: primaryProgramPubKey,
								},
								Seeds: []chainwriter.Seed{
									{Static: []byte("multi_read2")},
								},
								IsWritable: true,
								IsSigner:   false,
							},
							chainwriter.AccountConstant{
								Name:       "SystemProgram",
								Address:    solana.SystemProgramID.String(),
								IsWritable: false,
								IsSigner:   false,
							},
						},
						DebugIDLocation: "",
					},
					"initializeTokenPrices": {
						FromAddress:        fromAddress,
						InputModifications: nil,
						ChainSpecificName:  "initializetokenprices",
						LookupTables:       chainwriter.LookupTables{},
						Accounts: []chainwriter.Lookup{
							chainwriter.AccountConstant{
								Name:       "Signer",
								Address:    fromAddress,
								IsSigner:   true,
								IsWritable: true,
							},
							chainwriter.PDALookups{
								Name: "BillingTokenConfigWrapper1",
								PublicKey: chainwriter.AccountConstant{
									Name:    "ProgramID",
									Address: primaryProgramPubKey,
								},
								Seeds: []chainwriter.Seed{
									{Static: []byte("fee_billing_token_config")},
									{Static: pubKey1.Bytes()},
								},
								IsWritable: true,
								IsSigner:   false,
							},
							chainwriter.PDALookups{
								Name: "BillingTokenConfigWrapper2",
								PublicKey: chainwriter.AccountConstant{
									Name:    "ProgramID",
									Address: primaryProgramPubKey,
								},
								Seeds: []chainwriter.Seed{
									{Static: []byte("fee_billing_token_config")},
									{Static: pubKey2.Bytes()},
								},
								IsWritable: true,
								IsSigner:   false,
							},
							chainwriter.AccountConstant{
								Name:       "SystemProgram",
								Address:    solana.SystemProgramID.String(),
								IsWritable: false,
								IsSigner:   false,
							},
						},
						DebugIDLocation: "",
					},
					MethodSettingStruct: {
						FromAddress: fromAddress,
						InputModifications: []commoncodec.ModifierConfig{
							&commoncodec.AddressBytesToStringModifierConfig{
								Fields: []string{"Data.AccountStruct.AccountStr"},
							},
							&commoncodec.HardCodeModifierConfig{
								OnChainValues: map[string]any{
									"Data.Padding0":                    []byte{},
									"Data.Padding1":                    []byte{},
									"Data.Padding2":                    []byte{},
									"Data.NestedDynamicStruct.Padding": []byte{},
									"Data.NestedStaticStruct.Padding":  []byte{},
									"Data.DifferentField":              copy(make([]byte, 32), []byte(testStruct.DifferentField)),
									"Data.NestedDynamicStruct.Inner.S": copy(make([]byte, 32), []byte(testStruct.NestedDynamicStruct.Inner.S)),
								},
								OffChainValues: map[string]any{
									"Data.DifferentField":              testStruct.DifferentField,
									"Data.NestedDynamicStruct.Inner.S": testStruct.NestedDynamicStruct.Inner.S,
								},
							},
						},
						ChainSpecificName: "store",
						LookupTables:      chainwriter.LookupTables{},
						Accounts: []chainwriter.Lookup{
							chainwriter.AccountConstant{
								Name:       "Signer",
								Address:    fromAddress,
								IsSigner:   true,
								IsWritable: true,
							},
							chainwriter.PDALookups{
								Name: "Account",
								PublicKey: chainwriter.AccountConstant{
									Name:    "ProgramID",
									Address: primaryProgramPubKey,
								},
								Seeds: []chainwriter.Seed{
									{Static: []byte("struct_data")},
									{Static: testIdx},
								},
								IsWritable: true,
								IsSigner:   false,
							},
							chainwriter.AccountConstant{
								Name:       "SystemProgram",
								Address:    solana.SystemProgramID.String(),
								IsWritable: false,
								IsSigner:   false,
							},
						},
						DebugIDLocation: "",
					},
				},
			},
			AnySecondContractName: {
				IDL: string(it.Helper.GetSecondaryIDL(t)),
				Methods: map[string]chainwriter.MethodConfig{
					"initialize": {
						FromAddress:        fromAddress,
						InputModifications: nil,
						ChainSpecificName:  "initialize",
						LookupTables:       chainwriter.LookupTables{},
						Accounts: []chainwriter.Lookup{
							chainwriter.AccountConstant{
								Name:       "Signer",
								Address:    fromAddress,
								IsSigner:   true,
								IsWritable: true,
							},
							chainwriter.PDALookups{
								Name: "Account",
								PublicKey: chainwriter.AccountConstant{
									Name:    "ProgramID",
									Address: secondaryProgramPubKey,
								},
								Seeds: []chainwriter.Seed{
									{Static: []byte("data")},
									{Static: testIdx},
								},
								IsWritable: true,
								IsSigner:   false,
							},
							chainwriter.AccountConstant{
								Name:       "SystemAccount",
								Address:    solana.SystemProgramID.String(),
								IsWritable: false,
								IsSigner:   false,
							},
						},
						DebugIDLocation: "",
					},
					MethodSettingStruct: {
						FromAddress: fromAddress,
						InputModifications: []commoncodec.ModifierConfig{
							&commoncodec.AddressBytesToStringModifierConfig{
								Fields: []string{"Data.AccountStruct.AccountStr"},
							},
							&commoncodec.HardCodeModifierConfig{
								OnChainValues: map[string]any{
									"Data.Padding0":                    []byte{},
									"Data.Padding1":                    []byte{},
									"Data.Padding2":                    []byte{},
									"Data.NestedDynamicStruct.Padding": []byte{},
									"Data.NestedStaticStruct.Padding":  []byte{},
									"Data.DifferentField":              copy(make([]byte, 32), []byte(testStruct.DifferentField)),
									"Data.NestedDynamicStruct.Inner.S": copy(make([]byte, 32), []byte(testStruct.NestedDynamicStruct.Inner.S)),
								},
								OffChainValues: map[string]any{
									"Data.DifferentField":              testStruct.DifferentField,
									"Data.NestedDynamicStruct.Inner.S": testStruct.NestedDynamicStruct.Inner.S,
								},
							},
						},
						ChainSpecificName: "store",
						LookupTables:      chainwriter.LookupTables{},
						Accounts: []chainwriter.Lookup{
							chainwriter.AccountConstant{
								Name:       "Signer",
								Address:    fromAddress,
								IsSigner:   true,
								IsWritable: true,
							},
							chainwriter.PDALookups{
								Name: "Account",
								PublicKey: chainwriter.AccountConstant{
									Name:    "ProgramID",
									Address: secondaryProgramPubKey,
								},
								Seeds: []chainwriter.Seed{
									{Static: []byte("struct_data")},
									{Static: testIdx},
								},
								IsWritable: true,
								IsSigner:   false,
							},
							chainwriter.AccountConstant{
								Name:       "SystemProgram",
								Address:    solana.SystemProgramID.String(),
								IsWritable: false,
								IsSigner:   false,
							},
						},
						DebugIDLocation: "",
					},
				},
			},
		},
	}
}

func mustUnmarshalIDL[T WrappedTestingT[T]](t T, rawIDL string) codec.IDL {
	var idl codec.IDL
	if err := json.Unmarshal([]byte(rawIDL), &idl); err != nil {
		t.Errorf("failed to unmarshal test IDL", err)
		t.FailNow()
	}

	return idl
}

const (
	primaryProgramPubKey   = "6AfuXF6HapDUhQfE4nQG9C1SGtA1YjP3icaJyRfU4RyE"
	secondaryProgramPubKey = "9SFyk8NmGYh5D612mJwUYhguCRY9cFgaS2vksrigepjf"
)
