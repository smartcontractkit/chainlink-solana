package chainreader_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	ag_solana "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	codeccommon "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings/binary"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	commontestutils "github.com/smartcontractkit/chainlink-common/pkg/loop/testutils"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	. "github.com/smartcontractkit/chainlink-common/pkg/types/interfacetests" //nolint common practice to import test mods with .
	"github.com/smartcontractkit/chainlink-common/pkg/types/query"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainreader"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/testutils"
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
	lsIt := &skipEventsChainReaderTester{ChainReaderInterfaceTester: commontestutils.WrapChainReaderTesterForLoop(it)}
	RunChainReaderInterfaceTests(t, lsIt)
}

func TestSolanaChainReaderService_ServiceCtx(t *testing.T) {
	t.Parallel()

	ctx := tests.Context(t)
	svc, err := chainreader.NewChainReaderService(logger.Test(t), new(mockedRPCClient), config.ChainReader{})

	require.NoError(t, err)
	require.NotNil(t, svc)

	require.Error(t, svc.Ready())
	require.Len(t, svc.HealthReport(), 1)
	require.Contains(t, svc.HealthReport(), chainreader.ServiceName)
	require.Error(t, svc.HealthReport()[chainreader.ServiceName])

	require.NoError(t, svc.Start(ctx))
	require.NoError(t, svc.Ready())
	require.Equal(t, map[string]error{chainreader.ServiceName: nil}, svc.HealthReport())

	require.Error(t, svc.Start(ctx))

	require.NoError(t, svc.Close())
	require.Error(t, svc.Ready())
	require.Error(t, svc.Close())
}

func TestSolanaChainReaderService_GetLatestValue(t *testing.T) {
	t.Parallel()

	ctx := tests.Context(t)

	// encode values from unmodified test struct to be read and decoded
	expected := testutils.DefaultTestStruct

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		testCodec, conf := newTestConfAndCodec(t)
		encoded, err := testCodec.Encode(ctx, expected, testutils.TestStructWithNestedStruct)

		require.NoError(t, err)

		client := new(mockedRPCClient)
		svc, err := chainreader.NewChainReaderService(logger.Test(t), client, conf)

		require.NoError(t, err)
		require.NotNil(t, svc)
		require.NoError(t, svc.Start(ctx))

		t.Cleanup(func() {
			require.NoError(t, svc.Close())
		})

		client.SetNext(encoded, nil, 0)

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
		svc, err := chainreader.NewChainReaderService(logger.Test(t), client, conf)

		require.NoError(t, err)
		require.NotNil(t, svc)
		require.NoError(t, svc.Start(ctx))

		t.Cleanup(func() {
			require.NoError(t, svc.Close())
		})

		client.SetNext(nil, expectedErr, 0)

		var result modifiedStructWithNestedStruct

		assert.ErrorIs(t, svc.GetLatestValue(ctx, Namespace, NamedMethod, nil, &result), expectedErr)
	})

	t.Run("Method Not Found", func(t *testing.T) {
		t.Parallel()

		_, conf := newTestConfAndCodec(t)

		client := new(mockedRPCClient)
		svc, err := chainreader.NewChainReaderService(logger.Test(t), client, conf)

		require.NoError(t, err)
		require.NotNil(t, svc)
		require.NoError(t, svc.Start(ctx))

		t.Cleanup(func() {
			require.NoError(t, svc.Close())
		})

		var result modifiedStructWithNestedStruct

		assert.NotNil(t, svc.GetLatestValue(ctx, Namespace, "Unknown", nil, &result))
	})

	t.Run("Namespace Not Found", func(t *testing.T) {
		t.Parallel()

		_, conf := newTestConfAndCodec(t)

		client := new(mockedRPCClient)
		svc, err := chainreader.NewChainReaderService(logger.Test(t), client, conf)

		require.NoError(t, err)
		require.NotNil(t, svc)
		require.NoError(t, svc.Start(ctx))

		t.Cleanup(func() {
			require.NoError(t, svc.Close())
		})

		var result modifiedStructWithNestedStruct

		assert.NotNil(t, svc.GetLatestValue(ctx, "Unknown", "Unknown", nil, &result))
	})

	t.Run("Bind Success", func(t *testing.T) {
		t.Parallel()

		_, conf := newTestConfAndCodec(t)

		client := new(mockedRPCClient)
		svc, err := chainreader.NewChainReaderService(logger.Test(t), client, conf)

		require.NoError(t, err)
		require.NotNil(t, svc)
		require.NoError(t, svc.Start(ctx))

		t.Cleanup(func() {
			require.NoError(t, svc.Close())
		})

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
		svc, err := chainreader.NewChainReaderService(logger.Test(t), client, conf)

		require.NoError(t, err)
		require.NotNil(t, svc)
		require.NoError(t, svc.Start(ctx))

		t.Cleanup(func() {
			require.NoError(t, svc.Close())
		})

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

func newTestIDLAndCodec(t *testing.T) (string, codec.IDL, types.RemoteCodec) {
	t.Helper()

	var idl codec.IDL
	if err := json.Unmarshal([]byte(testutils.JSONIDLWithAllTypes), &idl); err != nil {
		t.Logf("failed to unmarshal test IDL: %s", err.Error())
		t.FailNow()
	}

	entry, err := codec.NewIDLAccountCodec(idl, binary.LittleEndian())
	if err != nil {
		t.Logf("failed to create new codec from test IDL: %s", err.Error())
		t.FailNow()
	}

	require.NotNil(t, entry)

	return testutils.JSONIDLWithAllTypes, idl, entry
}

func newTestConfAndCodec(t *testing.T) (types.RemoteCodec, config.ChainReader) {
	t.Helper()

	rawIDL, _, testCodec := newTestIDLAndCodec(t)
	conf := config.ChainReader{
		Namespaces: map[string]config.ChainReaderMethods{
			Namespace: {
				Methods: map[string]config.ChainDataReader{
					NamedMethod: {
						AnchorIDL: rawIDL,
						Procedures: []config.ChainReaderProcedure{
							{
								IDLAccount: testutils.TestStructWithNestedStruct,
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
	InnerStruct      testutils.ObjectRef1
	BasicNestedArray [][]uint32
	Option           *string
	DefinedArray     []testutils.ObjectRef2
	BasicVector      []string
	TimeVal          int64
	DurationVal      time.Duration
	PublicKey        ag_solana.PublicKey
	EnumVal          uint8
}

type mockedRPCCall struct {
	bts   []byte
	err   error
	delay time.Duration
}

// TODO BCI-3156 use a localnet for testing instead of a mock.
type mockedRPCClient struct {
	mu                sync.Mutex
	responseByAddress map[string]mockedRPCCall
	sequence          []mockedRPCCall
}

func (_m *mockedRPCClient) ReadAll(_ context.Context, pk ag_solana.PublicKey, _ *rpc.GetAccountInfoOpts) ([]byte, error) {
	_m.mu.Lock()
	defer _m.mu.Unlock()

	if _m.responseByAddress == nil {
		_m.responseByAddress = make(map[string]mockedRPCCall)
	}

	if resp, ok := _m.responseByAddress[pk.String()]; ok {
		if resp.delay > 0 {
			time.Sleep(resp.delay)
		}

		delete(_m.responseByAddress, pk.String())

		return resp.bts, resp.err
	}

	if len(_m.sequence) == 0 {
		panic("no values to return")
	}

	next := _m.sequence[0]
	_m.sequence = _m.sequence[1:len(_m.sequence)]

	if next.delay > 0 {
		time.Sleep(next.delay)
	}

	return next.bts, next.err
}

func (_m *mockedRPCClient) SetNext(bts []byte, err error, delay time.Duration) {
	_m.mu.Lock()
	defer _m.mu.Unlock()

	_m.sequence = append(_m.sequence, mockedRPCCall{
		bts:   bts,
		err:   err,
		delay: delay,
	})
}

func (_m *mockedRPCClient) SetForAddress(pk ag_solana.PublicKey, bts []byte, err error, delay time.Duration) {
	_m.mu.Lock()
	defer _m.mu.Unlock()

	if _m.responseByAddress == nil {
		_m.responseByAddress = make(map[string]mockedRPCCall)
	}

	_m.responseByAddress[pk.String()] = mockedRPCCall{
		bts:   bts,
		err:   err,
		delay: delay,
	}
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
	r.address = make([]string, 8)
	for idx := range r.address {
		r.address[idx] = ag_solana.NewWallet().PublicKey().String()
	}

	encodingBase64 := solana.EncodingBase64
	commitment := rpc.CommitmentConfirmed
	offset := uint64(1)
	length := uint64(1)

	r.conf = config.ChainReader{
		Namespaces: map[string]config.ChainReaderMethods{
			AnyContractName: {
				Methods: map[string]config.ChainDataReader{
					MethodTakingLatestParamsReturningTestStruct: {
						AnchorIDL: fullStructIDL(t),
						Encoding:  config.EncodingTypeBorsh,
						Procedures: []config.ChainReaderProcedure{
							{
								IDLAccount: "TestStructB",
								RPCOpts: &config.RPCOpts{
									Encoding:   &encodingBase64,
									Commitment: &commitment,
									DataSlice: &rpc.DataSlice{
										Offset: &offset,
										Length: &length,
									},
								},
							},
							{
								IDLAccount: "TestStructA",
							},
						},
					},
					MethodReturningUint64: {
						AnchorIDL: fmt.Sprintf(baseIDL, uint64BaseTypeIDL, ""),
						Encoding:  config.EncodingTypeBorsh,
						Procedures: []config.ChainReaderProcedure{
							{
								IDLAccount: "SimpleUint64Value",
								OutputModifications: codeccommon.ModifiersConfig{
									&codeccommon.PropertyExtractorConfig{FieldName: "I"},
								},
							},
						},
					},
					DifferentMethodReturningUint64: {
						AnchorIDL: fmt.Sprintf(baseIDL, uint64BaseTypeIDL, ""),
						Encoding:  config.EncodingTypeBorsh,
						Procedures: []config.ChainReaderProcedure{
							{
								IDLAccount: "SimpleUint64Value",
								OutputModifications: codeccommon.ModifiersConfig{
									&codeccommon.PropertyExtractorConfig{FieldName: "I"},
								},
							},
						},
					},
					MethodReturningUint64Slice: {
						AnchorIDL: fmt.Sprintf(baseIDL, uint64SliceBaseTypeIDL, ""),
						Encoding:  config.EncodingTypeBincode,
						Procedures: []config.ChainReaderProcedure{
							{
								IDLAccount: "Uint64Slice",
								OutputModifications: codeccommon.ModifiersConfig{
									&codeccommon.PropertyExtractorConfig{FieldName: "Vals"},
								},
							},
						},
					},
					MethodReturningSeenStruct: {
						AnchorIDL: fullStructIDL(t),
						Encoding:  config.EncodingTypeBorsh,
						Procedures: []config.ChainReaderProcedure{
							{
								IDLAccount: "TestStructB",
							},
							{
								IDLAccount: "TestStructA",
								OutputModifications: codeccommon.ModifiersConfig{
									&codeccommon.HardCodeModifierConfig{OffChainValues: map[string]any{"ExtraField": AnyExtraValue}},
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
						Encoding:  config.EncodingTypeBorsh,
						Procedures: []config.ChainReaderProcedure{
							{
								IDLAccount: "SimpleUint64Value",
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

func (r *chainReaderInterfaceTester) GetChainReader(t *testing.T) types.ContractReader {
	client := new(mockedRPCClient)
	svc, err := chainreader.NewChainReaderService(logger.Test(t), client, r.conf)
	if err != nil {
		t.Logf("chain reader service was not able to start: %s", err.Error())
		t.FailNow()
	}

	require.NoError(t, svc.Start(context.Background()))
	t.Cleanup(func() {
		require.NoError(t, svc.Close())
	})

	if r.reader == nil {
		r.reader = &wrappedTestChainReader{tester: r}
	}

	r.reader.test = t
	r.reader.service = svc
	r.reader.client = client

	return r.reader
}

type wrappedTestChainReader struct {
	test            *testing.T
	service         *chainreader.SolanaChainReaderService
	client          *mockedRPCClient
	tester          ChainReaderInterfaceTester[*testing.T]
	testStructQueue []*TestStruct
}

func (r *wrappedTestChainReader) Start(ctx context.Context) error {
	return nil
}

func (r *wrappedTestChainReader) Close() error {
	return nil
}

func (r *wrappedTestChainReader) Ready() error {
	return nil
}

func (r *wrappedTestChainReader) HealthReport() map[string]error {
	return nil
}

func (r *wrappedTestChainReader) Name() string {
	return "wrappedTestChainReader"
}

func (r *wrappedTestChainReader) GetLatestValue(ctx context.Context, contractName string, method string, params, returnVal any) error {
	var (
		a ag_solana.PublicKey
		b ag_solana.PublicKey
	)
	switch contractName + method {
	case AnyContractName + EventName:
		r.test.Skip("Events are not yet supported in Solana")
	case AnyContractName + MethodReturningUint64:
		cdc := makeTestCodec(r.test, fmt.Sprintf(baseIDL, uint64BaseTypeIDL, ""), config.EncodingTypeBorsh)
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

		r.client.SetNext(bts, nil, 0)
	case AnyContractName + MethodReturningUint64Slice:
		cdc := makeTestCodec(r.test, fmt.Sprintf(baseIDL, uint64SliceBaseTypeIDL, ""), config.EncodingTypeBincode)
		onChainStruct := struct {
			Vals []uint64
		}{
			Vals: AnySliceToReadWithoutAnArgument,
		}

		bts, err := cdc.Encode(ctx, onChainStruct, "Uint64Slice")
		if err != nil {
			r.test.FailNow()
		}

		r.client.SetNext(bts, nil, 0)
	case AnySecondContractName + MethodReturningUint64, AnyContractName + DifferentMethodReturningUint64:
		cdc := makeTestCodec(r.test, fmt.Sprintf(baseIDL, uint64BaseTypeIDL, ""), config.EncodingTypeBorsh)
		onChainStruct := struct {
			I uint64
		}{
			I: AnyDifferentValueToReadWithoutAnArgument,
		}

		bts, err := cdc.Encode(ctx, onChainStruct, "SimpleUint64Value")
		if err != nil {
			r.test.FailNow()
		}

		r.client.SetNext(bts, nil, 0)
	case AnyContractName + MethodReturningSeenStruct:
		nextStruct := CreateTestStruct[*testing.T](0, r.tester)
		r.testStructQueue = append(r.testStructQueue, &nextStruct)

		a, b = getAddresses(r.test, r.tester, 5, 6)

		fallthrough
	default:
		if r.testStructQueue == nil || len(r.testStructQueue) == 0 {
			r.test.FailNow()
		}

		if contractName+method != AnyContractName+MethodReturningSeenStruct {
			a, b = getAddresses(r.test, r.tester, 0, 1)
		}

		nextTestStruct := r.testStructQueue[0]
		r.testStructQueue = r.testStructQueue[1:len(r.testStructQueue)]

		// split into two encoded parts to test the preloading function
		cdc := makeTestCodec(r.test, fullStructIDL(r.test), config.EncodingTypeBorsh)

		bts, err := cdc.Encode(ctx, nextTestStruct, "TestStructB")
		if err != nil {
			r.test.FailNow()
		}

		// make part A return slower than part B
		r.client.SetForAddress(a, bts, nil, 300*time.Millisecond)

		bts, err = cdc.Encode(ctx, nextTestStruct, "TestStructA")
		if err != nil {
			r.test.FailNow()
		}

		r.client.SetForAddress(b, bts, nil, 50*time.Millisecond)
	}

	return r.service.GetLatestValue(ctx, contractName, method, params, returnVal)
}

// BatchGetLatestValue implements the types.ContractReader interface.
func (r *wrappedTestChainReader) BatchGetLatestValue(_ context.Context, _ types.BatchGetLatestValueRequest) (types.BatchGetLatestValueResult, error) {
	r.test.Skip("BatchGetLatestValue is not yet supported in Solana")
	return nil, nil
}

// QueryKey implements the types.ContractReader interface.
func (r *wrappedTestChainReader) QueryKey(ctx context.Context, contractName string, filter query.KeyFilter, limitAndSort query.LimitAndSort, sequenceDataType any) ([]types.Sequence, error) {
	r.test.Skip("QueryKey is not yet supported in Solana")
	return nil, nil
}

func getAddresses(t *testing.T, tester ChainReaderInterfaceTester[*testing.T], a, b int) (ag_solana.PublicKey, ag_solana.PublicKey) {
	t.Helper()

	bindings := tester.GetBindings(t)
	fn := ag_solana.MustPublicKeyFromBase58

	return fn(bindings[a].Address), fn(bindings[b].Address)
}

func (r *wrappedTestChainReader) Bind(ctx context.Context, bindings []types.BoundContract) error {
	return r.service.Bind(ctx, bindings)
}

func (r *wrappedTestChainReader) CreateContractType(contractName, itemType string, forEncoding bool) (any, error) {
	if AnyContractName+EventName == contractName+itemType {
		r.test.Skip("Events are not yet supported in Solana")
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

func (r *chainReaderInterfaceTester) SetBatchLatestValues(t *testing.T, _ BatchCallEntry) {
	t.Skip("GetBatchLatestValues is not yet supported in Solana")
}

func (r *chainReaderInterfaceTester) TriggerEvent(t *testing.T, testStruct *TestStruct) {
	t.Skip("Events are not yet supported in Solana")
}

func (r *chainReaderInterfaceTester) GetBindings(t *testing.T) []types.BoundContract {
	return []types.BoundContract{
		{Name: strings.Join([]string{AnyContractName, MethodTakingLatestParamsReturningTestStruct, "0"}, "."), Address: r.address[0], Pending: true},
		{Name: strings.Join([]string{AnyContractName, MethodTakingLatestParamsReturningTestStruct, "1"}, "."), Address: r.address[1], Pending: true},
		{Name: strings.Join([]string{AnyContractName, MethodReturningUint64, "0"}, "."), Address: r.address[2], Pending: true},
		{Name: strings.Join([]string{AnyContractName, DifferentMethodReturningUint64, "0"}, "."), Address: r.address[3], Pending: true},
		{Name: strings.Join([]string{AnyContractName, MethodReturningUint64Slice, "0"}, "."), Address: r.address[4], Pending: true},
		{Name: strings.Join([]string{AnyContractName, MethodReturningSeenStruct, "0"}, "."), Address: r.address[5], Pending: true},
		{Name: strings.Join([]string{AnyContractName, MethodReturningSeenStruct, "1"}, "."), Address: r.address[6], Pending: true},
		{Name: strings.Join([]string{AnySecondContractName, MethodReturningUint64, "0"}, "."), Address: r.address[7], Pending: true},
	}
}

func (r *chainReaderInterfaceTester) MaxWaitTimeForEvents() time.Duration {
	// From trial and error, when running on CI, sometimes the boxes get slow
	maxWaitTime := time.Second
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

func makeTestCodec(t *testing.T, rawIDL string, encoding config.EncodingType) types.RemoteCodec {
	t.Helper()

	var idl codec.IDL
	if err := json.Unmarshal([]byte(rawIDL), &idl); err != nil {
		t.Logf("failed to unmarshal test IDL: %s", err.Error())
		t.FailNow()
	}

	testCodec, err := codec.NewIDLAccountCodec(idl, config.BuilderForEncoding(encoding))
	if err != nil {
		t.Logf("failed to create new codec from test IDL: %s", err.Error())
		t.FailNow()
	}

	return testCodec
}

func fullStructIDL(t *testing.T) string {
	t.Helper()

	return fmt.Sprintf(
		baseIDL,
		strings.Join([]string{testStructAIDL, testStructBIDL}, ","),
		strings.Join([]string{midLevelStructIDL, innerStructIDL}, ","),
	)
}

const (
	baseIDL = `{
		"version": "0.1.0",
		"name": "some_test_idl",
		"accounts": [%s],
		"types": [%s]
	}`

	testStructAIDL = `{
		"name": "TestStructA",
		"type": {
			"kind": "struct",
			"fields": [
				{"name": "field","type": {"option": "i32"}},
				{"name": "differentField","type": "string"},
				{"name": "bigField","type": "i128"},
				{"name": "nestedStruct","type": {"defined": "MidLevelStruct"}}
			]
		}
	}`

	testStructBIDL = `{
		"name": "TestStructB",
		"type": {
			"kind": "struct",
			"fields": [
				{"name": "oracleID","type": "u8"},
				{"name": "oracleIDs","type": {"array": ["u8",32]}},
				{"name": "account","type": "bytes"},
				{"name": "accounts","type": {"vec": "bytes"}}
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

// Required to allow test skipping to be on the same goroutine
type skipEventsChainReaderTester struct {
	ChainReaderInterfaceTester[*testing.T]
}

func (s *skipEventsChainReaderTester) GetChainReader(t *testing.T) types.ContractReader {
	return &skipEventsChainReader{
		ContractReader: s.ChainReaderInterfaceTester.GetChainReader(t),
		t:              t,
	}
}

type skipEventsChainReader struct {
	types.ContractReader
	t *testing.T
}

func (s *skipEventsChainReader) GetLatestValue(ctx context.Context, contractName string, method string, params, returnVal any) error {
	if contractName == AnyContractName && method == EventName {
		s.t.Skip("Events are not yet supported in Solana")
	}

	return s.ContractReader.GetLatestValue(ctx, contractName, method, params, returnVal)
}

func (s *skipEventsChainReader) BatchGetLatestValue(_ context.Context, _ types.BatchGetLatestValueRequest) (types.BatchGetLatestValueResult, error) {
	s.t.Skip("BatchGetLatestValue is not yet supported in Solana")
	return nil, nil
}

func (s *skipEventsChainReader) QueryKey(ctx context.Context, contractName string, filter query.KeyFilter, limitAndSort query.LimitAndSort, sequenceDataType any) ([]types.Sequence, error) {
	s.t.Skip("QueryKey is not yet supported in Solana")
	return nil, nil
}
