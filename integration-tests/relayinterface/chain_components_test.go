/*
Package relayinterface contains the interface tests for chain components.
*/
package relayinterface

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/ws"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/smartcontractkit/chainlink-common/pkg/sqlutil/pg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/tokens"
	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	commonconfig "github.com/smartcontractkit/chainlink-common/pkg/config"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	commontestutils "github.com/smartcontractkit/chainlink-common/pkg/loop/testutils"
	"github.com/smartcontractkit/chainlink-common/pkg/services/servicetest"
	"github.com/smartcontractkit/chainlink-common/pkg/sqlutil/sqltest"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	. "github.com/smartcontractkit/chainlink-common/pkg/types/interfacetests" //nolint common practice to import test mods with .
	"github.com/smartcontractkit/chainlink-common/pkg/types/query"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query/primitives"
	"github.com/smartcontractkit/chainlink-protos/cre/go/values"

	contractprimary "github.com/smartcontractkit/chainlink-solana/contracts/generated/contract_reader_interface"
	contractsecondary "github.com/smartcontractkit/chainlink-solana/contracts/generated/contract_reader_interface_secondary"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/solclient"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/utils"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainreader"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainwriter"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	codecv1 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v1"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/txm"
	keyMocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/mocks"
	solanautils "github.com/smartcontractkit/chainlink-solana/pkg/solana/utils"
)

const (
	AnyContractNameWithSharedAddress1 = AnyContractName + "Shared1"
	AnyContractNameWithSharedAddress2 = AnyContractName + "Shared2"
	AnyContractNameWithSharedAddress3 = AnyContractName + "Shared3"
)

var trueVal = true

// StoreTestStruct is used to send data on chain
// TestIdx is hardcoded per test in chainReaderConfig
type StoreTestStruct struct {
	Data    TestStruct // test data
	ListIdx uint8      // identifier to differentiate between different test structs in the same test
}

func TestChainComponents(t *testing.T) {
	t.Parallel()

	t.Run("RunChainComponentsSolanaTests", func(t *testing.T) {
		t.Parallel()
		helper := &helper{}
		helper.Init(t)
		it := &SolanaChainComponentsInterfaceTester[*testing.T]{Helper: helper, testContext: make(map[string]uint64), testContextMu: &sync.RWMutex{}, testIdx: &atomic.Uint64{}, inMemoryDB: helper.InMemoryDB()}
		DisableTests(it)
		it.Setup(t)
		RunChainComponentsSolanaTests(t, it)
	})

	t.Run("RunChainComponentsInLoopSolanaTests", func(t *testing.T) {
		t.Parallel()
		helper := &helper{}
		helper.Init(t)
		it := &SolanaChainComponentsInterfaceTester[*testing.T]{Helper: helper, testContext: make(map[string]uint64), testContextMu: &sync.RWMutex{}, testIdx: &atomic.Uint64{}, inMemoryDB: helper.InMemoryDB()}
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
		ContractReaderGetLatestValueBasedOnConfidenceLevelForEvent,
		// these two tests do not make sense for solana
		ContractReaderGetLatestValueWithModifiersUsingOwnMapstrctureOverrides,
		ContractReaderBatchGetLatestValueWithModifiersOwnMapstructureOverride,

		// THESE TESTS HAVE BEEN ADAPTED FOR SOLANA BELOW, under Solana(TestName)
		ContractReaderGetLatestValue,
		ContractReaderGetLatestValueAsValuesDotValue,
		ContractReaderBatchGetLatestValue,
		ContractReaderBatchGetLatestValueDifferentParamsResultsRetainOrder,
		ContractReaderBatchGetLatestValueDifferentParamsResultsRetainOrderMultipleContracts,

		// THESE TESTS HAVE BEEN ADAPTED FOR SOLANA BELOW
		// All lumped into "SolanaContractReader_QueryKeyTests_And_GetsLatestForEventTests" below
		// We are not deploying multiple contracts, so we cannot test order of logs if we run tests in parallel
		ContractReaderGetLatestValueGetsLatestForEvent,
		ContractReaderGetLatestValueReturnsNotFoundWhenNotTriggeredForEvent,
		ContractReaderGetLatestValueWithFilteringForEvent,
		ContractReaderQueryKeyNotFound,
		ContractReaderQueryKeyReturnsData,
		ContractReaderQueryKeyReturnsDataAsValuesDotValue,
		ContractReaderQueryKeyCanFilterWithValueComparator,
		ContractReaderQueryKeyCanLimitResultsWithCursor,

		// query keys not implemented yet
		ContractReaderQueryKeysReturnsDataTwoEventTypes,
		ContractReaderQueryKeysNotFound,
		ContractReaderQueryKeysReturnsData,
		ContractReaderQueryKeysReturnsDataAsValuesDotValue,
		ContractReaderQueryKeysCanFilterWithValueComparator,
		ContractReaderQueryKeysCanLimitResultsWithCursor,
	})
	if it.inMemoryDB {
		it.DisableTests([]string{ContractReaderGetLatestValueIncludeReverted})
	}
}

func RunChainComponentsSolanaTests[T WrappedTestingT[T]](t T, it *SolanaChainComponentsInterfaceTester[T]) {
	testCases := []Testcase[T]{
		{
			Name: "Test address groups where first namespace shares address with second namespace",
			Test: func(t T) {
				ctx := t.Context()
				cfg := it.buildContractReaderConfig(t)
				cfg.AddressShareGroups = [][]string{{AnyContractNameWithSharedAddress1, AnyContractNameWithSharedAddress2, AnyContractNameWithSharedAddress3}}
				cr := it.GetContractReaderWithCustomCfg(t, cfg)

				t.Run("Namespace is part of an address share group that doesn't have a registered address and provides no address during Bind", func(t T) {
					bound1 := []types.BoundContract{{
						Name: AnyContractNameWithSharedAddress1,
					}}
					require.Error(t, cr.Bind(ctx, bound1))
				})

				addressToBeShared := it.Helper.CreateAccount(t, *it, AnyContractName, AnyValueToReadWithoutAnArgument).String()
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
		},
		{
			Name: ContractReaderGetLatestValueGetTokenPrices,
			Test: func(t T) {
				cr := it.GetContractReader(t)
				bindings := it.GetBindings(t)
				ctx := t.Context()

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
		{
			Name: "Solana" + ContractReaderGetLatestValue,
			Test: func(t T) {
				cr := it.GetContractReader(t)
				cw := it.GetContractWriter(t)
				bindings := it.GetBindings(t)
				ctx := t.Context()
				require.NoError(t, cr.Bind(ctx, bindings))
				bound := BindingsByName(bindings, AnyContractName)[0]

				firstItemData := CreateTestStruct(1, it)
				firstItem := StoreTestStruct{
					Data:    firstItemData,
					ListIdx: uint8(1),
				}
				_ = SubmitTransactionToCW(t, it, cw, MethodSettingStruct, firstItem, bound, types.Finalized)

				secondItemData := CreateTestStruct(2, it)
				secondItem := StoreTestStruct{
					Data:    secondItemData,
					ListIdx: uint8(2),
				}
				_ = SubmitTransactionToCW(t, it, cw, MethodSettingStruct, secondItem, bound, types.Finalized)

				actual := &TestStruct{}
				params := &LatestParams{I: int(firstItem.ListIdx)}
				require.NoError(t, cr.GetLatestValue(ctx, bound.ReadIdentifier(MethodTakingLatestParamsReturningTestStruct), primitives.Finalized, params, actual))
				assert.Equal(t, &firstItemData, actual)

				params.I = int(secondItem.ListIdx)
				actual = &TestStruct{}
				require.NoError(t, cr.GetLatestValue(ctx, bound.ReadIdentifier(MethodTakingLatestParamsReturningTestStruct), primitives.Finalized, params, actual))
				assert.Equal(t, &secondItemData, actual)
			},
		},
		{
			Name: "Solana" + ContractReaderGetLatestValueAsValuesDotValue,
			Test: func(t T) {
				cr := it.GetContractReader(t)
				cw := it.GetContractWriter(t)
				bindings := it.GetBindings(t)
				ctx := t.Context()
				require.NoError(t, cr.Bind(ctx, bindings))
				bound := BindingsByName(bindings, AnyContractName)[0]

				firstItemData := CreateTestStruct(1, it)
				firstItem := StoreTestStruct{
					Data:    firstItemData,
					ListIdx: uint8(1),
				}
				_ = SubmitTransactionToCW(t, it, cw, MethodSettingStruct, firstItem, bound, types.Finalized)

				params := &LatestParams{I: int(firstItem.ListIdx)}
				var value values.Value
				err := cr.GetLatestValue(ctx, bound.ReadIdentifier(MethodTakingLatestParamsReturningTestStruct), primitives.Finalized, params, &value)
				require.NoError(t, err)
				actual := TestStruct{}
				err = value.UnwrapTo(&actual)
				require.NoError(t, err)
				assert.Equal(t, &firstItemData, &actual)
			},
		},
		{
			Name: "Solana" + ContractReaderBatchGetLatestValue,
			Test: func(t T) {
				cr := it.GetContractReader(t)
				cw := it.GetContractWriter(t)
				bindings := it.GetBindings(t)
				ctx := t.Context()
				require.NoError(t, cr.Bind(ctx, bindings))
				bound := BindingsByName(bindings, AnyContractName)[0]

				firstItemData := CreateTestStruct(1, it)
				firstItem := StoreTestStruct{
					Data:    firstItemData,
					ListIdx: uint8(1),
				}
				_ = SubmitTransactionToCW(t, it, cw, MethodSettingStruct, firstItem, bound, types.Finalized)

				params, actual := &LatestParams{I: int(firstItem.ListIdx)}, &TestStruct{}
				batchGetLatestValueRequest := make(types.BatchGetLatestValuesRequest)
				batchGetLatestValueRequest[bound] = []types.BatchRead{
					{
						ReadName:  MethodTakingLatestParamsReturningTestStruct,
						Params:    params,
						ReturnVal: actual,
					},
				}
				result, err := cr.BatchGetLatestValues(ctx, batchGetLatestValueRequest)
				require.NoError(t, err)

				anyContractBatch := result[bound]
				returnValue, err := anyContractBatch[0].GetResult()
				assert.NoError(t, err)
				assert.Contains(t, anyContractBatch[0].ReadName, MethodTakingLatestParamsReturningTestStruct)
				assert.Equal(t, &firstItemData, returnValue)
			},
		},
		{
			Name: "Solana" + ContractReaderBatchGetLatestValueDifferentParamsResultsRetainOrder,
			Test: func(t T) {
				cr := it.GetContractReader(t)
				cw := it.GetContractWriter(t)
				bindings := it.GetBindings(t)
				ctx := t.Context()
				require.NoError(t, cr.Bind(ctx, bindings))
				bound := BindingsByName(bindings, AnyContractName)[0]

				batchGetLatestValueRequest := make(types.BatchGetLatestValuesRequest)
				batchCallEntry := make(BatchCallEntry)
				for i := 0; i < 3; i++ {
					// setup test data
					ts := CreateTestStruct(i, it)
					tsItem := StoreTestStruct{
						Data:    ts,
						ListIdx: uint8(i),
					}
					// this is only used for assertion below, but not for batch writing
					batchCallEntry[bound] = append(batchCallEntry[bound], ReadEntry{Name: MethodTakingLatestParamsReturningTestStruct, ReturnValue: &ts})
					_ = SubmitTransactionToCW(t, it, cw, MethodSettingStruct, tsItem, bound, types.Finalized)
					// setup call data
					batchGetLatestValueRequest[bound] = append(
						batchGetLatestValueRequest[bound],
						types.BatchRead{ReadName: MethodTakingLatestParamsReturningTestStruct, Params: &LatestParams{I: i}, ReturnVal: &TestStruct{}},
					)
				}
				result, err := cr.BatchGetLatestValues(ctx, batchGetLatestValueRequest)
				require.NoError(t, err)
				for i := 0; i < 3; i++ {
					resultAnyContract, testDataAnyContract := result[bound], batchCallEntry[bound]
					returnValue, err := resultAnyContract[i].GetResult()
					assert.NoError(t, err)
					assert.Contains(t, resultAnyContract[i].ReadName, MethodTakingLatestParamsReturningTestStruct)
					assert.Equal(t, testDataAnyContract[i].ReturnValue, returnValue)
				}
			},
		},
		{
			Name: "Solana" + ContractReaderBatchGetLatestValueDifferentParamsResultsRetainOrderMultipleContracts,
			Test: func(t T) {
				cr := it.GetContractReader(t)
				cw := it.GetContractWriter(t)
				bindings := it.GetBindings(t)
				ctx := t.Context()
				require.NoError(t, cr.Bind(ctx, bindings))
				bound1 := BindingsByName(bindings, AnyContractName)[0]
				bound2 := BindingsByName(bindings, AnySecondContractName)[0]

				batchGetLatestValueRequest := make(types.BatchGetLatestValuesRequest)
				batchCallEntry := make(BatchCallEntry)
				for i := 0; i < 3; i++ {
					// setup test data
					firstItemIdx := i
					secondItemIdx := i + 10
					ts1, ts2 := CreateTestStruct(firstItemIdx, it), CreateTestStruct(secondItemIdx, it)
					ts1Item, ts2Item := StoreTestStruct{
						Data:    ts1,
						ListIdx: uint8(firstItemIdx),
					}, StoreTestStruct{
						Data:    ts2,
						ListIdx: uint8(secondItemIdx),
					}
					batchCallEntry[bound1] = append(batchCallEntry[bound1], ReadEntry{Name: MethodTakingLatestParamsReturningTestStruct, ReturnValue: &ts1})
					batchCallEntry[bound2] = append(batchCallEntry[bound2], ReadEntry{Name: MethodTakingLatestParamsReturningTestStruct, ReturnValue: &ts2})
					_ = SubmitTransactionToCW(t, it, cw, MethodSettingStruct, ts1Item, bound1, types.Finalized)
					_ = SubmitTransactionToCW(t, it, cw, MethodSettingStruct, ts2Item, bound2, types.Finalized)
					// setup call data
					batchGetLatestValueRequest[bound1] = append(batchGetLatestValueRequest[bound1], types.BatchRead{ReadName: MethodTakingLatestParamsReturningTestStruct, Params: &LatestParams{I: firstItemIdx}, ReturnVal: &TestStruct{}})
					batchGetLatestValueRequest[bound2] = append(batchGetLatestValueRequest[bound2], types.BatchRead{ReadName: MethodTakingLatestParamsReturningTestStruct, Params: &LatestParams{I: secondItemIdx}, ReturnVal: &TestStruct{}})
				}
				result, err := cr.BatchGetLatestValues(ctx, batchGetLatestValueRequest)
				require.NoError(t, err)
				for idx := 0; idx < 3; idx++ {
					fmt.Printf("expected: %+v\n", batchCallEntry[bound1][idx].ReturnValue)
					if val, err := result[bound1][idx].GetResult(); err == nil {
						fmt.Printf("result: %+v\n", val)
					}
				}

				for i := 0; i < 3; i++ {
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
		{
			Name: "SolanaContractReader_QueryKeyTests_And_GetsLatestForEventTests",
			Test: func(t T) {
				cr := it.GetContractReader(t)
				cw := it.GetContractWriter(t)
				bindings := it.GetBindings(t)
				ctx := t.Context()
				require.NoError(t, cr.Bind(ctx, bindings))
				bound := BindingsByName(bindings, AnyContractName)[0]

				// ContractReaderQueryKeyNotFound
				logs, logError := cr.QueryKey(ctx, bound, query.KeyFilter{Key: EventName}, query.LimitAndSort{}, &TestStruct{})
				require.NoError(t, logError)
				assert.Len(t, logs, 0)

				// ContractReaderGetLatestValueReturnsNotFoundWhenNotTriggeredForEvent
				result := &TestStruct{}
				latestEventError := cr.GetLatestValue(ctx, bound.ReadIdentifier(EventName), primitives.Unconfirmed, nil, &result)
				assert.True(t, errors.Is(latestEventError, types.ErrNotFound))

				testStructs := make([]TestStruct, 4)

				ts1 := CreateTestStruct[T](0, it)
				testStructs[0] = ts1
				_ = SubmitTransactionToCW(t, it, cw, MethodTriggeringEvent, it.Helper.DataWrap(ts1), bound, types.Finalized)

				ts2 := CreateTestStruct[T](1, it)
				testStructs[1] = ts2
				_ = SubmitTransactionToCW(t, it, cw, MethodTriggeringEvent, it.Helper.DataWrap(ts2), bound, types.Finalized)

				// ContractReaderGetLatestValueGetsLatestForEvent
				result = &TestStruct{}
				require.Eventually(t, func() bool {
					err := cr.GetLatestValue(ctx, bound.ReadIdentifier(EventName), primitives.Finalized, nil, &result)
					return err == nil && reflect.DeepEqual(result, &testStructs[1])
				}, it.MaxWaitTimeForEvents(), time.Millisecond*10)

				// ContractReaderGetLatestValueWithFilteringForEvent
				filterParams := &FilterEventParams{Field: *ts1.Field}
				assert.Never(t, func() bool {
					result2 := &TestStruct{}
					latestEventError2 := cr.GetLatestValue(ctx, bound.ReadIdentifier(EventName), primitives.Unconfirmed, filterParams, &result2)
					return latestEventError2 == nil && reflect.DeepEqual(result2, &ts2)
				}, it.MaxWaitTimeForEvents(), time.Millisecond*10)

				// get the result one more time to verify it.
				// Using the result from the Never statement by creating result outside the block is a data race
				result = &TestStruct{}
				err := cr.GetLatestValue(ctx, bound.ReadIdentifier(EventName), primitives.Unconfirmed, filterParams, &result)
				require.NoError(t, err)
				assert.Equal(t, &ts1, result)

				// ContractReaderQueryKeyReturnsData
				tst := &TestStruct{}
				require.Eventually(t, func() bool {
					// sequences from queryKey without limit and sort should be in descending order
					sequences, err := cr.QueryKey(ctx, bound, query.KeyFilter{Key: EventName}, query.LimitAndSort{}, tst)
					return err == nil && len(sequences) == 2 && reflect.DeepEqual(&ts1, sequences[1].Data) && reflect.DeepEqual(&ts2, sequences[0].Data)
				}, it.MaxWaitTimeForEvents(), time.Millisecond*10)

				// ContractReaderQueryKeyReturnsDataAsValuesDotValue
				var value values.Value
				require.Eventually(t, func() bool {
					// sequences from queryKey without limit and sort should be in descending order
					sequences, err := cr.QueryKey(ctx, bound, query.KeyFilter{Key: EventName}, query.LimitAndSort{}, &value)
					if err != nil || len(sequences) != 2 {
						return false
					}

					data1 := *sequences[1].Data.(*values.Value)
					ts := TestStruct{}
					err = data1.UnwrapTo(&ts)
					require.NoError(t, err)
					fmt.Printf("ts from data1: %+v\n", ts)
					assert.Equal(t, &ts1, &ts)

					data2 := *sequences[0].Data.(*values.Value)
					ts = TestStruct{}
					err = data2.UnwrapTo(&ts)
					require.NoError(t, err)
					fmt.Printf("ts from data2: %+v\n", ts)
					assert.Equal(t, &ts2, &ts)

					return true
				}, it.MaxWaitTimeForEvents(), time.Millisecond*10)

				ts5 := CreateTestStruct[T](5, it)
				testStructs[2] = ts5
				_ = SubmitTransactionToCW(t, it, cw, MethodTriggeringEvent, it.Helper.DataWrap(ts5), bound, types.Finalized)

				ts10 := CreateTestStruct[T](10, it)
				testStructs[3] = ts10
				_ = SubmitTransactionToCW(t, it, cw, MethodTriggeringEvent, it.Helper.DataWrap(ts10), bound, types.Finalized)

				// ContractReaderQueryKeyCanFilterWithValueComparator
				tst = &TestStruct{}
				require.Eventually(t, func() bool {
					sequences, err := cr.QueryKey(ctx, bound, query.KeyFilter{Key: EventName, Expressions: []query.Expression{
						query.Comparator("Field",
							primitives.ValueComparator{
								Value:    *ts5.Field,
								Operator: primitives.Gte,
							},
							primitives.ValueComparator{
								Value:    *ts10.Field,
								Operator: primitives.Lte,
							}),
					},
					}, query.LimitAndSort{}, tst)

					return err == nil && len(sequences) == 2 && reflect.DeepEqual(&ts5, sequences[1].Data) && reflect.DeepEqual(&ts10, sequences[0].Data)
				}, it.MaxWaitTimeForEvents(), time.Millisecond*10)

				// ContractReaderQueryKeyCanLimitResultsWithCursor
				require.Eventually(t, func() bool {
					var allSequences []types.Sequence

					filter := query.KeyFilter{Key: EventName, Expressions: []query.Expression{
						query.Confidence(primitives.Finalized),
					}}
					limit := query.LimitAndSort{
						SortBy: []query.SortBy{query.NewSortBySequence(query.Asc)},
						Limit:  query.CountLimit(2),
					}

					for idx := 0; idx < len(testStructs)/2; idx++ {
						// sequences from queryKey without limit and sort should be in descending order
						sequences, err := cr.QueryKey(ctx, bound, filter, limit, &TestStruct{})

						require.NoError(t, err)

						if len(sequences) == 0 {
							continue
						}

						limit.Limit = query.CursorLimit(sequences[len(sequences)-1].Cursor, query.CursorFollowing, 2)
						allSequences = append(allSequences, sequences...)
					}

					return len(allSequences) == len(testStructs) &&
						reflect.DeepEqual(&testStructs[0], allSequences[0].Data) &&
						reflect.DeepEqual(&testStructs[len(testStructs)-1], allSequences[len(testStructs)-1].Data)
				}, it.MaxWaitTimeForEvents(), 500*time.Millisecond)
			},
		},
	}

	RunTests(t, it, testCases)
	RunContractReaderTests(t, it)
	RunChainWriterTests(t, it)
}

func RunChainComponentsInLoopSolanaTests[T WrappedTestingT[T]](t T, it ChainComponentsInterfaceTester[T]) {
	RunContractReaderInLoopTests(t, it)
	// Add ChainWriter tests here
}

func RunContractReaderTests[T WrappedTestingT[T]](t T, it *SolanaChainComponentsInterfaceTester[T]) {
	RunContractReaderInterfaceTests(t, it, false, true)
}

func RunChainWriterTests[T WrappedTestingT[T]](t T, it *SolanaChainComponentsInterfaceTester[T]) {
	testCases := []Testcase[T]{
		{
			Name: ChainWriterLookupTableTest,
			Test: func(t T) {
				cr := it.GetContractReader(t)
				cw := it.GetContractWriter(t)
				contracts := it.GetBindings(t)

				idx := it.getTestIdx(t.Name())
				ctx := t.Context()
				bound := BindingsByName(contracts, AnyContractName)[0]
				require.NoError(t, cr.Bind(ctx, contracts))

				testIdx := binary.LittleEndian.AppendUint64([]byte{}, idx)
				dataPDAAccount, _, err := solana.FindProgramAddress([][]byte{[]byte("data"), testIdx}, solana.MustPublicKeyFromBase58(bound.Address))
				require.NoError(t, err)

				// append random addresses to lookup table address list
				lookupTableAddresses := make([]solana.PublicKey, 0, 10)
				for i := 0; i < 9; i++ {
					pk, pkErr := solana.NewRandomPrivateKey()
					require.NoError(t, pkErr)
					lookupTableAddresses = append(lookupTableAddresses, pk.PublicKey())
				}

				lookupTableAddresses = append(lookupTableAddresses, dataPDAAccount)

				lookupTableAddr := CreateTestLookupTable(ctx, t, it.Helper.SolanaClient(), *it.Helper.TXM(), it.Helper.Sender(), lookupTableAddresses)
				initLookupTableArgs := LookupTableArgs{
					LookupTable: lookupTableAddr,
				}

				SubmitTransactionToCW(t, it, cw, "initializeLookupTable", initLookupTableArgs, bound, types.Finalized)

				dataValue := uint64(1)
				storeValArgs := DataAccountArgs{
					TestIdx: idx,
					Value:   dataValue,
				}
				SubmitTransactionToCW(t, it, cw, "storeVal", storeValArgs, bound, types.Finalized)

				var value values.Value
				err = cr.GetLatestValue(ctx, bound.ReadIdentifier(MethodReturningUint64), primitives.Unconfirmed, nil, &value)
				require.NoError(t, err)

				var prim uint64
				err = value.UnwrapTo(&prim)
				require.NoError(t, err)

				assert.Equal(t, dataValue, prim)
			},
		},
		{
			Name: ChainWriterATASupportTest,
			Test: func(t T) {
				cr := it.GetContractReader(t)
				cw := it.GetContractWriter(t)
				contracts := it.GetBindings(t)

				idx := it.getTestIdx(t.Name())
				ctx := t.Context()
				bound := BindingsByName(contracts, AnyContractName)[0]
				require.NoError(t, cr.Bind(ctx, contracts))

				tokenProgram := solana.Token2022ProgramID
				feePayerPk := solana.MustPrivateKeyFromBase58(solclient.DefaultPrivateKeysSolValidator[1])
				mint := utils.CreateRandomToken(t.Context(), t, feePayerPk, tokenProgram, it.Helper.RPC())

				wallet, err := solana.NewRandomPrivateKey()
				require.NoError(t, err)

				ataAddress, _, err := tokens.FindAssociatedTokenAddress(tokenProgram, mint, wallet.PublicKey())
				require.NoError(t, err)

				args := StoreTokenAccountArgs{
					TestIdx:      idx,
					TokenAccount: ataAddress,
					ATAInfo: ATAInfo{
						Receiver:     wallet.PublicKey(),
						Wallet:       wallet.PublicKey(),
						TokenProgram: tokenProgram,
						Mint:         mint,
					},
				}
				SubmitTransactionToCW(t, it, cw, "storeTokenAccount", args, bound, types.Finalized)

				// Resubmit the same transaction to test ATA creation is skipped when the account already exists
				SubmitTransactionToCW(t, it, cw, "storeTokenAccount", args, bound, types.Finalized)
			},
		},
	}

	RunTests(t, it, testCases)
}

// GetLatestValue method
const (
	ContractReaderNotFoundReadsReturnZeroedResponses             = "Get latest value not found reads return zeroed responses"
	ContractReaderGetLatestValueUsingMultiReader                 = "Get latest value using multi reader"
	ContractReaderBatchGetLatestValueUsingMultiReader            = "Batch Get latest value using multi reader"
	ContractReaderGetLatestValueWithAddressHardcodedIntoResponse = "Get latest value with AddressHardcoded into response"
	ContractReaderGetLatestValueUsingMultiReaderWithParmsReuse   = "Get latest value using multi reader with params reuse"
	ContractReaderGetLatestValueGetTokenPrices                   = "Get latest value handles get token prices edge case"
	ContractReaderGetLatestValueIncludeReverted                  = "GetLatestValue includes reverted transactions when asked"
	ChainWriterLookupTableTest                                   = "Set contract value using a lookup table for addresses"
	ChainWriterATASupportTest                                    = "Initialize ATA if one does not exist"
)

func RunContractReaderInLoopTests[T WrappedTestingT[T]](t T, it ChainComponentsInterfaceTester[T]) {
	//RunContractReaderInterfaceTests(t, it, false, true)
	testCases := []Testcase[T]{
		{
			Name: ContractReaderNotFoundReadsReturnZeroedResponses,
			Test: func(t T) {
				cr := it.GetContractReader(t)
				bindings := it.GetBindings(t)
				ctx := t.Context()

				bound := BindingsByName(bindings, AnyContractName)[0]
				require.NoError(t, cr.Bind(ctx, bindings))

				dAccRes := contractprimary.DataAccount{}
				require.NoError(t, cr.GetLatestValue(ctx, bound.ReadIdentifier(ReadUninitializedPDA), primitives.Unconfirmed, nil, &dAccRes))
				require.Equal(t, contractprimary.DataAccount{}, dAccRes)

				mR3Res := contractprimary.MultiRead3{}
				batchGetLatestValueRequest := make(types.BatchGetLatestValuesRequest)
				batchGetLatestValueRequest[bound] = []types.BatchRead{
					{
						ReadName:  ReadUninitializedPDA,
						Params:    nil,
						ReturnVal: &dAccRes,
					},
					{
						ReadName:  MultiReadWithParamsReuse,
						Params:    map[string]any{"ID": 999},
						ReturnVal: &mR3Res,
					},
				}

				batchResult, err := cr.BatchGetLatestValues(ctx, batchGetLatestValueRequest)
				require.NoError(t, err)

				result, err := batchResult[bound][0].GetResult()
				require.NoError(t, err)
				require.Equal(t, &contractprimary.DataAccount{}, result)

				result, err = batchResult[bound][1].GetResult()
				require.NoError(t, err)
				require.Equal(t, &contractprimary.MultiRead3{}, result)
			},
		},
		{
			Name: ContractReaderGetLatestValueWithAddressHardcodedIntoResponse,
			Test: func(t T) {
				cr := it.GetContractReader(t)
				bindings := it.GetBindings(t)
				ctx := t.Context()

				bound := BindingsByName(bindings, AnyContractName)[0]
				require.NoError(t, cr.Bind(ctx, bindings))

				boundAddress, err := solana.PublicKeyFromBase58(bound.Address)
				require.NoError(t, err)

				type MultiReadResult struct {
					A              uint8
					B              int16
					SharedAddress  []byte
					AddressToShare []byte
				}

				mRR := MultiReadResult{}
				require.NoError(t, cr.GetLatestValue(ctx, bound.ReadIdentifier(ReadWithAddressHardCodedIntoResponse), primitives.Unconfirmed, nil, &mRR))

				expectedMRR := MultiReadResult{A: 1, B: 2, SharedAddress: boundAddress.Bytes(), AddressToShare: boundAddress.Bytes()}
				require.Equal(t, expectedMRR, mRR)
			},
		},
		{
			Name: ContractReaderGetLatestValueUsingMultiReader,
			Test: func(t T) {
				cr := it.GetContractReader(t)
				bindings := it.GetBindings(t)
				ctx := t.Context()

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
			Name: ContractReaderGetLatestValueUsingMultiReaderWithParmsReuse,
			Test: func(t T) {
				cr := it.GetContractReader(t)
				bindings := it.GetBindings(t)
				ctx := t.Context()

				bound := BindingsByName(bindings, AnyContractName)[0]

				require.NoError(t, cr.Bind(ctx, bindings))

				type MultiReadResult struct {
					A uint8
					B int16
					U string
					V bool
				}

				mRR := MultiReadResult{}
				require.NoError(t, cr.GetLatestValue(ctx, bound.ReadIdentifier(MultiReadWithParamsReuse), primitives.Unconfirmed, map[string]any{"ID": 1}, &mRR))

				expectedMRR := MultiReadResult{A: 10, B: 20, U: "olleH", V: true}
				require.Equal(t, expectedMRR, mRR)
			},
		},
		{
			Name: ContractReaderGetLatestValueGetTokenPrices,
			Test: func(t T) {
				cr := it.GetContractReader(t)
				bindings := it.GetBindings(t)
				ctx := t.Context()

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
		{
			Name: ContractReaderBatchGetLatestValueUsingMultiReader,
			Test: func(t T) {
				cr := it.GetContractReader(t)
				bindings := it.GetBindings(t)
				ctx := t.Context()
				bound := BindingsByName(bindings, AnyContractName)[0]

				require.NoError(t, cr.Bind(ctx, bindings))

				type MultiReadResult struct {
					A uint8
					B int16
					U string
					V bool
				}

				// setup call data
				actual := uint64(0)
				multiParams, multiActual := map[string]any{"ID": 1}, &MultiReadResult{}

				batchGetLatestValueRequest := make(types.BatchGetLatestValuesRequest)
				batchGetLatestValueRequest[bound] = []types.BatchRead{
					{
						ReadName:  MethodReturningUint64,
						Params:    nil,
						ReturnVal: &actual,
					},
					{
						ReadName:  MultiReadWithParamsReuse,
						Params:    multiParams,
						ReturnVal: multiActual,
					},
				}

				result, err := cr.BatchGetLatestValues(ctx, batchGetLatestValueRequest)

				require.NoError(t, err)

				expectedMRR := MultiReadResult{A: 10, B: 20, U: "olleH", V: true}
				anyContractBatch := result[bound]

				returnValue, err := anyContractBatch[1].GetResult()
				assert.NoError(t, err)
				assert.Contains(t, anyContractBatch[1].ReadName, MultiReadWithParamsReuse)
				require.Equal(t, &expectedMRR, returnValue)

				returnValue, err = anyContractBatch[0].GetResult()
				assert.NoError(t, err)
				assert.Contains(t, anyContractBatch[0].ReadName, MethodReturningUint64)
				assert.Equal(t, AnyValueToReadWithoutAnArgument, *returnValue.(*uint64))
			},
		},
		{
			Name: ContractReaderGetLatestValueIncludeReverted,
			Test: func(t T) {
				cr := it.GetContractReader(t)
				cw := it.GetContractWriter(t)
				bindings := it.GetBindings(t)
				ctx := t.Context()
				bound := BindingsByName(bindings, AnyContractName)[0]

				require.NoError(t, cr.Bind(ctx, bindings))

				stateChangedEvent := struct {
					NewState string
				}{}
				err := cr.GetLatestValue(ctx, bound.ReadIdentifier(StateChangedEventName), primitives.Finalized, nil, &stateChangedEvent)
				require.ErrorContains(t, err, "NotFound")

				SubmitTransactionAndExpectFailure(t, it, cw, MethodTriggeringEventBeforeFailing, nil, bound)

				assert.Eventually(t, func() bool {
					err = cr.GetLatestValue(ctx, bound.ReadIdentifier(StateChangedEventName), primitives.Finalized, nil, &stateChangedEvent)
					if err != nil {
						//it.Helper.Logger().Debugw("Waiting for GetLatestValue to return successfully:", "err", err)
						return false
					}
					assert.Equal(t, "Pending", stateChangedEvent.NewState)
					return true
				}, 5*time.Minute, time.Second, "Timed out while waiting for StateChangedEvent to show up on chain")
				assert.NoError(t, err)
			},
		},
	}
	RunTests(t, it, testCases)
}

// Similar to SubmitTransactionToCW, but requires that the tx fails instead of succeeds.
func SubmitTransactionAndExpectFailure[T TestingT[T]](t T, tester ChainComponentsInterfaceTester[T], cw types.ContractWriter, method string, args any, contract types.BoundContract) string {
	tester.DirtyContracts()
	txID := uuid.New().String()
	err := cw.SubmitTransaction(t.Context(), contract.Name, method, args, txID, contract.Address, nil, big.NewInt(0))
	require.NoError(t, err)

	err = WaitForTransactionStatus(t, tester, cw, txID, types.Failed, false)
	require.ErrorContains(t, err, "has failed or is fatal")

	return txID
}

type SolanaChainComponentsInterfaceTesterHelper[T WrappedTestingT[T]] interface {
	Init(t T)
	RPCClient() *chainreader.RPCClientWrapper
	RPC() *rpc.Client
	Context(t T) context.Context
	Logger(t T) logger.Logger
	GetPrimaryIDL(t T) []byte
	GetSecondaryIDL(t T) []byte
	CreateAccount(t T, it SolanaChainComponentsInterfaceTester[T], contractName string, value uint64) solana.PublicKey
	TXM() *txm.TxManager
	MultiClient() *client.MultiClient
	SolanaClient() *client.Client
	Sender() solana.PrivateKey
	Database() *sqlx.DB
	DataWrap(testStruct any) map[string]any
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
	inMemoryDB    bool
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
	chainID, err := it.Helper.MultiClient().ChainID(it.Helper.Context(t))

	require.NoError(t, err)

	orm := logpoller.NewORM(chainID.String(), it.Helper.Database(), it.Helper.Logger(t))
	lp, err := logpoller.New(logger.Sugared(it.Helper.Logger(t)), orm, it.Helper.MultiClient(), config.NewDefault(), chainID.String())
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = lp.Close
	})
	svc, err := chainreader.NewContractReaderService(
		it.Helper.Logger(t),
		it.Helper.RPCClient(),
		contractReaderConfig,
		lp)

	require.NoError(t, err)
	servicetest.Run(t, svc)

	return svc
}

func (it *SolanaChainComponentsInterfaceTester[T]) GetContractReaderWithCustomCfg(t T, contractReaderConfig config.ContractReader) types.ContractReader {
	ctx := it.Helper.Context(t)
	chainID, err := it.Helper.MultiClient().ChainID(it.Helper.Context(t))

	require.NoError(t, err)

	orm := logpoller.NewORM(chainID.String(), it.Helper.Database(), it.Helper.Logger(t))
	lp, err := logpoller.New(logger.Sugared(it.Helper.Logger(t)), orm, it.Helper.MultiClient(), config.NewDefault(), chainID.String())
	require.NoError(t, err)

	svc, err := chainreader.NewContractReaderService(
		it.Helper.Logger(t),
		it.Helper.RPCClient(),
		contractReaderConfig,
		lp)

	require.NoError(t, err)
	require.NoError(t, svc.Start(ctx))

	return svc
}

func (it *SolanaChainComponentsInterfaceTester[T]) GetContractWriter(t T) types.ContractWriter {
	chainWriterConfig := it.buildContractWriterConfig(t)
	cw, err := chainwriter.NewSolanaChainWriterService(it.Helper.Logger(t), *it.Helper.MultiClient(), *it.Helper.TXM(), nil, chainWriterConfig)
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
	return []types.BoundContract{
		{Name: AnyContractName, Address: it.Helper.CreateAccount(t, *it, AnyContractName, AnyValueToReadWithoutAnArgument).String()},
		{Name: AnySecondContractName, Address: it.Helper.CreateAccount(t, *it, AnySecondContractName, AnyDifferentValueToReadWithoutAnArgument).String()},
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
	sender             solana.PrivateKey
	db                 *sqlx.DB
	inMemoryDB         bool
}

func (h *helper) Init(t *testing.T) {
	t.Helper()

	dbURL := sqltest.TestURL(t)
	h.db = sqltest.NewDB(t, dbURL)

	if dbURL == pg.DriverInMemoryPostgres {
		h.inMemoryDB = true
	}

	privateKey, err := solana.PrivateKeyFromBase58(solclient.DefaultPrivateKeysSolValidator[1])
	require.NoError(t, err)
	h.sender = privateKey

	h.rpcURL, h.wsURL = utils.SetupTestValidatorWithAnchorPrograms(t, privateKey.PublicKey().String(), []string{"contract-reader-interface", "contract-reader-interface-secondary"})
	h.wsClient, err = ws.Connect(t.Context(), h.wsURL)
	h.rpcClient = rpc.New(h.rpcURL)
	lggr := logger.Test(t)

	require.NoError(t, err)

	utils.FundAccounts(t, []solana.PrivateKey{privateKey}, h.rpcClient)

	cfg := config.NewDefault()
	cfg.Chain.TxRetentionTimeout = commonconfig.MustNewDuration(10 * time.Minute)
	solanaClient, err := client.NewClient(h.rpcURL, cfg, 5*time.Second, lggr)
	require.NoError(t, err)

	h.sc = solanaClient

	loader := solanautils.NewLoader[client.ReaderWriter](func(ctx context.Context) (client.ReaderWriter, error) { return solanaClient, nil })
	mkey := keyMocks.NewSimpleKeystore(t)
	mkey.On("Sign", mock.Anything, privateKey.PublicKey().String(), mock.Anything).Return(func(_ context.Context, _ string, data []byte) []byte {
		sig, _ := privateKey.Sign(data)
		return sig[:]
	}, nil)

	txm, err := txm.NewTxm("localnet", loader, nil, cfg, mkey, lggr)
	require.NoError(t, err)
	err = txm.Start(t.Context())
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

func (h *helper) InMemoryDB() bool {
	return h.inMemoryDB
}

func (h *helper) RPCClient() *chainreader.RPCClientWrapper {
	return &chainreader.RPCClientWrapper{AccountReader: h.rpcClient}
}

func (h *helper) RPC() *rpc.Client {
	return h.rpcClient
}

func (h *helper) TXM() *txm.TxManager {
	return &h.txm
}

func (h *helper) MultiClient() *client.MultiClient {
	return client.NewMultiClient(func(context.Context) (client.ReaderWriter, error) {
		return h.sc, nil
	})
}

func (h *helper) SolanaClient() *client.Client {
	return h.sc
}

func (h *helper) Context(t *testing.T) context.Context {
	return t.Context()
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

func (h *helper) CreateAccount(t *testing.T, it SolanaChainComponentsInterfaceTester[*testing.T], contractName string, value uint64) solana.PublicKey {
	t.Helper()

	var programID solana.PublicKey
	switch contractName {
	case AnyContractName:
		programID = h.primaryProgramID
		h.initOnce.Do(func() {
			cw := it.GetContractWriter(t)
			SubmitTransactionToCW(t, &it, cw, "initializeMultiRead", nil, types.BoundContract{Name: contractName, Address: programID.String()}, types.Finalized)
			SubmitTransactionToCW(t, &it, cw, "initializeMultiReadWithParams", nil, types.BoundContract{Name: contractName, Address: programID.String()}, types.Finalized)
			SubmitTransactionToCW(t, &it, cw, "initializeTokenPrices", nil, types.BoundContract{Name: contractName, Address: programID.String()}, types.Finalized)
		})
	case AnySecondContractName:
		programID = h.secondaryProgramID
	}

	h.runInitialize(t, it, contractName, programID, value)
	return programID
}

func (h *helper) Sender() solana.PrivateKey {
	return h.sender
}

func (h *helper) Database() *sqlx.DB {
	return h.db
}

func (h *helper) DataWrap(testStruct any) map[string]any {
	return map[string]any{
		"Data": testStruct,
	}
}

type DataAccountArgs struct {
	TestIdx uint64
	Value   uint64
}

type LookupTableArgs struct {
	LookupTable solana.PublicKey
}

type StoreTokenAccountArgs struct {
	TestIdx      uint64
	TokenAccount solana.PublicKey
	ATAInfo      ATAInfo
}

type ATAInfo struct {
	Receiver     solana.PublicKey
	Wallet       solana.PublicKey
	TokenProgram solana.PublicKey
	Mint         solana.PublicKey
}

func (h *helper) runInitialize(
	t *testing.T,
	it SolanaChainComponentsInterfaceTester[*testing.T],
	contractName string,
	programID solana.PublicKey,
	value uint64,
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

	initArgs := DataAccountArgs{
		TestIdx: testIdx,
		Value:   value,
	}
	SubmitTransactionToCW(t, &it, cw, "initialize", initArgs, types.BoundContract{Name: contractName, Address: programID.String()}, types.Finalized)
}

const (
	ReadUninitializedPDA                 = "ReadUninitializedPDA"
	MultiRead                            = "MultiRead"
	ReadWithAddressHardCodedIntoResponse = "ReadWithAddressHardCodedIntoResponse"
	MultiReadWithParamsReuse             = "MultiReadWithParamsReuse"
	GetTokenPrices                       = "GetTokenPrices"
	StateChangedEventName                = "StateChangedEvent"
	MethodTriggeringEventBeforeFailing   = "triggerEventAndFail"
)

func (it *SolanaChainComponentsInterfaceTester[T]) buildContractReaderConfig(t T) config.ContractReader {
	idx := it.getTestIdx(t.Name())
	pdaDataPrefix := []byte("data")
	pdaDataPrefix = binary.LittleEndian.AppendUint64(pdaDataPrefix, idx)
	pdaStructDataPrefix := []byte("struct_data")
	pdaStructDataPrefix = binary.LittleEndian.AppendUint64(pdaStructDataPrefix, idx)
	uint64ReadDef := config.ReadDefinition{
		ChainSpecificName: "DataAccount",
		ReadType:          config.Account,
		PDADefinition: codecv1.PDATypeDef{
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

	readWithAddressHardCodedIntoResponseDef := config.ReadDefinition{
		ChainSpecificName: "MultiRead1",
		ReadType:          config.Account,
		PDADefinition: codecv1.PDATypeDef{
			Prefix: []byte("multi_read1"),
		},
		ResponseAddressHardCoder: &commoncodec.HardCodeModifierConfig{
			// placeholder values, whatever is put as value gets replaced with a solana pub key anyway
			OffChainValues: map[string]any{
				"SharedAddress":  "",
				"AddressToShare": "",
			},
		},
	}

	multiReadDef := readWithAddressHardCodedIntoResponseDef
	multiReadDef.ResponseAddressHardCoder = nil
	multiReadDef.OutputModifications = commoncodec.ModifiersConfig{
		&commoncodec.HardCodeModifierConfig{
			OffChainValues: map[string]any{"U": "", "V": false},
		},
	}
	multiReadDef.MultiReader = &config.MultiReader{
		Reads: []config.ReadDefinition{{
			ChainSpecificName: "MultiRead2",
			PDADefinition:     codecv1.PDATypeDef{Prefix: []byte("multi_read2")},
			ReadType:          config.Account,
		}},
	}

	idl := mustUnmarshalIDL(t, string(it.Helper.GetPrimaryIDL(t)))
	idl.Accounts = append(idl.Accounts, codecv1.IdlTypeDef{
		Name: "USDPerToken",
		Type: codecv1.IdlTypeDefTy{
			Kind: codecv1.IdlTypeDefTyKindStruct,
			Fields: &codecv1.IdlTypeDefStruct{
				{
					Name: "tokenPrices",
					Type: codecv1.IdlType{
						AsIdlTypeVec: &codecv1.IdlTypeVec{Vec: codecv1.IdlType{AsIdlTypeDefined: &codecv1.IdlTypeDefined{Defined: "TimestampedPackedU224"}}},
					},
				},
			},
		},
	})

	cfg := config.ContractReader{
		Namespaces: map[string]config.ChainContractReader{
			AnyContractName: {
				IDL: idl,
				Reads: map[string]config.ReadDefinition{
					ReadUninitializedPDA: {
						ChainSpecificName: "DataAccount",
						ReadType:          config.Account,
						PDADefinition: codecv1.PDATypeDef{
							Prefix: []byte("AAAAAAAAAA"),
						},
					},
					ReadWithAddressHardCodedIntoResponse: readWithAddressHardCodedIntoResponseDef,
					GetTokenPrices: {
						ChainSpecificName: "USDPerToken",
						ReadType:          config.Account,
						PDADefinition: codecv1.PDATypeDef{
							Prefix: []byte("fee_billing_token_config"),
							Seeds: []codecv1.PDASeed{
								{
									Name: "Tokens",
									Type: codecv1.IdlType{
										AsIdlTypeVec: &codecv1.IdlTypeVec{
											Vec: codecv1.IdlType{AsString: codecv1.IdlTypePublicKey},
										},
									},
								},
							},
						},
						OutputModifications: commoncodec.ModifiersConfig{
							&commoncodec.PropertyExtractorConfig{FieldName: "TokenPrices"},
						},
					},
					MultiRead: multiReadDef,
					MultiReadWithParamsReuse: {
						ChainSpecificName: "MultiRead3",
						PDADefinition: codecv1.PDATypeDef{
							Prefix: []byte("multi_read_with_params3"),
							Seeds:  []codecv1.PDASeed{{Name: "ID", Type: codecv1.IdlType{AsString: codecv1.IdlTypeU64}}},
						},
						OutputModifications: commoncodec.ModifiersConfig{
							&commoncodec.HardCodeModifierConfig{
								OffChainValues: map[string]any{"U": "", "V": false},
							},
						},
						MultiReader: &config.MultiReader{
							ReuseParams: true,
							Reads: []config.ReadDefinition{
								{
									ChainSpecificName: "MultiRead4",
									PDADefinition: codecv1.PDATypeDef{
										Prefix: []byte("multi_read_with_params4"),
										Seeds:  []codecv1.PDASeed{{Name: "ID", Type: codecv1.IdlType{AsString: codecv1.IdlTypeU64}}},
									},
									ReadType: config.Account,
								},
							}},
						ReadType: config.Account,
					},
					MethodReturningUint64: uint64ReadDef,
					MethodReturningUint64Slice: {
						ChainSpecificName: "DataAccount",
						ReadType:          config.Account,
						PDADefinition: codecv1.PDATypeDef{
							Prefix: pdaDataPrefix,
						},
						OutputModifications: commoncodec.ModifiersConfig{
							&commoncodec.PropertyExtractorConfig{FieldName: "U64Slice"},
						},
					},
					MethodSettingUint64: {
						ChainSpecificName: "DataAccount",
						ReadType:          config.Account,
						PDADefinition: codecv1.PDATypeDef{
							Prefix: pdaDataPrefix,
						},
						OutputModifications: commoncodec.ModifiersConfig{
							&commoncodec.PropertyExtractorConfig{FieldName: "U64Value"},
						},
					},
					MethodReturningSeenStruct: {
						ChainSpecificName: "TestStruct",
						ReadType:          config.Account,
						PDADefinition: codecv1.PDATypeDef{
							Prefix: pdaStructDataPrefix,
						},
						OutputModifications: commoncodec.ModifiersConfig{
							&commoncodec.HardCodeModifierConfig{
								OffChainValues: map[string]any{
									"ExtraField": AnyExtraValue,
								},
							},
							&commoncodec.AddressBytesToStringModifierConfig{
								Fields: []string{"AccountStruct.AccountStr"},
							},
						},
					},
					MethodTakingLatestParamsReturningTestStruct: {
						ChainSpecificName:       "TestStruct",
						ReadType:                config.Account,
						ErrOnMissingAccountData: true,
						PDADefinition: codecv1.PDATypeDef{
							Prefix: pdaStructDataPrefix,
							Seeds: []codecv1.PDASeed{
								{
									Name: "I", // this is read from params passed in while reading the account, its analogous to ListIdx which is used while writing to chain
									Type: codecv1.IdlType{AsString: codecv1.IdlTypeU64},
								},
							},
						},
						OutputModifications: commoncodec.ModifiersConfig{
							&commoncodec.AddressBytesToStringModifierConfig{
								Fields: []string{"AccountStruct.AccountStr"},
							},
						},
					},
					StateChangedEventName: {
						ChainSpecificName: "StateChangedEvent",
						ReadType:          config.Event,
						EventDefinitions: &config.EventDefinitions{
							PollingFilter: &config.PollingFilter{
								IncludeReverted: &trueVal,
							},
						},
					},
					EventName: {
						ChainSpecificName: "SomeEvent",
						ReadType:          config.Event,
						EventDefinitions: &config.EventDefinitions{
							PollingFilter: &config.PollingFilter{
								IncludeReverted: &trueVal,
							},
							IndexedField0: &config.IndexedField{
								OffChainPath: "Field",
								OnChainPath:  "Field",
							},
						},
						OutputModifications: commoncodec.ModifiersConfig{
							&commoncodec.HardCodeModifierConfig{
								OffChainValues: map[string]any{
									"ExtraField": AnyExtraValue,
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
						PDADefinition: codecv1.PDATypeDef{
							Prefix: pdaDataPrefix,
						},
						OutputModifications: commoncodec.ModifiersConfig{
							&commoncodec.PropertyExtractorConfig{FieldName: "U64Value"},
						},
					},
					MethodTakingLatestParamsReturningTestStruct: {
						ChainSpecificName:       "TestStruct",
						ReadType:                config.Account,
						ErrOnMissingAccountData: true,
						PDADefinition: codecv1.PDATypeDef{
							Prefix: pdaStructDataPrefix,
							Seeds: []codecv1.PDASeed{
								{
									Name: "I",
									Type: codecv1.IdlType{AsString: codecv1.IdlTypeU64},
								},
							},
						},
						OutputModifications: commoncodec.ModifiersConfig{
							&commoncodec.AddressBytesToStringModifierConfig{
								Fields: []string{"AccountStruct.AccountStr"},
							},
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
	if it.inMemoryDB {
		delete(cfg.Namespaces[AnyContractName].Reads, StateChangedEventName)
	}
	return cfg
}

const (
	GetTokenPricesPubKey1 = "57FUKrjY7Dywph1bqNGztvtTGWcXvk5VLNCfAXtk6jqK"
	GetTokenPricesPubKey2 = "47XyyAALxH7WeNT1DGWsPeA8veSVJaF8MHFMqBM5DkP6"
)

func (it *SolanaChainComponentsInterfaceTester[T]) buildContractWriterConfig(t T) chainwriter.ChainWriterConfig {
	idx := it.getTestIdx(t.Name())
	testIdx := binary.LittleEndian.AppendUint64([]byte{}, idx)
	fromAddress := solana.MustPrivateKeyFromBase58(solclient.DefaultPrivateKeysSolValidator[1]).PublicKey().String()
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
							{AccountConstant: &chainwriter.AccountConstant{
								Name:       "Signer",
								Address:    fromAddress,
								IsSigner:   true,
								IsWritable: true,
							}},
							{PDALookups: &chainwriter.PDALookups{
								Name: "Account",
								PublicKey: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
									Address: primaryProgramPubKey,
								}},
								Seeds: []chainwriter.Seed{
									{Static: []byte("data")},
									{Static: testIdx},
								},
								IsWritable: true,
								IsSigner:   false,
							}},
							{AccountConstant: &chainwriter.AccountConstant{
								Name:       "SystemProgram",
								Address:    solana.SystemProgramID.String(),
								IsWritable: false,
								IsSigner:   false,
							}},
						},
						DebugIDLocation: "",
					},
					"initializeMultiRead": {
						FromAddress:        fromAddress,
						InputModifications: nil,
						ChainSpecificName:  "initializemultiread",
						LookupTables:       chainwriter.LookupTables{},
						Accounts: []chainwriter.Lookup{
							{AccountConstant: &chainwriter.AccountConstant{
								Name:       "Signer",
								Address:    fromAddress,
								IsSigner:   true,
								IsWritable: true,
							}},
							{PDALookups: &chainwriter.PDALookups{
								Name: "MultiRead1",
								PublicKey: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
									Name:    "ProgramID",
									Address: primaryProgramPubKey,
								}},
								Seeds: []chainwriter.Seed{
									{Static: []byte("multi_read1")},
								},
								IsWritable: true,
								IsSigner:   false,
							}},
							{PDALookups: &chainwriter.PDALookups{
								Name: "MultiRead2",
								PublicKey: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
									Name:    "ProgramID",
									Address: primaryProgramPubKey,
								}},
								Seeds: []chainwriter.Seed{
									{Static: []byte("multi_read2")},
								},
								IsWritable: true,
								IsSigner:   false,
							}},
							{AccountConstant: &chainwriter.AccountConstant{
								Name:       "SystemProgram",
								Address:    solana.SystemProgramID.String(),
								IsWritable: false,
								IsSigner:   false,
							}},
						},
						DebugIDLocation: "",
					},
					"initializeMultiReadWithParams": {
						FromAddress:        fromAddress,
						InputModifications: nil,
						ChainSpecificName:  "initializemultireadwithparams",
						LookupTables:       chainwriter.LookupTables{},
						Accounts: []chainwriter.Lookup{
							{AccountConstant: &chainwriter.AccountConstant{
								Name:       "Signer",
								Address:    fromAddress,
								IsSigner:   true,
								IsWritable: true,
							}},
							{PDALookups: &chainwriter.PDALookups{
								Name: "MultiRead3",
								PublicKey: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
									Name:    "ProgramID",
									Address: primaryProgramPubKey,
								}},
								Seeds: []chainwriter.Seed{
									{Static: []byte("multi_read_with_params3")},
									{Static: binary.LittleEndian.AppendUint64([]byte{}, 1)},
								},
								IsWritable: true,
								IsSigner:   false,
							}},
							{PDALookups: &chainwriter.PDALookups{
								Name: "MultiRead4",
								PublicKey: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
									Name:    "ProgramID",
									Address: primaryProgramPubKey,
								}},
								Seeds: []chainwriter.Seed{
									{Static: []byte("multi_read_with_params4")},
									{Static: binary.LittleEndian.AppendUint64([]byte{}, 1)},
								},
								IsWritable: true,
								IsSigner:   false,
							}},
							{AccountConstant: &chainwriter.AccountConstant{
								Name:       "SystemProgram",
								Address:    solana.SystemProgramID.String(),
								IsWritable: false,
								IsSigner:   false,
							}},
						},
						DebugIDLocation: "",
					},
					"initializeTokenPrices": {
						FromAddress:        fromAddress,
						InputModifications: nil,
						ChainSpecificName:  "initializetokenprices",
						LookupTables:       chainwriter.LookupTables{},
						Accounts: []chainwriter.Lookup{
							{AccountConstant: &chainwriter.AccountConstant{
								Name:       "Signer",
								Address:    fromAddress,
								IsSigner:   true,
								IsWritable: true,
							}},
							{PDALookups: &chainwriter.PDALookups{
								Name: "BillingTokenConfigWrapper1",
								PublicKey: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
									Name:    "ProgramID",
									Address: primaryProgramPubKey,
								}},
								Seeds: []chainwriter.Seed{
									{Static: []byte("fee_billing_token_config")},
									{Static: pubKey1.Bytes()},
								},
								IsWritable: true,
								IsSigner:   false,
							}},
							{PDALookups: &chainwriter.PDALookups{
								Name: "BillingTokenConfigWrapper2",
								PublicKey: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
									Name:    "ProgramID",
									Address: primaryProgramPubKey,
								}},
								Seeds: []chainwriter.Seed{
									{Static: []byte("fee_billing_token_config")},
									{Static: pubKey2.Bytes()},
								},
								IsWritable: true,
								IsSigner:   false,
							}},
							{AccountConstant: &chainwriter.AccountConstant{
								Name:       "SystemProgram",
								Address:    solana.SystemProgramID.String(),
								IsWritable: false,
								IsSigner:   false,
							}},
						},
						DebugIDLocation: "",
					},
					"initializeLookupTable": {
						FromAddress:        fromAddress,
						InputModifications: nil,
						ChainSpecificName:  "initializelookuptable",
						LookupTables:       chainwriter.LookupTables{},
						Accounts: []chainwriter.Lookup{
							{AccountConstant: &chainwriter.AccountConstant{
								Name:       "Signer",
								Address:    fromAddress,
								IsSigner:   true,
								IsWritable: true,
							}},
							{PDALookups: &chainwriter.PDALookups{
								Name: "Account",
								PublicKey: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
									Address: primaryProgramPubKey,
								}},
								Seeds: []chainwriter.Seed{
									{Static: []byte("lookup")},
								},
								IsWritable: true,
								IsSigner:   false,
							}},
							{AccountConstant: &chainwriter.AccountConstant{
								Name:       "SystemProgram",
								Address:    solana.SystemProgramID.String(),
								IsWritable: false,
								IsSigner:   false,
							}},
						},
						DebugIDLocation: "",
					},
					"storeVal": {
						FromAddress:        fromAddress,
						InputModifications: nil,
						ChainSpecificName:  "storeval",
						LookupTables: chainwriter.LookupTables{
							DerivedLookupTables: []chainwriter.DerivedLookupTable{
								{
									Name: "LookupTable",
									Accounts: chainwriter.Lookup{PDALookups: &chainwriter.PDALookups{
										Name: "LookupTableAccount",
										PublicKey: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
											Address: primaryProgramPubKey,
										}},
										Seeds: []chainwriter.Seed{
											{Static: []byte("lookup")},
										},
										InternalField: chainwriter.InternalField{
											TypeName: "LookupTableDataAccount",
											Location: "LookupTable",
											IDL:      string(it.Helper.GetPrimaryIDL(t)),
										},
									}},
								},
							},
						},
						Accounts: []chainwriter.Lookup{
							{AccountConstant: &chainwriter.AccountConstant{
								Name:       "Signer",
								Address:    fromAddress,
								IsSigner:   true,
								IsWritable: true,
							}},
							{PDALookups: &chainwriter.PDALookups{
								Name: "Account",
								PublicKey: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
									Address: primaryProgramPubKey,
								}},
								Seeds: []chainwriter.Seed{
									{Static: []byte("data")},
									{Static: testIdx},
								},
								IsWritable: true,
								IsSigner:   false,
							}},
							{AccountConstant: &chainwriter.AccountConstant{
								Name:       "SystemProgram",
								Address:    solana.SystemProgramID.String(),
								IsWritable: false,
								IsSigner:   false,
							}},
						},
						DebugIDLocation: "",
					},
					"storeTokenAccount": {
						FromAddress:       fromAddress,
						ChainSpecificName: "storeTokenAccount",
						ATAs: []chainwriter.ATALookup{
							{
								Location:      "ATAInfo.Receiver",
								WalletAddress: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{Location: "ATAInfo.Wallet"}},
								TokenProgram:  chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{Location: "ATAInfo.TokenProgram"}},
								MintAddress:   chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{Location: "ATAInfo.Mint"}},
							},
						},
						Accounts: []chainwriter.Lookup{
							{AccountConstant: &chainwriter.AccountConstant{
								Name:       "Signer",
								Address:    fromAddress,
								IsSigner:   true,
								IsWritable: true,
							}},
							{AccountLookup: &chainwriter.AccountLookup{
								Location:   "TokenAccount",
								IsWritable: chainwriter.MetaBool{Value: true},
								IsSigner:   chainwriter.MetaBool{Value: false},
							}},
							{PDALookups: &chainwriter.PDALookups{
								Name: "Account",
								PublicKey: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
									Address: primaryProgramPubKey,
								}},
								Seeds: []chainwriter.Seed{
									{Static: []byte("token_account")},
									{Static: testIdx},
								},
								IsWritable: true,
								IsSigner:   false,
							}},
							{AccountConstant: &chainwriter.AccountConstant{
								Name:       "SystemProgram",
								Address:    solana.SystemProgramID.String(),
								IsWritable: false,
								IsSigner:   false,
							}},
						},
						DebugIDLocation: "",
					},
					MethodSettingStruct: {
						FromAddress: fromAddress,
						InputModifications: []commoncodec.ModifierConfig{
							&commoncodec.HardCodeModifierConfig{
								OnChainValues: map[string]any{
									"TestIdx": idx,
								},
							},
							&commoncodec.AddressBytesToStringModifierConfig{
								Fields: []string{"Data.AccountStruct.AccountStr"},
							},
						},
						ChainSpecificName: "store",
						LookupTables:      chainwriter.LookupTables{},
						Accounts: []chainwriter.Lookup{
							{AccountConstant: &chainwriter.AccountConstant{
								Name:       "Signer",
								Address:    fromAddress,
								IsSigner:   true,
								IsWritable: true,
							}},
							{PDALookups: &chainwriter.PDALookups{
								Name: "Account",
								PublicKey: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
									Name:    "ProgramID",
									Address: primaryProgramPubKey,
								}},
								Seeds: []chainwriter.Seed{
									{Static: []byte("struct_data")},
									{Static: testIdx},
									{Dynamic: chainwriter.Lookup{
										AccountLookup: &chainwriter.AccountLookup{
											Location: "ListIdx", // this will be sent as input params while writing to chain, see StoreTestStruct
										},
									}},
								},
								IsWritable: true,
								IsSigner:   false,
							}},
							{AccountConstant: &chainwriter.AccountConstant{
								Name:       "SystemProgram",
								Address:    solana.SystemProgramID.String(),
								IsWritable: false,
								IsSigner:   false,
							}},
						},
						DebugIDLocation: "",
					},
					MethodTriggeringEventBeforeFailing: {
						FromAddress:       fromAddress,
						ChainSpecificName: "createEventAndFail",
						LookupTables:      chainwriter.LookupTables{},
						Accounts: []chainwriter.Lookup{
							{AccountConstant: &chainwriter.AccountConstant{
								Name:       "Signer",
								Address:    fromAddress,
								IsSigner:   true,
								IsWritable: true,
							}},
							{AccountConstant: &chainwriter.AccountConstant{
								Name:       "SystemProgram",
								Address:    solana.SystemProgramID.String(),
								IsWritable: false,
								IsSigner:   false,
							}},
						},
						DebugIDLocation: "",
					},
					MethodTriggeringEvent: {
						FromAddress:       fromAddress,
						ChainSpecificName: "triggerEvent",
						InputModifications: []commoncodec.ModifierConfig{
							&commoncodec.AddressBytesToStringModifierConfig{
								Fields: []string{"Data.AccountStruct.AccountStr"},
							},
						},
						LookupTables: chainwriter.LookupTables{},
						Accounts: []chainwriter.Lookup{
							{AccountConstant: &chainwriter.AccountConstant{
								Name:       "Signer",
								Address:    fromAddress,
								IsSigner:   true,
								IsWritable: true,
							}},
							{AccountConstant: &chainwriter.AccountConstant{
								Name:       "SystemProgram",
								Address:    solana.SystemProgramID.String(),
								IsWritable: false,
								IsSigner:   false,
							}},
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
							{AccountConstant: &chainwriter.AccountConstant{
								Name:       "Signer",
								Address:    fromAddress,
								IsSigner:   true,
								IsWritable: true,
							}},
							{PDALookups: &chainwriter.PDALookups{
								Name: "Account",
								PublicKey: chainwriter.Lookup{
									AccountConstant: &chainwriter.AccountConstant{
										Name:    "ProgramID",
										Address: secondaryProgramPubKey, // line ~1338
									}, // line ~1339 closes AccountConstant
								}, // line ~1340 closes chainwriter.Lookup
								Seeds: []chainwriter.Seed{
									{Static: []byte("data")},
									{Static: testIdx},
								},
								IsWritable: true,
								IsSigner:   false,
							}},
							{AccountConstant: &chainwriter.AccountConstant{
								Name:       "SystemAccount",
								Address:    solana.SystemProgramID.String(),
								IsWritable: false,
								IsSigner:   false,
							}},
						},
						DebugIDLocation: "",
					},
					MethodSettingStruct: {
						FromAddress: fromAddress,
						InputModifications: []commoncodec.ModifierConfig{
							&commoncodec.HardCodeModifierConfig{
								OnChainValues: map[string]any{
									"TestIdx": idx,
								},
							},
							&commoncodec.AddressBytesToStringModifierConfig{
								Fields: []string{"Data.AccountStruct.AccountStr"},
							},
						},
						ChainSpecificName: "store",
						LookupTables:      chainwriter.LookupTables{},
						Accounts: []chainwriter.Lookup{
							{AccountConstant: &chainwriter.AccountConstant{
								Name:       "Signer",
								Address:    fromAddress,
								IsSigner:   true,
								IsWritable: true,
							}},
							{PDALookups: &chainwriter.PDALookups{
								Name: "Account",
								PublicKey: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
									Name:    "ProgramID",
									Address: secondaryProgramPubKey,
								}},
								Seeds: []chainwriter.Seed{
									{Static: []byte("struct_data")},
									{Static: testIdx},
									{Dynamic: chainwriter.Lookup{
										AccountLookup: &chainwriter.AccountLookup{
											Location: "ListIdx",
										},
									}},
								},
								IsWritable: true,
								IsSigner:   false,
							}},
							{AccountConstant: &chainwriter.AccountConstant{
								Name:       "SystemProgram",
								Address:    solana.SystemProgramID.String(),
								IsWritable: false,
								IsSigner:   false,
							}},
						},
						DebugIDLocation: "",
					},
				},
			},
		},
	}
}

func mustUnmarshalIDL[T WrappedTestingT[T]](t T, rawIDL string) codecv1.IDL {
	var idl codecv1.IDL
	if err := json.Unmarshal([]byte(rawIDL), &idl); err != nil {
		t.Errorf("failed to unmarshal test IDL", err)
		t.FailNow()
	}

	return idl
}

func CreateTestLookupTable[T WrappedTestingT[T]](ctx context.Context, t T, c *client.Client, txm txm.TxManager, sender solana.PrivateKey, addresses []solana.PublicKey) solana.PublicKey {
	// Create lookup tables
	slot, serr := c.SlotHeightWithCommitment(ctx, rpc.CommitmentFinalized)
	require.NoError(t, serr)
	table, createTableInstruction, ierr := utils.NewCreateLookupTableInstruction(
		sender.PublicKey(),
		sender.PublicKey(),
		slot,
	)
	require.NoError(t, ierr)
	res, err := c.LatestBlockhash(ctx)
	require.NoError(t, err)

	tx1, err1 := solana.NewTransaction([]solana.Instruction{createTableInstruction}, res.Value.Blockhash)
	require.NoError(t, err1)
	txID1 := uuid.NewString()
	err = txm.Enqueue(ctx, "", tx1, &txID1, res.Value.LastValidBlockHeight)
	require.NoError(t, err)
	pollTxStatusTillCommitment(ctx, t, txm, txID1, types.Finalized)

	res, err = c.LatestBlockhash(ctx)
	require.NoError(t, err)

	addEntriesInstruction := utils.NewExtendLookupTableInstruction(table, sender.PublicKey(), sender.PublicKey(), addresses)
	tx2, err2 := solana.NewTransaction([]solana.Instruction{addEntriesInstruction}, res.Value.Blockhash)
	require.NoError(t, err2)
	txID2 := uuid.NewString()
	err = txm.Enqueue(ctx, "", tx2, &txID2, res.Value.LastValidBlockHeight)
	require.NoError(t, err)
	pollTxStatusTillCommitment(ctx, t, txm, txID2, types.Finalized)

	return table
}

func pollTxStatusTillCommitment[T WrappedTestingT[T]](ctx context.Context, t T, txm txm.TxManager, txID string, targetStatus types.TransactionStatus) {
	var txStatus types.TransactionStatus
	count := 0
	for txStatus != targetStatus && txStatus != types.Finalized {
		count++
		status, err := txm.GetTransactionStatus(ctx, txID)
		if err == nil {
			txStatus = status
		}
		time.Sleep(100 * time.Millisecond)
		if count > 500 {
			require.NoError(t, fmt.Errorf("unable to find transaction within timeout"))
		}
	}
}

const (
	primaryProgramPubKey   = "6AfuXF6HapDUhQfE4nQG9C1SGtA1YjP3icaJyRfU4RyE"
	secondaryProgramPubKey = "9SFyk8NmGYh5D612mJwUYhguCRY9cFgaS2vksrigepjf"
)
