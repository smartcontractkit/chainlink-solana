package solana

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	solanago "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
	commonsol "github.com/smartcontractkit/chainlink-common/pkg/types/chains/solana"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	clientmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	configmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/config/mocks"
	txmutils "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/utils"
)

// --- test helpers & stubs ---

func pk(i byte) solanago.PublicKey {
	var p solanago.PublicKey
	for j := range p {
		p[j] = i
	}
	return p
}

func cpk(i byte) commonsol.PublicKey { return commonsol.PublicKey(pk(i)) }

func sig(i byte) solanago.Signature {
	var s solanago.Signature
	for j := range s {
		s[j] = i
	}
	return s
}

func csig(i byte) commonsol.Signature { return commonsol.Signature(sig(i)) }

func uint64Ptr(v uint64) *uint64 { return &v }

// stubChain implements the Chain interface with only the methods needed by
// SubmitTransaction, avoiding the import cycle that mocks/chain.go causes.
type stubChain struct {
	Chain
	reader    client.Reader
	readerErr error
	txm       TxManager
	cfg       config.Config
}

func (c *stubChain) Reader() (client.Reader, error) { return c.reader, c.readerErr }
func (c *stubChain) TxManager() TxManager           { return c.txm }
func (c *stubChain) Config() config.Config          { return c.cfg }

type stubWorkflow struct {
	acceptanceTimeout time.Duration
}

func (w *stubWorkflow) IsEnabled() bool                                   { return true }
func (w *stubWorkflow) AcceptanceTimeout() time.Duration                  { return w.acceptanceTimeout }
func (w *stubWorkflow) PollPeriod() time.Duration                         { return time.Second }
func (w *stubWorkflow) ForwarderAddress() *solanago.PublicKey             { return nil }
func (w *stubWorkflow) FromAddress() *solanago.PublicKey                  { return nil }
func (w *stubWorkflow) ForwarderState() *solanago.PublicKey               { return nil }
func (w *stubWorkflow) GasLimitDefault() *uint64                          { return nil }
func (w *stubWorkflow) TxAcceptanceState() *commontypes.TransactionStatus { return nil }
func (w *stubWorkflow) Local() bool                                       { return false }

type stubTxManager struct {
	mock.Mock
}

func (m *stubTxManager) Start(context.Context) error    { return nil }
func (m *stubTxManager) Close() error                   { return nil }
func (m *stubTxManager) Ready() error                   { return nil }
func (m *stubTxManager) HealthReport() map[string]error { return nil }
func (m *stubTxManager) Name() string                   { return "stubTxManager" }

func (m *stubTxManager) Enqueue(ctx context.Context, accountID string, tx *solanago.Transaction, txID *string, lastValidBlockHeight uint64, txCfgs ...txmutils.SetTxConfig) error {
	args := m.MethodCalled("Enqueue", ctx, accountID, tx, txID, lastValidBlockHeight, txCfgs)
	return args.Error(0)
}

func (m *stubTxManager) GetTransactionStatus(ctx context.Context, transactionID string) (commontypes.TransactionStatus, error) {
	args := m.MethodCalled("GetTransactionStatus", ctx, transactionID)
	return args.Get(0).(commontypes.TransactionStatus), args.Error(1)
}

func (m *stubTxManager) GetTransactionSig(transactionID string) (solanago.Signature, error) {
	args := m.MethodCalled("GetTransactionSig", transactionID)
	return args.Get(0).(solanago.Signature), args.Error(1)
}

func validBase64Tx(t *testing.T) string {
	t.Helper()
	tx, err := solanago.NewTransaction(
		[]solanago.Instruction{
			solanago.NewInstruction(
				pk(0),
				solanago.AccountMetaSlice{
					solanago.NewAccountMeta(pk(1), true, true),
				},
				[]byte{0x01, 0x02},
			),
		},
		solanago.Hash(pk(9)),
		solanago.TransactionPayer(pk(1)),
	)
	require.NoError(t, err)

	raw, err := tx.MarshalBinary()
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(raw)
}

// --- convertAccountResult tests ---

func TestConvertAccountResult(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		got, err := convertAccountResult(nil, commonsol.EncodingBase64)
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("non-nil result with nil Value", func(t *testing.T) {
		acc := &rpc.GetAccountInfoResult{
			RPCContext: rpc.RPCContext{Context: rpc.Context{Slot: 42}},
			Value:      nil,
		}
		got, err := convertAccountResult(acc, commonsol.EncodingBase64)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, uint64(42), got.RPCContext.Slot)
		assert.Nil(t, got.Value)
	})

	t.Run("non-nil result with populated Value", func(t *testing.T) {
		data := rpc.DataBytesOrJSONFromBytes([]byte{0xca, 0xfe})
		acc := &rpc.GetAccountInfoResult{
			RPCContext: rpc.RPCContext{Context: rpc.Context{Slot: 100}},
			Value: &rpc.Account{
				Lamports:   999,
				Owner:      pk(5),
				Data:       data,
				Executable: true,
				Space:      64,
			},
		}
		got, err := convertAccountResult(acc, commonsol.EncodingBase64)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, uint64(100), got.RPCContext.Slot)
		require.NotNil(t, got.Value)
		assert.Equal(t, uint64(999), got.Value.Lamports)
		assert.Equal(t, cpk(5), got.Value.Owner)
		assert.True(t, got.Value.Executable)
		require.NotNil(t, got.Value.Data)
		assert.Equal(t, commonsol.EncodingBase64, got.Value.Data.RawDataEncoding)
		assert.Equal(t, []byte{0xca, 0xfe}, got.Value.Data.AsDecodedBinary)
	})

	t.Run("non-nil result with nil Data inside Value", func(t *testing.T) {
		acc := &rpc.GetAccountInfoResult{
			RPCContext: rpc.RPCContext{Context: rpc.Context{Slot: 7}},
			Value: &rpc.Account{
				Lamports: 1,
				Owner:    pk(2),
				Data:     nil,
			},
		}
		got, err := convertAccountResult(acc, commonsol.EncodingBase64)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.NotNil(t, got.Value)
		assert.Nil(t, got.Value.Data)
	})
}

// --- convertDataBytesOrJSON tests ---

func TestConvertDataBytesOrJSON(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		got, err := convertDataBytesOrJSON(nil, "")
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("base64 default when pref is empty", func(t *testing.T) {
		raw := []byte{0xde, 0xad, 0xbe, 0xef}
		obj := rpc.DataBytesOrJSONFromBytes(raw)

		got, err := convertDataBytesOrJSON(obj, "")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, commonsol.EncodingBase64, got.RawDataEncoding)
		assert.Equal(t, raw, got.AsDecodedBinary)
		assert.NotNil(t, got.AsJSON)
	})

	t.Run("base64 explicit pref with binary data", func(t *testing.T) {
		raw := []byte{0x01, 0x02, 0x03}
		obj := rpc.DataBytesOrJSONFromBytes(raw)

		got, err := convertDataBytesOrJSON(obj, commonsol.EncodingBase64)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, commonsol.EncodingBase64, got.RawDataEncoding)
		assert.Equal(t, raw, got.AsDecodedBinary)
	})

	t.Run("JSON fallback with EncodingJSON", func(t *testing.T) {
		raw := []byte{0xaa, 0xbb}
		obj := rpc.DataBytesOrJSONFromBytes(raw)

		got, err := convertDataBytesOrJSON(obj, commonsol.EncodingJSON)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, commonsol.EncodingJSON, got.RawDataEncoding)
		assert.NotNil(t, got.AsJSON)
		assert.Equal(t, raw, got.AsDecodedBinary)
	})

	t.Run("JSON fallback with EncodingJSONParsed", func(t *testing.T) {
		raw := []byte{0xcc, 0xdd}
		obj := rpc.DataBytesOrJSONFromBytes(raw)

		got, err := convertDataBytesOrJSON(obj, commonsol.EncodingJSONParsed)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, commonsol.EncodingJSONParsed, got.RawDataEncoding)
		assert.NotNil(t, got.AsJSON)
		assert.Equal(t, raw, got.AsDecodedBinary)
	})

	t.Run("base64 fallback parses json array when GetBinary is empty", func(t *testing.T) {
		encoded := base64.StdEncoding.EncodeToString([]byte("hello world"))
		rawJSON := fmt.Sprintf(`[%q, "base64"]`, encoded)
		var obj rpc.DataBytesOrJSON
		err := json.Unmarshal([]byte(rawJSON), &obj)
		require.NoError(t, err)

		got, err := convertDataBytesOrJSON(&obj, commonsol.EncodingBase64)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, commonsol.EncodingBase64, got.RawDataEncoding)
		assert.Equal(t, []byte("hello world"), got.AsDecodedBinary)
	})

	t.Run("unknown encoding with binary data falls back to base64", func(t *testing.T) {
		raw := []byte{0x11, 0x22}
		obj := rpc.DataBytesOrJSONFromBytes(raw)

		got, err := convertDataBytesOrJSON(obj, "some-unknown-encoding")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, commonsol.EncodingBase64, got.RawDataEncoding)
		assert.Equal(t, raw, got.AsDecodedBinary)
	})
}

// --- SubmitTransaction tests ---

func newSubmitTestHarness(t *testing.T, acceptanceTimeout time.Duration) (*solanaService, *clientmocks.ReaderWriter, *stubTxManager) {
	t.Helper()

	mockReader := clientmocks.NewReaderWriter(t)
	mockCfg := configmocks.NewConfig(t)
	txm := &stubTxManager{}
	txm.Test(t)
	t.Cleanup(func() { txm.AssertExpectations(t) })

	wf := &stubWorkflow{acceptanceTimeout: acceptanceTimeout}
	mockCfg.EXPECT().WF().Return(wf).Maybe()

	chain := &stubChain{
		reader: mockReader,
		txm:    txm,
		cfg:    mockCfg,
	}

	ss := &solanaService{
		chain:  chain,
		logger: logger.Nop(),
	}
	return ss, mockReader, txm
}

func TestSubmitTransaction_ZeroAcceptanceTimeout(t *testing.T) {
	ss, mockReader, txm := newSubmitTestHarness(t, 0)

	mockReader.EXPECT().LatestBlockhash(mock.Anything).Return(&rpc.GetLatestBlockhashResult{
		Value: &rpc.LatestBlockhashResult{
			Blockhash:            solanago.Hash(pk(8)),
			LastValidBlockHeight: 500,
		},
	}, nil)

	txm.On("Enqueue", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil)

	// retry.Do executes the function at least once even when the context is
	// already expired. Return Pending so the retry loop tries to continue but
	// finds the context cancelled.
	txm.On("GetTransactionStatus", mock.Anything, mock.Anything).
		Return(commontypes.Pending, nil).Maybe()

	req := commonsol.SubmitTransactionRequest{
		EncodedTransaction: validBase64Tx(t),
		Receiver:           cpk(1),
	}

	_, err := ss.SubmitTransaction(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context")
}

func TestSubmitTransaction_NonNilComputeMaxPrice(t *testing.T) {
	ss, mockReader, txm := newSubmitTestHarness(t, 5*time.Second)

	mockReader.EXPECT().LatestBlockhash(mock.Anything).Return(&rpc.GetLatestBlockhashResult{
		Value: &rpc.LatestBlockhashResult{
			Blockhash:            solanago.Hash(pk(8)),
			LastValidBlockHeight: 500,
		},
	}, nil)

	var capturedCfgs []txmutils.SetTxConfig
	txm.On("Enqueue", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			capturedCfgs = args.Get(5).([]txmutils.SetTxConfig)
		}).
		Return(nil)

	txm.On("GetTransactionStatus", mock.Anything, mock.Anything).
		Return(commontypes.Unconfirmed, nil)

	maxPrice := uint64(100_000)
	computeLimit := uint32(200_000)
	req := commonsol.SubmitTransactionRequest{
		EncodedTransaction: validBase64Tx(t),
		Receiver:           cpk(1),
		Cfg: &commonsol.ComputeConfig{
			ComputeLimit:    &computeLimit,
			ComputeMaxPrice: &maxPrice,
		},
	}

	reply, err := ss.SubmitTransaction(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, reply)
	assert.Equal(t, commonsol.TxSuccess, reply.Status)

	// Cfg is non-nil so we expect: SetEstimateComputeUnitLimit + SetComputeUnitLimit + SetComputeUnitPriceMax
	require.Len(t, capturedCfgs, 3)

	// Apply captured configs to a zero TxConfig and verify the values.
	var applied txmutils.TxConfig
	for _, fn := range capturedCfgs {
		fn(&applied)
	}
	assert.False(t, applied.EstimateComputeUnitLimit)
	assert.Equal(t, computeLimit, applied.ComputeUnitLimit)
	assert.Equal(t, maxPrice, applied.ComputeUnitPriceMax)
}

func TestSubmitTransaction_ContextExpiresBeforeConfirmation(t *testing.T) {
	ss, mockReader, txm := newSubmitTestHarness(t, 10*time.Second)

	mockReader.EXPECT().LatestBlockhash(mock.Anything).Return(&rpc.GetLatestBlockhashResult{
		Value: &rpc.LatestBlockhashResult{
			Blockhash:            solanago.Hash(pk(8)),
			LastValidBlockHeight: 500,
		},
	}, nil)

	txm.On("Enqueue", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil)

	txm.On("GetTransactionStatus", mock.Anything, mock.Anything).
		Return(commontypes.Pending, nil)

	req := commonsol.SubmitTransactionRequest{
		EncodedTransaction: validBase64Tx(t),
		Receiver:           cpk(1),
	}

	// Cancel immediately so both the parent context and the internal
	// retry context are expired before confirmation can succeed.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ss.SubmitTransaction(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context")
	assert.Contains(t, err.Error(), "tx with ID")
}

func TestSubmitTransaction_FatalStatus(t *testing.T) {
	ss, mockReader, txm := newSubmitTestHarness(t, 5*time.Second)

	mockReader.EXPECT().LatestBlockhash(mock.Anything).Return(&rpc.GetLatestBlockhashResult{
		Value: &rpc.LatestBlockhashResult{
			Blockhash:            solanago.Hash(pk(8)),
			LastValidBlockHeight: 500,
		},
	}, nil)

	txm.On("Enqueue", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil)

	txm.On("GetTransactionStatus", mock.Anything, mock.Anything).
		Return(commontypes.Fatal, nil)

	req := commonsol.SubmitTransactionRequest{
		EncodedTransaction: validBase64Tx(t),
		Receiver:           cpk(1),
	}

	reply, err := ss.SubmitTransaction(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, reply)
	assert.Equal(t, commonsol.TxFatal, reply.Status)
	assert.NotEmpty(t, reply.IdempotencyKey)
}

func TestSubmitTransaction_NilCfg(t *testing.T) {
	ss, mockReader, txm := newSubmitTestHarness(t, 5*time.Second)

	mockReader.EXPECT().LatestBlockhash(mock.Anything).Return(&rpc.GetLatestBlockhashResult{
		Value: &rpc.LatestBlockhashResult{
			Blockhash:            solanago.Hash(pk(8)),
			LastValidBlockHeight: 500,
		},
	}, nil)

	var capturedCfgs []txmutils.SetTxConfig
	txm.On("Enqueue", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			capturedCfgs = args.Get(5).([]txmutils.SetTxConfig)
		}).
		Return(nil)

	txm.On("GetTransactionStatus", mock.Anything, mock.Anything).
		Return(commontypes.Unconfirmed, nil)

	req := commonsol.SubmitTransactionRequest{
		EncodedTransaction: validBase64Tx(t),
		Receiver:           cpk(1),
		Cfg:                nil,
	}

	reply, err := ss.SubmitTransaction(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, reply)
	assert.Equal(t, commonsol.TxSuccess, reply.Status)
	assert.Nil(t, capturedCfgs, "no tx configs should be set when Cfg is nil")
}
