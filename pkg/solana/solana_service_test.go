package solana

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
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
	"github.com/smartcontractkit/chainlink-common/pkg/types/query"

	logpollertypes "github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	clientmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	configmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/config/mocks"
	txmutils "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/utils"
)

func Test_Converters(t *testing.T) {
	t.Run("convertMessageHeader", func(t *testing.T) {
		h := solanago.MessageHeader{
			NumRequiredSignatures:       3,
			NumReadonlySignedAccounts:   1,
			NumReadonlyUnsignedAccounts: 2,
		}
		got := convertMessageHeader(h)
		require.Equal(t, commonsol.MessageHeader{
			NumRequiredSignatures:       3,
			NumReadonlySignedAccounts:   1,
			NumReadonlyUnsignedAccounts: 2,
		}, got)
	})

	t.Run("convertCompiledInstruction", func(t *testing.T) {
		ix := solanago.CompiledInstruction{
			ProgramIDIndex: 5,
			Accounts:       []uint16{1, 2, 9},
			Data:           []byte{0xde, 0xad, 0xbe, 0xef},
		}
		got := convertCompiledInstruction(ix)
		require.Equal(t, uint16(5), got.ProgramIDIndex)
		require.Equal(t, []uint16{1, 2, 9}, got.Accounts)
		require.Equal(t, []byte{0xde, 0xad, 0xbe, 0xef}, got.Data)
		require.Equal(t, uint16(0), got.StackHeight)
	})

	t.Run("convertAddressTableLookupSlice", func(t *testing.T) {
		in := []solanago.MessageAddressTableLookup{
			{
				AccountKey:      pk(7),
				WritableIndexes: []uint8{1, 3},
				ReadonlyIndexes: []uint8{2, 4},
			},
		}
		got := convertAddressTableLookupSlice(in)
		require.Len(t, got, 1)
		require.Equal(t, cpk(7), got[0].AccountKey)
		require.Equal(t, []uint8{1, 3}, got[0].WritableIndexes)
		require.Equal(t, []uint8{2, 4}, got[0].ReadonlyIndexes)
		require.Nil(t, convertAddressTableLookupSlice(nil))
	})

	t.Run("convertMessage", func(t *testing.T) {
		m := solanago.Message{
			AccountKeys: solanago.PublicKeySlice{pk(1), pk(2)},
			Header: solanago.MessageHeader{
				NumRequiredSignatures:       2,
				NumReadonlySignedAccounts:   1,
				NumReadonlyUnsignedAccounts: 0,
			},
			RecentBlockhash: solanago.Hash(pk(9)),
			Instructions: []solanago.CompiledInstruction{
				{ProgramIDIndex: 0, Accounts: []uint16{0, 1}, Data: []byte("hi")},
			},
			AddressTableLookups: []solanago.MessageAddressTableLookup{
				{AccountKey: pk(3), WritableIndexes: []uint8{5}, ReadonlyIndexes: []uint8{7}},
			},
		}

		got := convertMessage(m)
		require.Equal(t, commonsol.PublicKeySlice{cpk(1), cpk(2)}, got.AccountKeys)
		require.Equal(t, commonsol.MessageHeader{NumReadonlySignedAccounts: 1,
			NumRequiredSignatures:       2,
			NumReadonlyUnsignedAccounts: 0}, got.Header)
		require.Equal(t, commonsol.Hash(cpk(9)), got.RecentBlockhash)
		require.Len(t, got.Instructions, 1)
		require.Equal(t, uint16(0), got.Instructions[0].ProgramIDIndex)
		require.Equal(t, []uint16{0, 1}, got.Instructions[0].Accounts)
		require.Equal(t, []byte("hi"), got.Instructions[0].Data)
		require.Equal(t, commonsol.MessageAddressTableLookupSlice{
			{AccountKey: cpk(3), WritableIndexes: []uint8{5}, ReadonlyIndexes: []uint8{7}},
		}, got.AddressTableLookups)
	})

	t.Run("convertTokenBalance", func(t *testing.T) {
		owner := pk(42)
		pid := pk(24)
		tb := rpc.TokenBalance{
			AccountIndex:  15,
			Owner:         &owner,
			ProgramId:     &pid,
			Mint:          pk(99),
			UiTokenAmount: &rpc.UiTokenAmount{Amount: "12345", Decimals: 6, UiAmountString: "12.345"},
		}
		got := convertTokenBalance(tb)
		require.Equal(t, uint16(15), got.AccountIndex)
		require.Equal(t, commonsol.PublicKey(owner), *got.Owner)
		require.Equal(t, commonsol.PublicKey(pid), *got.ProgramId)
		require.Equal(t, commonsol.PublicKey(pk(99)), got.Mint)
		require.Equal(t, "12345", got.UiTokenAmount.Amount)
		require.Equal(t, uint8(6), got.UiTokenAmount.Decimals)
		require.Equal(t, "12.345", got.UiTokenAmount.UiAmountString)
	})

	t.Run("convertInnerInstruction", func(t *testing.T) {
		in := rpc.InnerInstruction{
			Index: 2,
			Instructions: []rpc.CompiledInstruction{
				{ProgramIDIndex: 1, Accounts: []uint16{0, 2}, Data: []byte{0xaa}, StackHeight: 9},
			},
		}
		got := convertInnerInstruction(in)
		require.Equal(t, uint16(2), got.Index)
		require.Len(t, got.Instructions, 1)
		require.Equal(t, uint16(1), got.Instructions[0].ProgramIDIndex)
		require.Equal(t, []uint16{0, 2}, got.Instructions[0].Accounts)
		require.Equal(t, []byte{0xaa}, got.Instructions[0].Data)
		require.Equal(t, uint16(9), got.Instructions[0].StackHeight)
	})

	t.Run("convertTransactionMeta", func(t *testing.T) {
		owner := pk(7)
		prog := pk(8)
		meta := &rpc.TransactionMeta{
			Err:          map[string]any{"InstructionError": []any{uint64(0), "SomeError"}},
			Fee:          5000,
			PreBalances:  []uint64{1, 2},
			PostBalances: []uint64{3, 4},
			LogMessages:  []string{"a", "b"},
			InnerInstructions: []rpc.InnerInstruction{
				{Index: 0, Instructions: []rpc.CompiledInstruction{{ProgramIDIndex: 1, Accounts: []uint16{0}, Data: []byte{0x01}}}},
			},
			PreTokenBalances: []rpc.TokenBalance{
				{AccountIndex: 1, Owner: &owner, ProgramId: &prog, Mint: pk(10), UiTokenAmount: &rpc.UiTokenAmount{Amount: "1", Decimals: 0, UiAmountString: "1"}},
			},
			PostTokenBalances: []rpc.TokenBalance{
				{AccountIndex: 1, Owner: &owner, ProgramId: &prog, Mint: pk(10), UiTokenAmount: &rpc.UiTokenAmount{Amount: "2", Decimals: 0, UiAmountString: "2"}},
			},
			LoadedAddresses: rpc.LoadedAddresses{
				ReadOnly: []solanago.PublicKey{pk(1)},
				Writable: []solanago.PublicKey{pk(2)},
			},
			ReturnData: rpc.ReturnData{
				ProgramId: pk(3),
				Data:      solanago.Data{Content: []byte{0xde, 0xad}, Encoding: solanago.EncodingBase64},
			},
		}
		consumed := uint64(777)
		meta.ComputeUnitsConsumed = &consumed

		got := convertTransactionMeta(meta)
		require.NotNil(t, got)
		require.NotEmpty(t, got.Err)
		require.Equal(t, uint64(5000), got.Fee)
		require.Equal(t, []uint64{1, 2}, got.PreBalances)
		require.Equal(t, []uint64{3, 4}, got.PostBalances)
		require.Equal(t, []string{"a", "b"}, got.LogMessages)
		require.NotNil(t, got.ComputeUnitsConsumed)
		require.Equal(t, uint64(777), *got.ComputeUnitsConsumed)
		require.Len(t, got.InnerInstructions, 1)
		require.Len(t, got.PreTokenBalances, 1)
		require.Len(t, got.PostTokenBalances, 1)
		require.Equal(t, commonsol.PublicKey(pk(10)), got.PreTokenBalances[0].Mint)
		require.Equal(t, commonsol.PublicKeySlice{cpk(1)}, got.LoadedAddresses.ReadOnly)
		require.Equal(t, commonsol.PublicKeySlice{cpk(2)}, got.LoadedAddresses.Writable)
		require.Equal(t, commonsol.PublicKey(pk(3)), got.ReturnData.ProgramId)
		require.Equal(t, []byte{0xde, 0xad}, got.ReturnData.Data.Content)
		require.Equal(t, commonsol.EncodingType(solanago.EncodingBase64), got.ReturnData.Data.Encoding)
	})

	t.Run("convertTransactionMeta_nil", func(t *testing.T) {
		require.Nil(t, convertTransactionMeta(nil))
	})

	t.Run("convertAccountInfoOpts", func(t *testing.T) {
		opts := &commonsol.GetAccountInfoOpts{
			Encoding:       commonsol.EncodingBase64,
			Commitment:     commonsol.CommitmentConfirmed,
			DataSlice:      &commonsol.DataSlice{Offset: uint64Ptr(5), Length: uint64Ptr(9)},
			MinContextSlot: uint64Ptr(77),
		}
		got := convertAccountInfoOpts(opts)
		require.Equal(t, solanago.EncodingType(commonsol.EncodingBase64), got.Encoding)
		require.Equal(t, rpc.CommitmentType(commonsol.CommitmentConfirmed), got.Commitment)
		require.NotNil(t, got.DataSlice)
		require.Equal(t, uint64(5), *got.DataSlice.Offset)
		require.Equal(t, uint64(9), *got.DataSlice.Length)
		require.NotNil(t, got.MinContextSlot)
		require.Equal(t, uint64(77), *got.MinContextSlot)
	})

	t.Run("convertDataBytesOrJSON_nil", func(t *testing.T) {
		data, err := convertDataBytesOrJSON(nil, "")
		require.Nil(t, data)
		require.NoError(t, err)
	})
	t.Run("convertBlock_minimal", func(t *testing.T) {
		bt := solanago.UnixTimeSeconds(1730000000)
		rpcBlock := &rpc.GetBlockResult{
			Blockhash:         solanago.Hash(pk(1)),
			PreviousBlockhash: solanago.Hash(pk(2)),
			ParentSlot:        100,
			Signatures:        []solanago.Signature{sig(5)},
			BlockTime:         &bt,
			BlockHeight:       uint64Ptr(1234),
			Transactions: []rpc.TransactionWithMeta{
				{
					Version: rpc.TransactionVersion(0),
					Meta:    &rpc.TransactionMeta{Fee: 1},
				},
			},
		}

		got := convertBlock(rpcBlock)
		require.NotNil(t, got)
		require.Equal(t, commonsol.Hash(cpk(1)), got.Blockhash)
		require.Equal(t, commonsol.Hash(cpk(2)), got.PreviousBlockhash)
		require.Equal(t, uint64(100), got.ParentSlot)
		require.NotNil(t, got.BlockTime)
		require.Equal(t, uint64Ptr(1234), got.BlockHeight)
	})

	t.Run("convertTransaction_nilSafe", func(t *testing.T) {
		require.Nil(t, convertTransaction(nil))
	})

	t.Run("convertTransaction_basic", func(t *testing.T) {
		tx := &solanago.Transaction{
			Signatures: []solanago.Signature{sig(9)},
			Message: solanago.Message{
				AccountKeys:     solanago.PublicKeySlice{pk(7)},
				Header:          solanago.MessageHeader{},
				RecentBlockhash: solanago.Hash(pk(8)),
				Instructions: []solanago.CompiledInstruction{
					{ProgramIDIndex: 0, Accounts: []uint16{0}, Data: []byte{1}},
				},
			},
		}
		got := convertTransaction(tx)
		require.NotNil(t, got)
		require.Equal(t, []commonsol.Signature{csig(9)}, got.Signatures)
		require.Equal(t, cpk(7), got.Message.AccountKeys[0])
		require.Equal(t, commonsol.Hash(cpk(8)), got.Message.RecentBlockhash)
	})

	t.Run("convertTransactionEnvelope_nil", func(t *testing.T) {
		res, err := convertTransactionEnvelope(nil)
		require.NoError(t, err)
		require.Nil(t, res)
	})
	t.Run("convertTransaction", func(t *testing.T) {
		tx, err := solanago.TransactionFromBase64("AduqZjAgyh5j1WdY3U9AeS2ipk4CKvAwg05YgEE/PuGmiCKV01sK5OosREvDUtzYgEcy8udNEgrJ3h6EyNSiygoBAAEDW/Kcohx9SWr/V/UMmcy8RLIcyoTiGMJUzTO0hUeDFhBPITyQP/O3TBMr+8ECxBuHQ3bPl6iselx2P3Pd0jC7jQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAD4idjTDYMB0/8Mqa9G/bgm/1maapeTeQPGS9KIGaXpwBAgIAAQwCAAAAgJaYAAAAAAA=")
		require.NoError(t, err)
		got := convertTransaction(tx)

		require.Len(t, tx.Message.AccountKeys, len(got.Message.AccountKeys))
		for i := range tx.Message.AccountKeys {
			require.Equal(t, tx.Message.AccountKeys[i], solanago.PublicKey(got.Message.AccountKeys[i]))
		}
		require.Equal(t, tx.Message.Header.NumReadonlySignedAccounts, got.Message.Header.NumReadonlySignedAccounts)
		require.Equal(t, tx.Message.Header.NumReadonlyUnsignedAccounts, got.Message.Header.NumReadonlyUnsignedAccounts)
		require.Equal(t, tx.Message.Header.NumRequiredSignatures, got.Message.Header.NumRequiredSignatures)
	})

	t.Run("convertSimulateTXOpts_nil", func(t *testing.T) {
		opts := convertSimulateTXOpts(nil)
		require.NotNil(t, opts)
		require.Equal(t, rpc.CommitmentFinalized, opts.Commitment)
		require.Nil(t, opts.Accounts)
	})

	t.Run("convertAccountInfoOpts_nil", func(t *testing.T) {
		require.Empty(t, convertAccountInfoOpts(nil))
	})

	t.Run("convertProgramAccountsOpts_nil", func(t *testing.T) {
		opts, enc := convertProgramAccountsOpts(nil)
		require.Nil(t, opts)
		require.Equal(t, commonsol.EncodingType(""), enc)
	})

	t.Run("convertProgramAccountsOpts_full", func(t *testing.T) {
		offset := uint64(13)
		length := uint64(30)
		filterKey := pk(7)
		opts, enc := convertProgramAccountsOpts(&commonsol.GetProgramAccountsOpts{
			Encoding:   commonsol.EncodingBase64,
			Commitment: commonsol.CommitmentConfirmed,
			DataSlice:  &commonsol.DataSlice{Offset: &offset, Length: &length},
			Filters: []commonsol.RPCFilter{
				{DataSize: 165},
				{Memcmp: &commonsol.RPCFilterMemcmp{Offset: 0, Bytes: filterKey[:]}},
			},
		})
		require.Equal(t, commonsol.EncodingBase64, enc)
		require.NotNil(t, opts)
		require.Equal(t, solanago.EncodingBase64, opts.Encoding)
		require.Equal(t, rpc.CommitmentConfirmed, opts.Commitment)
		require.NotNil(t, opts.DataSlice)
		require.Equal(t, &offset, opts.DataSlice.Offset)
		require.Equal(t, &length, opts.DataSlice.Length)
		require.Len(t, opts.Filters, 2)
		require.Equal(t, uint64(165), opts.Filters[0].DataSize)
		require.NotNil(t, opts.Filters[1].Memcmp)
		require.Equal(t, uint64(0), opts.Filters[1].Memcmp.Offset)
		require.Equal(t, solanago.Base58(filterKey[:]), opts.Filters[1].Memcmp.Bytes)
	})

	t.Run("convertRPCFilters_empty", func(t *testing.T) {
		require.Nil(t, convertRPCFilters(nil))
	})

	t.Run("convertProgramAccountsReply", func(t *testing.T) {
		data := rpc.DataBytesOrJSONFromBytes([]byte{0xca, 0xfe})
		got, err := convertProgramAccountsReply(rpc.GetProgramAccountsResult{
			{
				Pubkey: pk(3),
				Account: &rpc.Account{
					Lamports:   500,
					Owner:      pk(4),
					Data:       data,
					Executable: false,
					Space:      32,
				},
			},
			nil,
		}, commonsol.EncodingBase64)
		require.NoError(t, err)
		require.Len(t, got.Value, 1)
		require.Equal(t, cpk(3), got.Value[0].Pubkey)
		require.Equal(t, uint64(500), got.Value[0].Account.Lamports)
		require.Equal(t, cpk(4), got.Value[0].Account.Owner)
		require.Equal(t, []byte{0xca, 0xfe}, got.Value[0].Account.Data.AsDecodedBinary)
	})
}

func Test_getPublicKeyWithHighestLamports(t *testing.T) {
	ctx := t.Context()
	ss := &solanaService{logger: logger.Test(t)}

	t.Run("single account", func(t *testing.T) {
		k, err := solanago.NewRandomPrivateKey()
		require.NoError(t, err)
		got, err := ss.getPublicKeyWithHighestLamports(ctx, nil, []string{k.PublicKey().String()})
		require.NoError(t, err)
		require.Equal(t, k.PublicKey(), got)
	})

	t.Run("picks highest balance", func(t *testing.T) {
		kLow, err := solanago.NewRandomPrivateKey()
		require.NoError(t, err)
		kHigh, err := solanago.NewRandomPrivateKey()
		require.NoError(t, err)

		mockR := clientmocks.NewReaderWriter(t)
		mockR.On("BalanceWithCommitment", mock.Anything, kLow.PublicKey(), rpc.CommitmentConfirmed).Return(&rpc.GetBalanceResult{Value: 100}, nil).Once()
		mockR.On("BalanceWithCommitment", mock.Anything, kHigh.PublicKey(), rpc.CommitmentConfirmed).Return(&rpc.GetBalanceResult{Value: 9000}, nil).Once()

		got, err := ss.getPublicKeyWithHighestLamports(ctx, mockR, []string{kLow.PublicKey().String(), kHigh.PublicKey().String()})
		require.NoError(t, err)
		require.Equal(t, kHigh.PublicKey(), got)
	})

	t.Run("fallback when all balances fail", func(t *testing.T) {
		k0, err := solanago.NewRandomPrivateKey()
		require.NoError(t, err)
		k1, err := solanago.NewRandomPrivateKey()
		require.NoError(t, err)

		mockR := clientmocks.NewReaderWriter(t)
		mockR.On("BalanceWithCommitment", mock.Anything, mock.Anything, rpc.CommitmentConfirmed).Return((*rpc.GetBalanceResult)(nil), errors.New("rpc down")).Twice()

		got, err := ss.getPublicKeyWithHighestLamports(ctx, mockR, []string{k0.PublicKey().String(), k1.PublicKey().String()})
		require.NoError(t, err)
		require.Equal(t, k0.PublicKey(), got)
	})
}

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

// submitStubChain implements the Chain interface with only the methods needed by
// SubmitTransaction, avoiding the import cycle that mocks/chain.go causes.
type submitStubChain struct {
	Chain
	reader    client.Reader
	readerErr error
	txm       TxManager
	cfg       config.Config
}

func (c *submitStubChain) Reader() (client.Reader, error) { return c.reader, c.readerErr }
func (c *submitStubChain) TxManager() TxManager           { return c.txm }
func (c *submitStubChain) Config() config.Config          { return c.cfg }

// stubAddressLister supplies enabled chain accounts for SubmitTransaction (fee payer selection).
type stubAddressLister struct {
	accounts []string
}

func (s *stubAddressLister) Accounts(ctx context.Context) ([]string, error) {
	return s.accounts, nil
}

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
func (w *stubWorkflow) RequestSizeLimit() uint32                          { return 0 }

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
		assert.Equal(t, uint64(42), got.Slot)
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
		assert.Equal(t, uint64(100), got.Slot)
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

	chain := &submitStubChain{
		reader: mockReader,
		txm:    txm,
		cfg:    mockCfg,
	}

	ss := &solanaService{
		chain:         chain,
		logger:        logger.Nop(),
		addressLister: &stubAddressLister{accounts: []string{pk(1).String()}},
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

	_, err := ss.SubmitTransaction(t.Context(), req)
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

	reply, err := ss.SubmitTransaction(t.Context(), req)
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
	ctx, cancel := context.WithCancel(t.Context())
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

	reply, err := ss.SubmitTransaction(t.Context(), req)
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

	reply, err := ss.SubmitTransaction(t.Context(), req)
	require.NoError(t, err)
	require.NotNil(t, reply)
	assert.Equal(t, commonsol.TxSuccess, reply.Status)
	assert.Nil(t, capturedCfgs, "no tx configs should be set when Cfg is nil")
}

func newGetProgramAccountsTestHarness(t *testing.T) (*solanaService, *clientmocks.ReaderWriter) {
	t.Helper()

	mockReader := clientmocks.NewReaderWriter(t)
	mockCfg := configmocks.NewConfig(t)
	mockCfg.EXPECT().WF().Return(&stubWorkflow{}).Maybe()

	chain := &submitStubChain{
		reader: mockReader,
		cfg:    mockCfg,
	}

	return &solanaService{
		chain:  chain,
		logger: logger.Nop(),
	}, mockReader
}

func TestGetProgramAccounts(t *testing.T) {
	ctx := t.Context()
	program := cpk(9)

	t.Run("success with opts", func(t *testing.T) {
		ss, mockReader := newGetProgramAccountsTestHarness(t)
		offset := uint64(0)
		length := uint64(32)
		req := commonsol.GetProgramAccountsRequest{
			Program: program,
			Opts: &commonsol.GetProgramAccountsOpts{
				Encoding:   commonsol.EncodingBase64,
				Commitment: commonsol.CommitmentConfirmed,
				DataSlice:  &commonsol.DataSlice{Offset: &offset, Length: &length},
				Filters:    []commonsol.RPCFilter{{DataSize: 165}},
			},
		}

		rpcResult := rpc.GetProgramAccountsResult{
			{
				Pubkey: pk(1),
				Account: &rpc.Account{
					Lamports: 1000,
					Owner:    solanago.PublicKey(program),
					Data:     rpc.DataBytesOrJSONFromBytes([]byte{0x01, 0x02}),
					Space:    165,
				},
			},
		}
		mockReader.EXPECT().
			GetProgramAccountsWithOpts(mock.Anything, solanago.PublicKey(program), mock.MatchedBy(func(opts *rpc.GetProgramAccountsOpts) bool {
				return opts != nil &&
					opts.Encoding == solanago.EncodingBase64 &&
					opts.Commitment == rpc.CommitmentConfirmed &&
					opts.DataSlice != nil &&
					*opts.DataSlice.Offset == offset &&
					*opts.DataSlice.Length == length &&
					len(opts.Filters) == 1 &&
					opts.Filters[0].DataSize == 165
			})).
			Return(rpcResult, nil)

		reply, err := ss.GetProgramAccounts(ctx, req)
		require.NoError(t, err)
		require.Len(t, reply.Value, 1)
		require.Equal(t, cpk(1), reply.Value[0].Pubkey)
		require.Equal(t, uint64(1000), reply.Value[0].Account.Lamports)
		require.Equal(t, program, reply.Value[0].Account.Owner)
		require.Equal(t, []byte{0x01, 0x02}, reply.Value[0].Account.Data.AsDecodedBinary)
	})

	t.Run("success with nil opts", func(t *testing.T) {
		ss, mockReader := newGetProgramAccountsTestHarness(t)
		req := commonsol.GetProgramAccountsRequest{Program: program}

		mockReader.EXPECT().
			GetProgramAccountsWithOpts(mock.Anything, solanago.PublicKey(program), (*rpc.GetProgramAccountsOpts)(nil)).
			Return(rpc.GetProgramAccountsResult{}, nil)

		reply, err := ss.GetProgramAccounts(ctx, req)
		require.NoError(t, err)
		require.Empty(t, reply.Value)
	})

	t.Run("reader error", func(t *testing.T) {
		ss, mockReader := newGetProgramAccountsTestHarness(t)
		req := commonsol.GetProgramAccountsRequest{Program: program}

		mockReader.EXPECT().
			GetProgramAccountsWithOpts(mock.Anything, solanago.PublicKey(program), (*rpc.GetProgramAccountsOpts)(nil)).
			Return(rpc.GetProgramAccountsResult(nil), errors.New("rpc unavailable"))

		_, err := ss.GetProgramAccounts(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get program accounts")
		assert.Contains(t, err.Error(), "rpc unavailable")
	})

	t.Run("chain reader error", func(t *testing.T) {
		ss := &solanaService{
			chain: &submitStubChain{readerErr: errors.New("no reader")},
		}
		_, err := ss.GetProgramAccounts(ctx, commonsol.GetProgramAccountsRequest{Program: program})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get reader")
	})
}

// mockLogPoller is a minimal LogPoller implementation for testing readiness guards.
type mockLogPoller struct {
	ready error
}

func (m *mockLogPoller) Start(context.Context) error            { return nil }
func (m *mockLogPoller) Ready() error                           { return m.ready }
func (m *mockLogPoller) Close() error                           { return nil }
func (m *mockLogPoller) HasFilter(context.Context, string) bool { return false }
func (m *mockLogPoller) RegisterFilter(context.Context, logpollertypes.Filter) error {
	return nil
}
func (m *mockLogPoller) UnregisterFilter(context.Context, string) error { return nil }
func (m *mockLogPoller) GetFilters(context.Context) (map[string]logpollertypes.Filter, error) {
	return nil, nil
}
func (m *mockLogPoller) GetLatestBlock(context.Context) (int64, error) { return 0, nil }
func (m *mockLogPoller) FilteredLogs(context.Context, []query.Expression, query.LimitAndSort, string) ([]logpollertypes.Log, error) {
	return nil, nil
}
func (m *mockLogPoller) Replay(int64)           {}
func (m *mockLogPoller) CPIEventsEnabled() bool { return false }

// stubChain is a minimal Chain stub that only provides a LogPoller.
// It embeds the Chain interface so unimplemented methods panic if called.
type lpStubChain struct {
	Chain
	lp LogPoller
}

func (s *lpStubChain) LogPoller() LogPoller { return s.lp }

func newSolanaService(t *testing.T, lpReady error) *solanaService {
	t.Helper()
	return &solanaService{
		chain:  &lpStubChain{lp: &mockLogPoller{ready: lpReady}},
		logger: logger.Test(t),
	}
}

func TestLogPollerReadinessGuard(t *testing.T) {
	ctx := t.Context()
	errNotStarted := errors.New("not started")

	t.Run("RegisterLogTracking returns error when LogPoller not started", func(t *testing.T) {
		ss := newSolanaService(t, errNotStarted)
		err := ss.RegisterLogTracking(ctx, commonsol.LPFilterQuery{Name: "test"})
		require.ErrorIs(t, err, ErrLogPollerNotStarted)
	})

	t.Run("RegisterLogTracking succeeds when LogPoller is ready", func(t *testing.T) {
		ss := newSolanaService(t, nil)
		err := ss.RegisterLogTracking(ctx, commonsol.LPFilterQuery{
			Name:            "test",
			ContractIdlJSON: []byte(`{}`),
		})
		require.NoError(t, err)
	})

	t.Run("UnregisterLogTracking returns error when LogPoller not started", func(t *testing.T) {
		ss := newSolanaService(t, errNotStarted)
		err := ss.UnregisterLogTracking(ctx, "test")
		require.ErrorIs(t, err, ErrLogPollerNotStarted)
	})

	t.Run("UnregisterLogTracking succeeds when LogPoller is ready", func(t *testing.T) {
		ss := newSolanaService(t, nil)
		err := ss.UnregisterLogTracking(ctx, "test")
		require.NoError(t, err)
	})

	t.Run("QueryTrackedLogs returns error when LogPoller not started", func(t *testing.T) {
		ss := newSolanaService(t, errNotStarted)
		_, err := ss.QueryTrackedLogs(ctx, nil, query.LimitAndSort{})
		require.ErrorIs(t, err, ErrLogPollerNotStarted)
	})

	t.Run("GetLatestLPBlock returns error when LogPoller not started", func(t *testing.T) {
		ss := newSolanaService(t, errNotStarted)
		_, err := ss.GetLatestLPBlock(ctx)
		require.ErrorIs(t, err, ErrLogPollerNotStarted)
	})

	t.Run("GetLatestLPBlock succeeds when LogPoller is ready", func(t *testing.T) {
		ss := newSolanaService(t, nil)
		block, err := ss.GetLatestLPBlock(ctx)
		require.NoError(t, err)
		require.NotNil(t, block)
		require.Equal(t, uint64(0), block.Slot)
	})

	t.Run("GetFiltersNames returns error when LogPoller not started", func(t *testing.T) {
		ss := newSolanaService(t, errNotStarted)
		_, err := ss.GetFiltersNames(ctx)
		require.ErrorIs(t, err, ErrLogPollerNotStarted)
	})

	t.Run("GetFiltersNames succeeds when LogPoller is ready", func(t *testing.T) {
		ss := newSolanaService(t, nil)
		names, err := ss.GetFiltersNames(ctx)
		require.NoError(t, err)
		require.Empty(t, names)
	})
}
