package fakes

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	gbinary "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonCap "github.com/smartcontractkit/chainlink-common/pkg/capabilities"
	caperrors "github.com/smartcontractkit/chainlink-common/pkg/capabilities/errors"
	solcap "github.com/smartcontractkit/chainlink-common/pkg/capabilities/v2/chain-capabilities/solana"
	solanaserver "github.com/smartcontractkit/chainlink-common/pkg/capabilities/v2/chain-capabilities/solana/server"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	"github.com/smartcontractkit/chainlink-common/pkg/types/core"
	sdk "github.com/smartcontractkit/chainlink-protos/cre/go/sdk"

	commoncodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/common"
)

const testChainSelector uint64 = 16423721717087811551 // SOLANA_DEVNET

func testKey(t *testing.T) solana.PrivateKey {
	t.Helper()
	w := solana.NewWallet()
	return w.PrivateKey
}

func testForwarderProgramID(t *testing.T) solana.PublicKey {
	t.Helper()
	return solana.MustPublicKeyFromBase58("7kuEAA3mSC1Tz8gQjnvH7bKFda9xSPRRin9SZbH49cNK")
}

func testForwarderStateAccount(t *testing.T) solana.PublicKey {
	t.Helper()
	return solana.MustPublicKeyFromBase58("MBUQyaWiZ6TmEr3k7p9nuVnHZWv6KTL1j3tQCUGrJ4r")
}

const testEventIDLJSON = `{
	"address": "11111111111111111111111111111111",
	"metadata": {"name": "log_read_test", "version": "0.1.0", "spec": "0.1.0"},
	"instructions": [],
	"events": [
		{"name": "TestEvent", "discriminator": [1, 2, 3, 4, 5, 6, 7, 8]}
	],
	"types": [
		{
			"name": "TestEvent",
			"type": {
				"kind": "struct",
				"fields": [
					{"name": "str_val", "type": "string"},
					{"name": "u64_value", "type": "u64"}
				]
			}
		}
	]
}`

func encodeTestEvent(t *testing.T, strVal string, u64Value uint64) []byte {
	t.Helper()
	buf := new(bytes.Buffer)
	buf.Write(commoncodec.NewDiscriminatorHashPrefix("TestEvent", false))
	enc := gbinary.NewBorshEncoder(buf)
	require.NoError(t, enc.Encode(strVal))
	require.NoError(t, enc.Encode(u64Value))
	return buf.Bytes()
}

func newTestSolanaChain(t *testing.T, dryRun bool) *FakeSolanaChain {
	t.Helper()
	fc, err := NewFakeSolanaChain(
		logger.Test(t),
		rpc.New(rpc.DevNet_RPC),
		testKey(t),
		testForwarderProgramID(t),
		testForwarderStateAccount(t),
		testChainSelector,
		dryRun,
	)
	require.NoError(t, err)
	require.NotNil(t, fc)
	return fc
}

func TestFakeSolanaChain_ConstructionValidation(t *testing.T) {
	t.Parallel()

	t.Run("nil client", func(t *testing.T) {
		_, err := NewFakeSolanaChain(logger.Test(t), nil, testKey(t),
			testForwarderProgramID(t), testForwarderStateAccount(t), testChainSelector, true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "rpc client")
	})

	t.Run("empty private key", func(t *testing.T) {
		_, err := NewFakeSolanaChain(logger.Test(t), rpc.New(rpc.DevNet_RPC), nil,
			testForwarderProgramID(t), testForwarderStateAccount(t), testChainSelector, true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "private key")
	})
}

func TestFakeSolanaChain_BasicAccessors(t *testing.T) {
	t.Parallel()
	fc := newTestSolanaChain(t, true)
	assert.Equal(t, testChainSelector, fc.ChainSelector())
	assert.Contains(t, strings.ToLower(fc.Name()), "solana")
	assert.NotEmpty(t, fc.Description())
}

func TestFakeSolanaChain_InterfaceAssertions(t *testing.T) {
	t.Parallel()
	fc := newTestSolanaChain(t, true)
	var _ services.Service = fc
	var _ solanaserver.ClientCapability = fc
	var _ commonCap.ExecutableCapability = fc
}

func TestFakeSolanaChain_InitialiseStartsService(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fc := newTestSolanaChain(t, true)

	require.Error(t, fc.Ready(), "service should not be Ready before Initialise")
	require.NoError(t, fc.Initialise(ctx, core.StandardCapabilitiesDependencies{}))
	require.NoError(t, fc.Ready())
	require.NoError(t, fc.Close())
}

func TestFakeSolanaChain_WriteReport_InputValidation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fc := newTestSolanaChain(t, true)
	md := commonCap.RequestMetadata{}

	validReceiver := solana.NewWallet().PublicKey().Bytes()
	validReport := mkReport()
	validCompute := &solcap.ComputeConfig{ComputeLimit: 200_000}

	cases := []struct {
		name    string
		input   *solcap.WriteReportRequest
		errLike string
	}{
		{
			name:    "nil input",
			input:   nil,
			errLike: "writeReportRequest is nil",
		},
		{
			name:    "nil report",
			input:   &solcap.WriteReportRequest{Receiver: validReceiver, ComputeConfig: validCompute},
			errLike: "report must not be nil",
		},
		{
			name:    "nil compute config",
			input:   &solcap.WriteReportRequest{Receiver: validReceiver, Report: validReport},
			errLike: "computeConfig must not be nil",
		},
		{
			name:    "zero compute limit",
			input:   &solcap.WriteReportRequest{Receiver: validReceiver, Report: validReport, ComputeConfig: &solcap.ComputeConfig{}},
			errLike: "computeLimit must be > 0",
		},
		{
			name:    "bad receiver length",
			input:   &solcap.WriteReportRequest{Receiver: []byte{0x01}, Report: validReport, ComputeConfig: validCompute},
			errLike: "receiver",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := fc.WriteReport(ctx, md, tc.input)
			require.NotNil(t, err)
			assert.Equal(t, caperrors.InvalidArgument, err.Code())
			assert.Contains(t, err.Error(), tc.errLike)
		})
	}
}

func TestFakeSolanaChain_UnimplementedMethods(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fc := newTestSolanaChain(t, true)
	md := commonCap.RequestMetadata{}

	t.Run("GetBlock", func(t *testing.T) {
		_, err := fc.GetBlock(ctx, md, nil)
		require.NotNil(t, err)
		assert.Equal(t, caperrors.Unimplemented, err.Code())
	})
	t.Run("GetFeeForMessage", func(t *testing.T) {
		_, err := fc.GetFeeForMessage(ctx, md, nil)
		require.NotNil(t, err)
		assert.Equal(t, caperrors.Unimplemented, err.Code())
	})
	t.Run("GetMultipleAccountsWithOpts", func(t *testing.T) {
		_, err := fc.GetMultipleAccountsWithOpts(ctx, md, nil)
		require.NotNil(t, err)
		assert.Equal(t, caperrors.Unimplemented, err.Code())
	})
	t.Run("GetSignatureStatuses", func(t *testing.T) {
		_, err := fc.GetSignatureStatuses(ctx, md, nil)
		require.NotNil(t, err)
		assert.Equal(t, caperrors.Unimplemented, err.Code())
	})
	t.Run("GetTransaction", func(t *testing.T) {
		_, err := fc.GetTransaction(ctx, md, nil)
		require.NotNil(t, err)
		assert.Equal(t, caperrors.Unimplemented, err.Code())
	})
}

func TestFakeSolanaChain_LogTrigger(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	md := commonCap.RequestMetadata{}
	prog := solana.NewWallet().PublicKey()
	const triggerID = "solana:ChainSelector:16423721717087811551@1.0.0"

	t.Run("registered log is delivered", func(t *testing.T) {
		t.Parallel()
		fc := newTestSolanaChain(t, true)
		ch, cerr := fc.RegisterLogTrigger(ctx, triggerID, md, &solcap.FilterLogTriggerRequest{Address: prog.Bytes()})
		require.Nil(t, cerr)

		log := &solcap.Log{Address: prog.Bytes(), Data: []byte("body"), LogIndex: 0}
		require.NoError(t, fc.ManualTrigger(ctx, triggerID, log))

		select {
		case got := <-ch:
			assert.Equal(t, prog.Bytes(), got.Trigger.GetAddress())
			assert.NotEmpty(t, got.Id)
		case <-time.After(2 * time.Second):
			t.Fatal("expected trigger event, got none")
		}
	})

	t.Run("filter rejects wrong program address", func(t *testing.T) {
		t.Parallel()
		fc := newTestSolanaChain(t, true)
		_, cerr := fc.RegisterLogTrigger(ctx, triggerID, md, &solcap.FilterLogTriggerRequest{Address: prog.Bytes()})
		require.Nil(t, cerr)

		other := solana.NewWallet().PublicKey()
		err := fc.ManualTrigger(ctx, triggerID, &solcap.Log{Address: other.Bytes()})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not match filter address")
	})

	t.Run("filter matches anchor event discriminator", func(t *testing.T) {
		t.Parallel()
		fc := newTestSolanaChain(t, true)
		_, cerr := fc.RegisterLogTrigger(ctx, triggerID, md, &solcap.FilterLogTriggerRequest{
			Address:   prog.Bytes(),
			EventName: "MessageEmitted",
		})
		require.Nil(t, cerr)

		// Wrong discriminator is rejected.
		err := fc.ManualTrigger(ctx, triggerID, &solcap.Log{Address: prog.Bytes(), EventSig: []byte{0, 0, 0, 0, 0, 0, 0, 0}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not match discriminator")

		err = fc.ManualTrigger(ctx, triggerID, &solcap.Log{Address: prog.Bytes(), EventSig: []byte{0x01}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "event signature must be 8 bytes")

		// Correct discriminator is accepted.
		ch, _ := fc.RegisterLogTrigger(ctx, triggerID, md, &solcap.FilterLogTriggerRequest{
			Address:   prog.Bytes(),
			EventName: "MessageEmitted",
		})
		require.NoError(t, fc.ManualTrigger(ctx, triggerID, &solcap.Log{
			Address:  prog.Bytes(),
			EventSig: commoncodec.NewDiscriminatorHashPrefix("MessageEmitted", false),
		}))
		select {
		case <-ch:
		case <-time.After(2 * time.Second):
			t.Fatal("expected trigger event, got none")
		}
	})

	t.Run("unregistered trigger errors", func(t *testing.T) {
		t.Parallel()
		fc := newTestSolanaChain(t, true)
		err := fc.ManualTrigger(ctx, "nope", &solcap.Log{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is not registered")
	})

	t.Run("missing filter address is rejected", func(t *testing.T) {
		t.Parallel()
		fc := newTestSolanaChain(t, true)
		_, cerr := fc.RegisterLogTrigger(ctx, triggerID, md, &solcap.FilterLogTriggerRequest{})
		require.Nil(t, cerr)
		err := fc.ManualTrigger(ctx, triggerID, &solcap.Log{Address: prog.Bytes()})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing program address")
	})

	t.Run("malformed program addresses are rejected", func(t *testing.T) {
		t.Parallel()
		fc := newTestSolanaChain(t, true)
		_, cerr := fc.RegisterLogTrigger(ctx, triggerID, md, &solcap.FilterLogTriggerRequest{Address: []byte{0x01}})
		require.Nil(t, cerr)
		err := fc.ManualTrigger(ctx, triggerID, &solcap.Log{Address: prog.Bytes()})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "filter program address must be 32 bytes")

		_, cerr = fc.RegisterLogTrigger(ctx, triggerID, md, &solcap.FilterLogTriggerRequest{Address: prog.Bytes()})
		require.Nil(t, cerr)
		err = fc.ManualTrigger(ctx, triggerID, &solcap.Log{Address: []byte{0x01}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "log program address must be 32 bytes")
	})

	t.Run("subkey filter matches decoded event field", func(t *testing.T) {
		t.Parallel()
		fc := newTestSolanaChain(t, true)
		filter := &solcap.FilterLogTriggerRequest{
			Address:         prog.Bytes(),
			EventName:       "TestEvent",
			ContractIdlJson: []byte(testEventIDLJSON),
			Subkeys: []*solcap.SubkeyConfig{
				{
					Path: []string{"U64Value"},
					Comparers: []*solcap.ValueComparator{
						{Operator: solcap.ComparisonOperator_COMPARISON_OPERATOR_EQ, Value: encodeUint(111)},
					},
				},
			},
		}
		_, cerr := fc.RegisterLogTrigger(ctx, triggerID, md, filter)
		require.Nil(t, cerr)

		log := &solcap.Log{Address: prog.Bytes(), EventSig: commoncodec.NewDiscriminatorHashPrefix("TestEvent", false), Data: encodeTestEvent(t, "Hello, World!", 111)}
		require.NoError(t, fc.ManualTrigger(ctx, triggerID, log))
	})

	t.Run("subkey filter rejects non-matching decoded event field", func(t *testing.T) {
		t.Parallel()
		fc := newTestSolanaChain(t, true)
		filter := &solcap.FilterLogTriggerRequest{
			Address:         prog.Bytes(),
			EventName:       "TestEvent",
			ContractIdlJson: []byte(testEventIDLJSON),
			Subkeys: []*solcap.SubkeyConfig{
				{
					Path: []string{"U64Value"},
					Comparers: []*solcap.ValueComparator{
						{Operator: solcap.ComparisonOperator_COMPARISON_OPERATOR_EQ, Value: encodeUint(111)},
					},
				},
			},
		}
		_, cerr := fc.RegisterLogTrigger(ctx, triggerID, md, filter)
		require.Nil(t, cerr)

		log := &solcap.Log{Address: prog.Bytes(), EventSig: commoncodec.NewDiscriminatorHashPrefix("TestEvent", false), Data: encodeTestEvent(t, "Hello, World!", 222)}
		err := fc.ManualTrigger(ctx, triggerID, log)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "subkey filter mismatch")
	})

	t.Run("subkey filter with nested path is rejected", func(t *testing.T) {
		t.Parallel()
		fc := newTestSolanaChain(t, true)
		_, cerr := fc.RegisterLogTrigger(ctx, triggerID, md, &solcap.FilterLogTriggerRequest{
			Address:         prog.Bytes(),
			EventName:       "TestEvent",
			ContractIdlJson: []byte(testEventIDLJSON),
			Subkeys: []*solcap.SubkeyConfig{
				{Path: []string{"nested", "field"}},
			},
		})
		require.Nil(t, cerr)
		log := &solcap.Log{Address: prog.Bytes(), EventSig: commoncodec.NewDiscriminatorHashPrefix("TestEvent", false), Data: encodeTestEvent(t, "Hello, World!", 111)}
		err := fc.ManualTrigger(ctx, triggerID, log)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only single-level")
	})

	t.Run("cpi filters validate config and deliver extracted logs", func(t *testing.T) {
		t.Parallel()
		fc := newTestSolanaChain(t, true)
		_, cerr := fc.RegisterLogTrigger(ctx, triggerID, md, &solcap.FilterLogTriggerRequest{
			Address: prog.Bytes(),
			CpiFilterConfig: &solcap.CPIFilterConfig{
				DestAddress: []byte{0x01},
				MethodName:  []byte("anchor:event"),
			},
		})
		require.Nil(t, cerr)
		err := fc.ManualTrigger(ctx, triggerID, &solcap.Log{Address: prog.Bytes()})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "CPI filter destination address must be 32 bytes")

		_, cerr = fc.RegisterLogTrigger(ctx, triggerID, md, &solcap.FilterLogTriggerRequest{
			Address: prog.Bytes(),
			CpiFilterConfig: &solcap.CPIFilterConfig{
				DestAddress: prog.Bytes(),
			},
		})
		require.Nil(t, cerr)
		err = fc.ManualTrigger(ctx, triggerID, &solcap.Log{Address: prog.Bytes()})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "CPI filter method name cannot be empty")

		ch, cerr := fc.RegisterLogTrigger(ctx, triggerID, md, &solcap.FilterLogTriggerRequest{
			Address: prog.Bytes(),
			CpiFilterConfig: &solcap.CPIFilterConfig{
				DestAddress: prog.Bytes(),
				MethodName:  []byte("anchor:event"),
			},
		})
		require.Nil(t, cerr)
		err = fc.ManualTrigger(ctx, triggerID, &solcap.Log{Address: prog.Bytes()})
		require.NoError(t, err)
		select {
		case <-ch:
		case <-time.After(2 * time.Second):
			t.Fatal("expected trigger event, got none")
		}
	})

	t.Run("nil log is rejected", func(t *testing.T) {
		t.Parallel()
		fc := newTestSolanaChain(t, true)
		_, cerr := fc.RegisterLogTrigger(ctx, triggerID, md, &solcap.FilterLogTriggerRequest{Address: prog.Bytes()})
		require.Nil(t, cerr)
		err := fc.ManualTrigger(ctx, triggerID, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "payload is nil")
	})

	t.Run("unregister removes the trigger", func(t *testing.T) {
		t.Parallel()
		fc := newTestSolanaChain(t, true)
		_, cerr := fc.RegisterLogTrigger(ctx, triggerID, md, &solcap.FilterLogTriggerRequest{Address: prog.Bytes()})
		require.Nil(t, cerr)
		require.Nil(t, fc.UnregisterLogTrigger(ctx, triggerID, md, nil))
		err := fc.ManualTrigger(ctx, triggerID, &solcap.Log{Address: prog.Bytes()})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is not registered")
	})
}

// ---------- pure helpers ----------

func TestManualSolanaTriggerEventsHaveStableUniqueIDs(t *testing.T) {
	t.Parallel()

	fc := newTestSolanaChain(t, true)
	first := fc.createManualTriggerEvent(&solcap.Log{
		BlockHash: []byte{0x01},
		TxHash:    []byte{0x02},
		LogIndex:  3,
	})
	second := fc.createManualTriggerEvent(&solcap.Log{
		BlockHash: []byte{0x01},
		TxHash:    []byte{0x04},
		LogIndex:  3,
	})

	require.NotEmpty(t, first.Id)
	require.NotEmpty(t, second.Id)
	require.NotEqual(t, first.Id, second.Id)
	assert.Equal(t, first.Id, fc.createManualTriggerEvent(&solcap.Log{
		BlockHash: []byte{0x01},
		TxHash:    []byte{0x02},
		LogIndex:  3,
	}).Id)
}

func TestPubkeyFromBytes(t *testing.T) {
	t.Parallel()
	t.Run("ok 32 bytes", func(t *testing.T) {
		want := solana.NewWallet().PublicKey()
		got, err := pubkeyFromBytes(want.Bytes())
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})
	t.Run("wrong length", func(t *testing.T) {
		_, err := pubkeyFromBytes([]byte{0x01, 0x02})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "32 bytes")
	})
}

func TestBuildReportPayload(t *testing.T) {
	t.Parallel()

	t.Run("happy path with 2 sigs", func(t *testing.T) {
		r := mkReportWith(2)
		out, err := buildReportPayload(r)
		require.NoError(t, err)
		// len_sigs(1) + 2*65 + raw_report(MetadataLength+) + context(96)
		assert.Equal(t, byte(2), out[0])
		assert.Equal(t, 1+2*signatureLen+len(r.RawReport)+reportContextLen, len(out))
	})
	t.Run("too many sigs", func(t *testing.T) {
		r := mkReportWith(maxOracles + 1)
		_, err := buildReportPayload(r)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds max")
	})
	t.Run("wrong sig length", func(t *testing.T) {
		r := mkReportWith(1)
		r.Sigs[0].Signature = []byte{0x01}
		_, err := buildReportPayload(r)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid length")
	})
	t.Run("wrong context length", func(t *testing.T) {
		r := mkReportWith(0)
		r.ReportContext = []byte{0x01}
		_, err := buildReportPayload(r)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "context length")
	})
}

func TestDeriveForwarderAuthority_Stable(t *testing.T) {
	t.Parallel()
	prog := testForwarderProgramID(t)
	state := testForwarderStateAccount(t)
	receiver := solana.NewWallet().PublicKey()

	a1, _, err := deriveForwarderAuthority(prog, state, receiver)
	require.NoError(t, err)
	a2, _, err := deriveForwarderAuthority(prog, state, receiver)
	require.NoError(t, err)
	assert.Equal(t, a1, a2)
}

func TestReceiverStatusFromLogs(t *testing.T) {
	t.Parallel()
	fwd := testForwarderProgramID(t)

	t.Run("forwarder-level failure returns nil", func(t *testing.T) {
		logs := []string{
			"Program " + fwd.String() + " invoke [1]",
			"Program " + fwd.String() + " failed: Custom program error",
		}
		assert.Nil(t, receiverStatusFromLogs(logs, fwd))
	})
	t.Run("non-forwarder failure returns REVERTED", func(t *testing.T) {
		logs := []string{
			"Program X invoke [1]",
			"Program X failed: receiver said no",
		}
		got := receiverStatusFromLogs(logs, fwd)
		require.NotNil(t, got)
		assert.Equal(t, solcap.ReceiverContractExecutionStatus_RECEIVER_CONTRACT_EXECUTION_STATUS_REVERTED, *got)
	})
}

// ---------- test fixtures ----------

// mkReport builds a minimal-but-validly-shaped *sdk.ReportResponse:
// no sigs, METADATA_LENGTH+ raw report bytes, 96-byte context.
func mkReport() *sdk.ReportResponse {
	return mkReportWith(0)
}

func mkReportWith(numSigs int) *sdk.ReportResponse {
	raw := make([]byte, 110) // > METADATA_LENGTH(109)
	sigs := make([]*sdk.AttributedSignature, numSigs)
	for i := range sigs {
		sigs[i] = &sdk.AttributedSignature{Signature: make([]byte, signatureLen)}
	}
	return &sdk.ReportResponse{
		RawReport:     raw,
		ReportContext: make([]byte, reportContextLen),
		Sigs:          sigs,
	}
}
