package solana_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	ag_solana "github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	codeccommon "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	commontestutils "github.com/smartcontractkit/chainlink-common/pkg/loop/testutils"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	. "github.com/smartcontractkit/chainlink-common/pkg/types/interfacetests" //nolint common practice to import test mods with .
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

const (
	Namespace   = "NameSpace"
	NamedMethod = "NamedMethod1"
)

func TestSolanaChainReaderService_ReaderInterface(t *testing.T) {
	t.Parallel()

	it := &chainReaderInterfaceTester{}
	RunChainReaderInterfaceTests(t, it)
	RunChainReaderInterfaceTests(t, commontestutils.WrapChainReaderTesterForLoop(it))
}

func TestSolanaChainReaderService_ServiceCtx(t *testing.T) {
	t.Parallel()

	ctx := tests.Context(t)
	svc, err := solana.NewChainReaderService(logger.Test(t), new(mockedRPCClient), config.ChainReader{})

	require.NoError(t, err)
	require.NotNil(t, svc)

	require.Error(t, svc.Ready())
	require.Len(t, svc.HealthReport(), 1)
	require.Contains(t, svc.HealthReport(), solana.ServiceName)
	require.Error(t, svc.HealthReport()[solana.ServiceName])

	require.NoError(t, svc.Start(ctx))
	require.NoError(t, svc.Ready())
	require.Equal(t, map[string]error{solana.ServiceName: nil}, svc.HealthReport())

	require.Error(t, svc.Start(ctx))

	require.NoError(t, svc.Close())
	require.Error(t, svc.Ready())
	require.Error(t, svc.Close())
}

func TestSolanaChainReaderService_GetLatestValue(t *testing.T) {
	t.Parallel()

	ctx := tests.Context(t)

	// encode values from unmodified test struct to be read and decoded
	expected := codec.DefaultTestStruct

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		testCodec, conf := newTestConfAndCodec(t)
		encoded, err := testCodec.Encode(ctx, expected, codec.TestStructWithNestedStruct)

		require.NoError(t, err)

		client := new(mockedRPCClient)
		svc, err := solana.NewChainReaderService(logger.Test(t), client, conf)

		require.NoError(t, err)
		require.NotNil(t, svc)

		client.On("ReadAll", mock.Anything, mock.Anything).Return(encoded, nil)

		var result modifiedStructWithNestedStruct

		require.NoError(t, svc.GetLatestValue(ctx, Namespace, NamedMethod, nil, &result))
		assert.Equal(t, expected.InnerStruct, result.InnerStruct)
		assert.Equal(t, expected.Value, result.V)
		assert.Equal(t, expected.TimeVal, result.TimeVal)
		assert.Equal(t, expected.DurationVal, result.DurationVal)
	})

	t.Run("Error Returned From Account Reader", func(t *testing.T) {
		t.Parallel()

		_, conf := newTestConfAndCodec(t)

		client := new(mockedRPCClient)
		expectedErr := fmt.Errorf("expected error")
		svc, err := solana.NewChainReaderService(logger.Test(t), client, conf)

		require.NoError(t, err)
		require.NotNil(t, svc)

		client.On("ReadAll", mock.Anything, mock.Anything).Return(nil, expectedErr)

		var result modifiedStructWithNestedStruct

		assert.ErrorIs(t, svc.GetLatestValue(ctx, Namespace, NamedMethod, nil, &result), expectedErr)
	})

	t.Run("Method Not Found", func(t *testing.T) {
		t.Parallel()

		_, conf := newTestConfAndCodec(t)

		client := new(mockedRPCClient)
		svc, err := solana.NewChainReaderService(logger.Test(t), client, conf)

		require.NoError(t, err)
		require.NotNil(t, svc)

		var result modifiedStructWithNestedStruct

		assert.NotNil(t, svc.GetLatestValue(ctx, Namespace, "Unknown", nil, &result))
	})

	t.Run("Namespace Not Found", func(t *testing.T) {
		t.Parallel()

		_, conf := newTestConfAndCodec(t)

		client := new(mockedRPCClient)
		svc, err := solana.NewChainReaderService(logger.Test(t), client, conf)

		require.NoError(t, err)
		require.NotNil(t, svc)

		var result modifiedStructWithNestedStruct

		assert.NotNil(t, svc.GetLatestValue(ctx, "Unknown", "Unknown", nil, &result))
	})

	t.Run("Bind Success", func(t *testing.T) {
		t.Parallel()

		_, conf := newTestConfAndCodec(t)

		client := new(mockedRPCClient)
		svc, err := solana.NewChainReaderService(logger.Test(t), client, conf)

		require.NoError(t, err)
		require.NotNil(t, svc)

		pk := ag_solana.NewWallet().PublicKey()
		err = svc.Bind(ctx, []types.BoundContract{
			{
				Address: pk.String(),
				Name:    fmt.Sprintf("%s.%s.%d", Namespace, NamedMethod, 0),
			},
		})

		assert.NoError(t, err)
	})

	t.Run("Bind Errors", func(t *testing.T) {
		t.Parallel()

		_, conf := newTestConfAndCodec(t)

		client := new(mockedRPCClient)
		svc, err := solana.NewChainReaderService(logger.Test(t), client, conf)

		require.NoError(t, err)
		require.NotNil(t, svc)

		pk := ag_solana.NewWallet().PublicKey()

		require.NotNil(t, svc.Bind(ctx, []types.BoundContract{
			{
				Address: pk.String(),
				Name:    "incorrect format",
			},
		}))

		require.NotNil(t, svc.Bind(ctx, []types.BoundContract{
			{
				Address: pk.String(),
				Name:    fmt.Sprintf("%s.%s.%d", "Unknown", "Unknown", 0),
			},
		}))

		require.NotNil(t, svc.Bind(ctx, []types.BoundContract{
			{
				Address: pk.String(),
				Name:    fmt.Sprintf("%s.%s.%d", Namespace, "Unknown", 0),
			},
		}))

		require.NotNil(t, svc.Bind(ctx, []types.BoundContract{
			{
				Address: pk.String(),
				Name:    fmt.Sprintf("%s.%s.%d", Namespace, NamedMethod, 1),
			},
		}))

		require.NotNil(t, svc.Bind(ctx, []types.BoundContract{
			{
				Address: pk.String(),
				Name:    fmt.Sprintf("%s.%s.o", Namespace, NamedMethod),
			},
		}))

		require.NotNil(t, svc.Bind(ctx, []types.BoundContract{
			{
				Address: "invalid",
				Name:    fmt.Sprintf("%s.%s.%d", Namespace, NamedMethod, 0),
			},
		}))
	})
}

func newTestConfAndCodec(t *testing.T) (encodings.CodecFromTypeCodec, config.ChainReader) {
	t.Helper()

	rawIDL, _, testCodec := codec.NewTestIDLAndCodec(t)
	conf := config.ChainReader{
		Namespaces: map[string]config.ChainReaderMethods{
			Namespace: {
				Methods: map[string]config.ChainDataReader{
					NamedMethod: {
						AnchorIDL: rawIDL,
						Procedures: []config.ChainReaderProcedure{
							{
								IDLAccount: codec.TestStructWithNestedStruct,
								Type:       config.ProcedureTypeAnchor,
								OutputModifications: codeccommon.ModifiersConfig{
									&codeccommon.RenameModifierConfig{Fields: map[string]string{"Value": "V"}},
								},
							},
						},
					},
				},
			},
		},
	}

	return testCodec, conf
}

type modifiedStructWithNestedStruct struct {
	V                uint8
	InnerStruct      codec.ObjectRef1
	BasicNestedArray [][]uint32
	Option           *string
	DefinedArray     []codec.ObjectRef2
	BasicVector      []string
	TimeVal          int64
	DurationVal      time.Duration
	PublicKey        ag_solana.PublicKey
	EnumVal          uint8
}

type mockedRPCClient struct {
	mock.Mock
}

func (_m *mockedRPCClient) ReadAll(ctx context.Context, pk ag_solana.PublicKey) ([]byte, error) {
	ret := _m.Called(ctx, pk)

	var r0 []byte

	if val, ok := ret.Get(0).([]byte); ok {
		r0 = val
	}

	return r0, ret.Error(1)
}

type chainReaderInterfaceTester struct {
	conf    config.ChainReader
	address []string
	reader  *wrappedTestChainReader
}

func (r *chainReaderInterfaceTester) GetAccountBytes(i int) []byte {
	account := [20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	account[i%20] += byte(i)
	account[(i+3)%20] += byte(i + 3)
	return account[:]
}

func (r *chainReaderInterfaceTester) Name() string {
	return "Solana"
}

func (r *chainReaderInterfaceTester) Setup(t *testing.T) {
	r.address = make([]string, 6)
	for idx := range r.address {
		r.address[idx] = ag_solana.NewWallet().PublicKey().String()
	}

	r.conf = config.ChainReader{
		Namespaces: map[string]config.ChainReaderMethods{
			AnyContractName: {
				Methods: map[string]config.ChainDataReader{
					MethodTakingLatestParamsReturningTestStruct: {
						AnchorIDL: fmt.Sprintf(baseIDL, testStructIDL, strings.Join([]string{midLevelStructIDL, innerStructIDL}, ",")),
						Procedures: []config.ChainReaderProcedure{
							{
								IDLAccount: "TestStruct",
								Type:       config.ProcedureTypeAnchor,
							},
						},
					},
					MethodReturningUint64: {
						AnchorIDL: fmt.Sprintf(baseIDL, uint64BaseTypeIDL, ""),
						Procedures: []config.ChainReaderProcedure{
							{
								IDLAccount: "SimpleUint64Value",
								Type:       config.ProcedureTypeAnchor,
								OutputModifications: codeccommon.ModifiersConfig{
									&codeccommon.PropertyExtractorConfig{FieldName: "I"},
								},
							},
						},
					},
					DifferentMethodReturningUint64: {
						AnchorIDL: fmt.Sprintf(baseIDL, uint64BaseTypeIDL, ""),
						Procedures: []config.ChainReaderProcedure{
							{
								IDLAccount: "SimpleUint64Value",
								Type:       config.ProcedureTypeAnchor,
								OutputModifications: codeccommon.ModifiersConfig{
									&codeccommon.PropertyExtractorConfig{FieldName: "I"},
								},
							},
						},
					},
					MethodReturningUint64Slice: {
						AnchorIDL: fmt.Sprintf(baseIDL, uint64SliceBaseTypeIDL, ""),
						Procedures: []config.ChainReaderProcedure{
							{
								IDLAccount: "Uint64Slice",
								Type:       config.ProcedureTypeAnchor,
								OutputModifications: codeccommon.ModifiersConfig{
									&codeccommon.PropertyExtractorConfig{FieldName: "Vals"},
								},
							},
						},
					},
					MethodReturningSeenStruct: {
						AnchorIDL: fmt.Sprintf(baseIDL, testStructIDL, strings.Join([]string{midLevelStructIDL, innerStructIDL}, ",")),
						Procedures: []config.ChainReaderProcedure{
							{
								IDLAccount: "TestStruct",
								Type:       config.ProcedureTypeAnchor,
								OutputModifications: codeccommon.ModifiersConfig{
									&codeccommon.HardCodeModifierConfig{OffChainValues: map[string]any{"ExtraField": AnyExtraValue}},
									// &codeccommon.RenameModifierConfig{Fields: map[string]string{"NestedStruct.Inner.IntVal": "I"}},
								},
							},
						},
					},
				},
			},
			AnySecondContractName: {
				Methods: map[string]config.ChainDataReader{
					MethodReturningUint64: {
						AnchorIDL: fmt.Sprintf(baseIDL, uint64BaseTypeIDL, ""),
						Procedures: []config.ChainReaderProcedure{
							{
								IDLAccount: "SimpleUint64Value",
								Type:       config.ProcedureTypeAnchor,
								OutputModifications: codeccommon.ModifiersConfig{
									&codeccommon.PropertyExtractorConfig{FieldName: "I"},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (r *chainReaderInterfaceTester) GetChainReader(t *testing.T) types.ChainReader {
	client := new(mockedRPCClient)
	svc, err := solana.NewChainReaderService(logger.Test(t), client, r.conf)
	if err != nil {
		t.Logf("chain reader service was not able to start: %s", err.Error())
		t.FailNow()
	}

	if r.reader == nil {
		r.reader = &wrappedTestChainReader{
			test:   t,
			tester: r,
		}
	}

	r.reader.service = svc
	r.reader.client = client

	return r.reader
}

type wrappedTestChainReader struct {
	test            *testing.T
	service         *solana.SolanaChainReaderService
	client          *mockedRPCClient
	tester          ChainReaderInterfaceTester
	testStructQueue []*TestStruct
}

func (r *wrappedTestChainReader) GetLatestValue(ctx context.Context, contractName string, method string, params, returnVal any) error {
	switch contractName + method {
	case AnyContractName + EventName:
		// t.Skip won't skip the test here
		// returning the expected error to satisfy the test
		return types.ErrNotFound
	case AnyContractName + MethodReturningUint64:
		cdc := makeTestCodec(r.test, fmt.Sprintf(baseIDL, uint64BaseTypeIDL, ""))
		onChainStruct := struct {
			I uint64
		}{
			I: AnyValueToReadWithoutAnArgument,
		}

		bts, err := cdc.Encode(ctx, onChainStruct, "SimpleUint64Value")
		if err != nil {
			r.test.Log(err.Error())
			r.test.FailNow()
		}

		r.client.On("ReadAll", mock.Anything, mock.Anything).Return(bts, nil).Once()
	case AnyContractName + MethodReturningUint64Slice:
		cdc := makeTestCodec(r.test, fmt.Sprintf(baseIDL, uint64SliceBaseTypeIDL, ""))
		onChainStruct := struct {
			Vals []uint64
		}{
			Vals: AnySliceToReadWithoutAnArgument,
		}

		bts, err := cdc.Encode(ctx, onChainStruct, "Uint64Slice")
		if err != nil {
			r.test.FailNow()
		}

		r.client.On("ReadAll", mock.Anything, mock.Anything).Return(bts, nil).Once()
	case AnySecondContractName + MethodReturningUint64, AnyContractName + DifferentMethodReturningUint64:
		cdc := makeTestCodec(r.test, fmt.Sprintf(baseIDL, uint64BaseTypeIDL, ""))
		onChainStruct := struct {
			I uint64
		}{
			I: AnyDifferentValueToReadWithoutAnArgument,
		}

		bts, err := cdc.Encode(ctx, onChainStruct, "SimpleUint64Value")
		if err != nil {
			r.test.FailNow()
		}

		r.client.On("ReadAll", mock.Anything, mock.Anything).Return(bts, nil).Once()
	case AnyContractName + MethodReturningSeenStruct:
		nextStruct := CreateTestStruct(0, r.tester)
		r.testStructQueue = append(r.testStructQueue, &nextStruct)

		fallthrough
	default:
		if r.testStructQueue == nil || len(r.testStructQueue) == 0 {
			r.test.FailNow()
		}

		nextTestStruct := r.testStructQueue[0]
		r.testStructQueue = r.testStructQueue[1:len(r.testStructQueue)]

		cdc := makeTestCodec(r.test, fmt.Sprintf(baseIDL, testStructIDL, strings.Join([]string{midLevelStructIDL, innerStructIDL}, ",")))
		bts, err := cdc.Encode(ctx, nextTestStruct, "TestStruct")
		if err != nil {
			r.test.FailNow()
		}

		r.client.On("ReadAll", mock.Anything, mock.Anything).Return(bts, nil).Once()
	}

	return r.service.GetLatestValue(ctx, contractName, method, params, returnVal)
}

func (r *wrappedTestChainReader) Bind(ctx context.Context, bindings []types.BoundContract) error {
	return r.service.Bind(ctx, bindings)
}

func (r *wrappedTestChainReader) CreateContractType(contractName, itemType string, forEncoding bool) (any, error) {
	if AnyContractName+EventName == contractName+itemType {
		// events are not supported, so just make the tests pass
		return nil, types.ErrNotFound
	}

	return r.service.CreateContractType(contractName, itemType, forEncoding)
}

// SetLatestValue is expected to return the same bound contract and method in the same test
// Any setup required for this should be done in Setup.
// The contract should take a LatestParams as the params and return the nth TestStruct set
func (r *chainReaderInterfaceTester) SetLatestValue(t *testing.T, testStruct *TestStruct) {
	if r.reader == nil {
		r.reader = &wrappedTestChainReader{
			test:   t,
			tester: r,
		}
	}

	r.reader.testStructQueue = append(r.reader.testStructQueue, testStruct)
}

func (r *chainReaderInterfaceTester) TriggerEvent(t *testing.T, testStruct *TestStruct) {
	t.Skip("Events are not yet supported in Solana")
}

func (r *chainReaderInterfaceTester) GetBindings(t *testing.T) []types.BoundContract {
	return []types.BoundContract{
		{Name: strings.Join([]string{AnyContractName, MethodTakingLatestParamsReturningTestStruct, "0"}, "."), Address: r.address[0], Pending: true},
		{Name: strings.Join([]string{AnyContractName, MethodReturningUint64, "0"}, "."), Address: r.address[1], Pending: true},
		{Name: strings.Join([]string{AnyContractName, DifferentMethodReturningUint64, "0"}, "."), Address: r.address[2], Pending: true},
		{Name: strings.Join([]string{AnyContractName, MethodReturningUint64Slice, "0"}, "."), Address: r.address[3], Pending: true},
		{Name: strings.Join([]string{AnyContractName, MethodReturningSeenStruct, "0"}, "."), Address: r.address[4], Pending: true},
		{Name: strings.Join([]string{AnySecondContractName, MethodReturningUint64, "0"}, "."), Address: r.address[5], Pending: true},
	}
}

func (r *chainReaderInterfaceTester) MaxWaitTimeForEvents() time.Duration {
	// From trial and error, when running on CI, sometimes the boxes get slow
	maxWaitTime := time.Second * 20
	maxWaitTimeStr, ok := os.LookupEnv("MAX_WAIT_TIME_FOR_EVENTS_S")
	if ok {
		wiatS, err := strconv.ParseInt(maxWaitTimeStr, 10, 64)
		if err != nil {
			fmt.Printf("Error parsing MAX_WAIT_TIME_FOR_EVENTS_S: %v, defaulting to %v\n", err, maxWaitTime)
		}
		maxWaitTime = time.Second * time.Duration(wiatS)
	}

	return maxWaitTime
}

func makeTestCodec(t *testing.T, rawIDL string) encodings.CodecFromTypeCodec {
	t.Helper()

	var idl codec.IDL
	if err := json.Unmarshal([]byte(rawIDL), &idl); err != nil {
		t.Logf("failed to unmarshal test IDL: %s", err.Error())
		t.FailNow()
	}

	testCodec, err := codec.NewIDLCodec(idl)
	if err != nil {
		t.Logf("failed to create new codec from test IDL: %s", err.Error())
		t.FailNow()
	}

	return testCodec
}

const (
	baseIDL = `{
		"version": "0.1.0",
		"name": "some_test_idl",
		"accounts": [%s],
		"types": [%s]
	}`

	testStructIDL = `{
		"name": "TestStruct",
		"type": {
			"kind": "struct",
			"fields": [
				{"name": "field","type": {"option": "i32"}},
				{"name": "differentField","type": "string"},
				{"name": "oracleID","type": "u8"},
				{"name": "oracleIDs","type": {"array": ["u8",32]}},
				{"name": "account","type": "bytes"},
				{"name": "accounts","type": {"vec": "bytes"}},
				{"name": "bigField","type": "i128"},
				{"name": "nestedStruct","type": {"defined": "MidLevelStruct"}}
			]
		}
	}`

	midLevelStructIDL = `{
		"name": "MidLevelStruct",
		"type": {
			"kind": "struct",
			"fields": [
				{"name": "fixedBytes", "type": {"array": ["u8",2]}},
				{"name": "inner", "type": {"defined": "InnerTestStruct"}}
			]
		}
	}`

	innerStructIDL = `{
		"name": "InnerTestStruct",
		"type": {
			"kind": "struct",
			"fields": [
				{"name": "i", "type": "i32"},
				{"name": "s", "type": "string"}
			]
		}
	}`

	uint64BaseTypeIDL = `{
		"name": "SimpleUint64Value",
		"type": {
			"kind": "struct",
			"fields": [
				{"name": "i", "type": "u64"}
			]
		}
	}`

	uint64SliceBaseTypeIDL = `{
		"name": "Uint64Slice",
		"type": {
			"kind": "struct",
			"fields": [
				{"name": "vals", "type": {"vec": "u64"}}
			]
		}
	}`
)
