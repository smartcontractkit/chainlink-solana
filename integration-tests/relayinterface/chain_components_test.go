/*
Package relayinterface contains the interface tests for chain components.
*/
package relayinterface

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
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
	"github.com/smartcontractkit/chainlink-common/pkg/values"

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
	SolanaContractReaderGetLatestValueAsValuesDotValue                                        = "Gets the latest value as a values.Value for Solana"
	SolanaContractReaderGetLatestValue                                                        = "Gets the latest value for Solana"
	SolanaContractReaderBatchGetLatestValue                                                   = "BatchGetLatestValues works for Solana"
	SolanaContractReaderBatchGetLatestValueDifferentParamsResultsRetainOrder                  = "BatchGetLatestValues supports same read with different params and results retain order from request for Solana"
	SolanaContractReaderBatchGetLatestValueDifferentParamsResultsRetainOrderMultipleContracts = "BatchGetLatestValues supports same read with different params and results retain order from request even with multiple contracts for Solana"
)

func TestChainComponents(t *testing.T) {
	t.Parallel()
	helper := &helper{}
	helper.Init(t)

	t.Run("RunChainComponentsSolanaTests", func(t *testing.T) {
		t.Parallel()
		it := &SolanaChainComponentsInterfaceTester[*testing.T]{Helper: helper, testContext: make(map[string]uint64), testContextMu: &sync.RWMutex{}, testIdx: &atomic.Uint64{}}
		DisableTests(it)
		it.Setup(t)
		RunChainComponentsSolanaTests(t, it)
	})

	t.Run("RunChainComponentsInLoopSolanaTests", func(t *testing.T) {
		t.Parallel()
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
		// disabling tests that required Solana specific logic. Covered in the Solana specific tests
		ContractReaderGetLatestValue,
		ContractReaderGetLatestValueAsValuesDotValue,
		ContractReaderBatchGetLatestValue,
		ContractReaderBatchGetLatestValueWithModifiersOwnMapstructureOverride,
		ContractReaderBatchGetLatestValueDifferentParamsResultsRetainOrder,
		ContractReaderBatchGetLatestValueDifferentParamsResultsRetainOrderMultipleContracts,

		// disable failing test
		ContractReaderGetLatestValueFromMultipleContractsNamesSameFunction,
		ContractReaderBatchGetLatestValueMultipleContractNamesSameFunction,
		ContractReaderBatchGetLatestValueSetsErrorsProperly,
		// disable failing tests requiring solana specific implementation
		SolanaContractReaderGetLatestValue,
		SolanaContractReaderGetLatestValueAsValuesDotValue,
		SolanaContractReaderBatchGetLatestValue,
		SolanaContractReaderBatchGetLatestValueDifferentParamsResultsRetainOrder,
		SolanaContractReaderBatchGetLatestValueDifferentParamsResultsRetainOrderMultipleContracts,

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
	RunContractReaderSolanaTests(t, it)
	// Add ChainWriter tests here
}

func RunChainComponentsInLoopSolanaTests[T WrappedTestingT[T]](t T, it ChainComponentsInterfaceTester[T]) {
	RunContractReaderInLoopTests(t, it)
	// Add ChainWriter tests here
}

func RunContractReaderSolanaTests[T WrappedTestingT[T]](t T, it *SolanaChainComponentsInterfaceTester[T]) {
	RunContractReaderInterfaceTests(t, it, false, true)

	testCases := []Testcase[T] {
		{
			Name: SolanaContractReaderGetLatestValue,
			Test: func(t T) {
				cr := it.GetContractReader(t)
				cw := it.GetContractWriter(t)
				contracts := it.GetBindings(t)
				ctx := tests.Context(t)
				firstItem := CreateTestStruct(0, it)
				testIdx := it.testContext[t.Name()]

				args1 := StoreStructArgs{
					TestIdx: testIdx,
					Data: firstItem,
				}
				_ = SubmitTransactionToCW(t, it, cw, MethodSettingStruct, args1, contracts[0], types.Unconfirmed)

				secondItem := CreateTestStruct(1, it)
				args2 := StoreStructArgs{
					TestIdx: testIdx,
					Data: secondItem,
				}
				_ = SubmitTransactionToCW(t, it, cw, MethodSettingStruct, args2, contracts[0], types.Unconfirmed)

				bound := BindingsByName(contracts, AnyContractName)[0] // minimum of one bound contract expected, otherwise panics

				require.NoError(t, cr.Bind(ctx, contracts))

				actual := &TestStruct{}
				params := &LatestParams{I: 1}
				require.NoError(t, cr.GetLatestValue(ctx, bound.ReadIdentifier(MethodTakingLatestParamsReturningTestStruct), primitives.Unconfirmed, params, actual))
				assert.Equal(t, &firstItem, actual)

				params.I = 2
				actual = &TestStruct{}
				require.NoError(t, cr.GetLatestValue(ctx, bound.ReadIdentifier(MethodTakingLatestParamsReturningTestStruct), primitives.Unconfirmed, params, actual))
				assert.Equal(t, &secondItem, actual)
			},
		},
		{
			Name: SolanaContractReaderGetLatestValueAsValuesDotValue,
			Test: func(t T) {
				cr := it.GetContractReader(t)
				cw := it.GetContractWriter(t)
				contracts := it.GetBindings(t)
				ctx := tests.Context(t)
				firstItem := CreateTestStruct(0, it)
				testIdx := it.testContext[t.Name()]
				args1 := StoreStructArgs{
					TestIdx: testIdx,
					Data: firstItem,
				}
				_ = SubmitTransactionToCW(t, it, cw, MethodSettingStruct, args1, contracts[0], types.Unconfirmed)

				secondItem := CreateTestStruct(1, it)
				args2 := StoreStructArgs{
					TestIdx: testIdx,
					Data: secondItem,
				}
				_ = SubmitTransactionToCW(t, it, cw, MethodSettingStruct, args2, contracts[0], types.Unconfirmed)

				bound := BindingsByName(contracts, AnyContractName)[0] // minimum of one bound contract expected, otherwise panics

				require.NoError(t, cr.Bind(ctx, contracts))

				params := &LatestParams{I: 1}
				var value values.Value

				err := cr.GetLatestValue(ctx, bound.ReadIdentifier(MethodTakingLatestParamsReturningTestStruct), primitives.Unconfirmed, params, &value)
				require.NoError(t, err)

				actual := TestStruct{}
				err = value.UnwrapTo(&actual)
				require.NoError(t, err)
				assert.Equal(t, &firstItem, &actual)

				params = &LatestParams{I: 2}
				err = cr.GetLatestValue(ctx, bound.ReadIdentifier(MethodTakingLatestParamsReturningTestStruct), primitives.Unconfirmed, params, &value)
				require.NoError(t, err)

				actual = TestStruct{}
				err = value.UnwrapTo(&actual)
				require.NoError(t, err)
				assert.Equal(t, &secondItem, &actual)
			},
		},
		{
			Name: SolanaContractReaderBatchGetLatestValue,
			Test: func(t T) {
				cr := it.GetContractReader(t)
				cw := it.GetContractWriter(t)
				bindings := it.GetBindings(t)
				// setup test data
				firstItem := CreateTestStruct(1, it)
				testIdx := it.testContext[t.Name()]
				args := StoreStructArgs{
					TestIdx: testIdx,
					Data: firstItem,
				}
				bound := BindingsByName(bindings, AnyContractName)[0]

				batchCallEntry := make(BatchCallEntry)
				batchCallEntry[bound] = ContractBatchEntry{{Name: MethodTakingLatestParamsReturningTestStruct, ReturnValue: &args}}
				batchContractWrite(t, it, cw, bindings, batchCallEntry)

				// setup call data
				params, actual := &LatestParams{I: 1}, &TestStruct{}
				batchGetLatestValueRequest := make(types.BatchGetLatestValuesRequest)
				batchGetLatestValueRequest[bound] = []types.BatchRead{
					{
						ReadName:  MethodTakingLatestParamsReturningTestStruct,
						Params:    params,
						ReturnVal: actual,
					},
				}

				ctx := tests.Context(t)

				require.NoError(t, cr.Bind(ctx, bindings))
				result, err := cr.BatchGetLatestValues(ctx, batchGetLatestValueRequest)
				require.NoError(t, err)

				anyContractBatch := result[bound]
				returnValue, err := anyContractBatch[0].GetResult()
				assert.NoError(t, err)
				assert.Contains(t, anyContractBatch[0].ReadName, MethodTakingLatestParamsReturningTestStruct)
				assert.Equal(t, &firstItem, returnValue)
			},
		},
		{
			Name: SolanaContractReaderBatchGetLatestValueDifferentParamsResultsRetainOrder,
			Test: func(t T) {
				cr := it.GetContractReader(t)
				cw := it.GetContractWriter(t)
				bindings := it.GetBindings(t)
				batchCallEntry := make(BatchCallEntry)
				batchGetLatestValueRequest := make(types.BatchGetLatestValuesRequest)
				bound := BindingsByName(bindings, AnyContractName)[0]
				testIdx := it.testContext[t.Name()]

				for i := 0; i < 10; i++ {
					// setup test data
					ts := CreateTestStruct(i, it)
					args := StoreStructArgs{
						TestIdx: testIdx,
						Data: ts,
					}
					batchCallEntry[bound] = append(batchCallEntry[bound], ReadEntry{Name: MethodTakingLatestParamsReturningTestStruct, ReturnValue: &args})
					// setup call data
					batchGetLatestValueRequest[bound] = append(
						batchGetLatestValueRequest[bound],
						types.BatchRead{ReadName: MethodTakingLatestParamsReturningTestStruct, Params: &LatestParams{I: 1 + i}, ReturnVal: &TestStruct{}},
					)
				}
				batchContractWrite(t, it, cw, bindings, batchCallEntry)

				ctx := tests.Context(t)
				require.NoError(t, cr.Bind(ctx, bindings))

				result, err := cr.BatchGetLatestValues(ctx, batchGetLatestValueRequest)
				require.NoError(t, err)

				for i := 0; i < 10; i++ {
					resultAnyContract, testDataAnyContract := result[bound], batchCallEntry[bound]
					returnValue, err := resultAnyContract[i].GetResult()
					assert.NoError(t, err)
					assert.Contains(t, resultAnyContract[i].ReadName, MethodTakingLatestParamsReturningTestStruct)
					assert.Equal(t, testDataAnyContract[i].ReturnValue, returnValue)
				}
			},
		},
		{
			Name: SolanaContractReaderBatchGetLatestValueDifferentParamsResultsRetainOrderMultipleContracts,
			Test: func(t T) {
				cr := it.GetContractReader(t)
				cw := it.GetContractWriter(t)
				bindings := it.GetBindings(t)
				batchCallEntry := make(BatchCallEntry)
				batchGetLatestValueRequest := make(types.BatchGetLatestValuesRequest)
				bound1 := BindingsByName(bindings, AnyContractName)[0]
				bound2 := BindingsByName(bindings, AnySecondContractName)[0]
				testIdx := it.testContext[t.Name()]

				for i := 0; i < 10; i++ {
					// setup test data
					ts1, ts2 := CreateTestStruct(i, it), CreateTestStruct(i+10, it)
					args1 := StoreStructArgs{
						TestIdx: testIdx,
						Data: ts1,
					}
					args2 := StoreStructArgs{
						TestIdx: testIdx,
						Data: ts2,
					}
					batchCallEntry[bound1] = append(batchCallEntry[bound1], ReadEntry{Name: MethodTakingLatestParamsReturningTestStruct, ReturnValue: &args1})
					batchCallEntry[bound2] = append(batchCallEntry[bound2], ReadEntry{Name: MethodTakingLatestParamsReturningTestStruct, ReturnValue: &args2})
					// setup call data
					batchGetLatestValueRequest[bound1] = append(batchGetLatestValueRequest[bound1], types.BatchRead{ReadName: MethodTakingLatestParamsReturningTestStruct, Params: &LatestParams{I: 1 + i}, ReturnVal: &TestStruct{}})
					batchGetLatestValueRequest[bound2] = append(batchGetLatestValueRequest[bound2], types.BatchRead{ReadName: MethodTakingLatestParamsReturningTestStruct, Params: &LatestParams{I: 1 + i}, ReturnVal: &TestStruct{}})
				}
				batchContractWrite(t, it, cw, bindings, batchCallEntry)

				ctx := tests.Context(t)
				require.NoError(t, cr.Bind(ctx, bindings))

				result, err := cr.BatchGetLatestValues(ctx, batchGetLatestValueRequest)
				require.NoError(t, err)

				for idx := 0; idx < 10; idx++ {
					fmt.Printf("expected: %+v\n", batchCallEntry[bound1][idx].ReturnValue)
					if val, err := result[bound1][idx].GetResult(); err == nil {
						fmt.Printf("result: %+v\n", val)
					}
				}

				for i := 0; i < 10; i++ {
					testDataAnyContract, testDataAnySecondContract := batchCallEntry[bound1], batchCallEntry[bound2]
					resultAnyContract, resultAnySecondContract := result[bound1], result[bound2]
					returnValueAnyContract, errAnyContract := resultAnyContract[i].GetResult()
					returnValueAnySecondContract, errAnySecondContract := resultAnySecondContract[i].GetResult()
					assert.NoError(t, errAnyContract)
					assert.NoError(t, errAnySecondContract)
					assert.Contains(t, resultAnyContract[i].ReadName, MethodTakingLatestParamsReturningTestStruct)
					assert.Contains(t, resultAnySecondContract[i].ReadName, MethodTakingLatestParamsReturningTestStruct)
					assert.Equal(t, testDataAnyContract[i].ReturnValue, returnValueAnyContract)
					assert.Equal(t, testDataAnySecondContract[i].ReturnValue, returnValueAnySecondContract)
				}
			},
		},
	}

	RunTests(t, it, testCases)
}

func RunContractReaderInLoopTests[T WrappedTestingT[T]](t T, it ChainComponentsInterfaceTester[T]) {
	RunContractReaderInterfaceTests(t, it, false, true)

	var testCases []Testcase[T]

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
		// {Name: AnySecondContractName, Address: it.Helper.CreateAccount(t, *it, AnySecondContractName, AnyDifferentValueToReadWithoutAnArgument, testStruct).String()},
	}
}

func (it *SolanaChainComponentsInterfaceTester[T]) DirtyContracts() {}

func (it *SolanaChainComponentsInterfaceTester[T]) MaxWaitTimeForEvents() time.Duration {
	return time.Second
}

func (it *SolanaChainComponentsInterfaceTester[T]) GenerateBlocksTillConfidenceLevel(t T, contractName, readName string, confidenceLevel primitives.ConfidenceLevel) {

}

type helper struct {
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

	loader := solanautils.NewLoader[client.ReaderWriter](func(ctx context.Context) (client.ReaderWriter, error) { return solanaClient, nil})
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

	soPath := filepath.Join(utils.IDLDir,  fileName)

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
	Data TestStruct
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
		Value: value,
	}
	SubmitTransactionToCW(t, &it, cw, "initialize", initArgs, types.BoundContract{Name: contractName, Address: programID.String()}, types.Finalized)

	storeStructArgs := StoreStructArgs{
		TestIdx: testIdx,
		Data: testStruct,
	}
	SubmitTransactionToCW(t, &it, cw, MethodSettingStruct, storeStructArgs, types.BoundContract{Name: contractName, Address: programID.String()}, types.Finalized)
}

func (it *SolanaChainComponentsInterfaceTester[T]) buildContractReaderConfig(t T) config.ContractReader {
	idx := it.getTestIdx(t.Name())
	pdaDataPrefix := []byte("data")
	pdaDataPrefix = binary.LittleEndian.AppendUint64(pdaDataPrefix, idx)
	pdaStructDataPrefix := []byte("struct_data")
	pdaStructDataPrefix = binary.LittleEndian.AppendUint64(pdaStructDataPrefix, idx)
	testStruct := CreateTestStruct(0, it)
	return config.ContractReader{
		Namespaces: map[string]config.ChainContractReader{
			AnyContractName: {
				IDL: mustUnmarshalIDL(t, string(it.Helper.GetPrimaryIDL(t))),
				Reads: map[string]config.ReadDefinition{
					MethodReturningUint64: {
						ChainSpecificName: "DataAccount",
						ReadType:          config.Account,
						PDADefiniton: codec.PDATypeDef{
							Prefix: pdaDataPrefix,
						},
						OutputModifications: commoncodec.ModifiersConfig{
							&commoncodec.PropertyExtractorConfig{FieldName: "U64Value"},
						},
					},
					MethodReturningUint64Slice: {
						ChainSpecificName: "DataAccount",
						ReadType:          config.Account,
						PDADefiniton: codec.PDATypeDef{
							Prefix: pdaDataPrefix,
						},
						OutputModifications: commoncodec.ModifiersConfig{
							&commoncodec.PropertyExtractorConfig{FieldName: "U64Slice"},
						},
					},
					MethodSettingUint64: {
						ChainSpecificName: "DataAccount",
						ReadType:          config.Account,
						PDADefiniton: codec.PDATypeDef{
							Prefix: pdaDataPrefix,
						},
						OutputModifications: commoncodec.ModifiersConfig{
							&commoncodec.PropertyExtractorConfig{FieldName: "U64Value"},
						},
					},
					MethodReturningSeenStruct: {
						ChainSpecificName: "TestStruct",
						ReadType:          config.Account,
						PDADefiniton: codec.PDATypeDef{
							Prefix: pdaStructDataPrefix,
						},
						OutputModifications: commoncodec.ModifiersConfig{
							&commoncodec.HardCodeModifierConfig{
								OnChainValues: map[string]any{
									"DifferentField": copy(make([]byte, 32), []byte(testStruct.DifferentField)),
									"NestedDynamicStruct.Inner.S": copy(make([]byte, 32), []byte(testStruct.NestedDynamicStruct.Inner.S)),
								},
								OffChainValues: map[string]any{
									"ExtraField": AnyExtraValue,
									"DifferentField": testStruct.DifferentField,
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
						PDADefiniton: codec.PDATypeDef{
							Prefix: pdaStructDataPrefix,
						},
						OutputModifications: commoncodec.ModifiersConfig{
							&commoncodec.HardCodeModifierConfig{
								OnChainValues: map[string]any{
									"DifferentField": copy(make([]byte, 32), []byte(testStruct.DifferentField)),
									"NestedDynamicStruct.Inner.S": copy(make([]byte, 32), []byte(testStruct.NestedDynamicStruct.Inner.S)),
								},
								OffChainValues: map[string]any{
									"ExtraField": AnyExtraValue,
									"DifferentField": testStruct.DifferentField,
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
						PDADefiniton: codec.PDATypeDef{
							Prefix: pdaDataPrefix,
						},
						OutputModifications: commoncodec.ModifiersConfig{
							&commoncodec.PropertyExtractorConfig{FieldName: "U64Value"},
						},
					},
				},
			},
		},
	}
}

func (it *SolanaChainComponentsInterfaceTester[T]) buildContractWriterConfig(t T) chainwriter.ChainWriterConfig {
	idx := it.getTestIdx(t.Name())
	testIdx := binary.LittleEndian.AppendUint64([]byte{}, idx)
	fromAddress := solana.MustPrivateKeyFromBase58(solclient.DefaultPrivateKeysSolValidator[1]).PublicKey().String()
	testStruct := CreateTestStruct(0, it)
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
								Name: "Signer",
								Address: fromAddress,
								IsSigner: true,
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
								Name: "SystemProgram",
								Address: solana.SystemProgramID.String(),
								IsWritable: false,
								IsSigner: false,
							},
						},
						DebugIDLocation: "",
					},
					MethodSettingStruct: {
						FromAddress:        fromAddress,
						InputModifications: []commoncodec.ModifierConfig{
							&commoncodec.AddressBytesToStringModifierConfig{
								Fields: []string{"Data.AccountStruct.AccountStr"},
							},
							&commoncodec.HardCodeModifierConfig{
								OnChainValues: map[string]any{
									"Data.Padding0": []byte{},
									"Data.Padding1": []byte{},
									"Data.Padding2": []byte{},
									"Data.NestedDynamicStruct.Padding": []byte{},
									"Data.NestedStaticStruct.Padding": []byte{},
									"Data.DifferentField": copy(make([]byte, 32), []byte(testStruct.DifferentField)),
									"Data.NestedDynamicStruct.Inner.S": copy(make([]byte, 32), []byte(testStruct.NestedDynamicStruct.Inner.S)),
								},
								OffChainValues: map[string]any{
									"Data.DifferentField": testStruct.DifferentField,
									"Data.NestedDynamicStruct.Inner.S": testStruct.NestedDynamicStruct.Inner.S,
								},
							},
						},
						ChainSpecificName: "store",
						LookupTables:       chainwriter.LookupTables{},
						Accounts: []chainwriter.Lookup{
							chainwriter.AccountConstant{
								Name: "Signer",
								Address: fromAddress,
								IsSigner: true,
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
								Name: "SystemProgram",
								Address: solana.SystemProgramID.String(),
								IsWritable: false,
								IsSigner: false,
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
								Name: "Signer",
								Address: fromAddress,
								IsSigner: true,
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
								Name: "SystemAccount",
								Address: solana.SystemProgramID.String(),
								IsWritable: false,
								IsSigner: false,
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

// Copied from chainlink-common since this method is not public: https://github.com/smartcontractkit/chainlink-common/blob/aea9294a7d555844336a92c9ffe41219dfb26c68/pkg/types/interfacetests/utils.go#L88
func batchContractWrite[T TestingT[T]](t T, tester ChainComponentsInterfaceTester[T], cw types.ContractWriter, boundContracts []types.BoundContract, batchCallEntry BatchCallEntry) {
	nameToAddress := make(map[string]string)
	for _, bc := range boundContracts {
		nameToAddress[bc.Name] = bc.Address
	}

	// For each contract in the batch call entry, submit the read entries to the chain
	for contract, contractBatch := range batchCallEntry {
		require.Contains(t, nameToAddress, contract.Name)
		for _, readEntry := range contractBatch {
			val, isOk := readEntry.ReturnValue.(*TestStruct)
			if !isOk {
				require.Fail(t, "expected *TestStruct for contract: %s read: %s, but received %T", contract.Name, readEntry.Name, readEntry.ReturnValue)
			}
			SubmitTransactionToCW(t, tester, cw, MethodSettingStruct, val, types.BoundContract{Name: contract.Name, Address: nameToAddress[contract.Name]}, types.Unconfirmed)
		}
	}
}

const (
	primaryProgramPubKey = "6AfuXF6HapDUhQfE4nQG9C1SGtA1YjP3icaJyRfU4RyE"
	secondaryProgramPubKey = "9SFyk8NmGYh5D612mJwUYhguCRY9cFgaS2vksrigepjf"
)
