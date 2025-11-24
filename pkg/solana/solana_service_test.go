package solana

import (
	"testing"

	solana "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	commonsol "github.com/smartcontractkit/chainlink-common/pkg/types/chains/solana"
)

func Test_Converters(t *testing.T) {
	t.Run("convertMessageHeader", func(t *testing.T) {
		h := solana.MessageHeader{
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
		ix := solana.CompiledInstruction{
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
		in := []solana.MessageAddressTableLookup{
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
		m := solana.Message{
			AccountKeys: solana.PublicKeySlice{pk(1), pk(2)},
			Header: solana.MessageHeader{
				NumRequiredSignatures:       2,
				NumReadonlySignedAccounts:   1,
				NumReadonlyUnsignedAccounts: 0,
			},
			RecentBlockhash: solana.Hash(pk(9)),
			Instructions: []solana.CompiledInstruction{
				{ProgramIDIndex: 0, Accounts: []uint16{0, 1}, Data: []byte("hi")},
			},
			AddressTableLookups: []solana.MessageAddressTableLookup{
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
				ReadOnly: []solana.PublicKey{pk(1)},
				Writable: []solana.PublicKey{pk(2)},
			},
			ReturnData: rpc.ReturnData{
				ProgramId: pk(3),
				Data:      solana.Data{Content: []byte{0xde, 0xad}, Encoding: solana.EncodingBase64},
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
		require.Equal(t, commonsol.EncodingType(solana.EncodingBase64), got.ReturnData.Data.Encoding)
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
		require.Equal(t, solana.EncodingType(commonsol.EncodingBase64), got.Encoding)
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
		bt := solana.UnixTimeSeconds(1730000000)
		rpcBlock := &rpc.GetBlockResult{
			Blockhash:         solana.Hash(pk(1)),
			PreviousBlockhash: solana.Hash(pk(2)),
			ParentSlot:        100,
			Signatures:        []solana.Signature{sig(5)},
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
		tx := &solana.Transaction{
			Signatures: []solana.Signature{sig(9)},
			Message: solana.Message{
				AccountKeys:     solana.PublicKeySlice{pk(7)},
				Header:          solana.MessageHeader{},
				RecentBlockhash: solana.Hash(pk(8)),
				Instructions: []solana.CompiledInstruction{
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
		tx, err := solana.TransactionFromBase64("AduqZjAgyh5j1WdY3U9AeS2ipk4CKvAwg05YgEE/PuGmiCKV01sK5OosREvDUtzYgEcy8udNEgrJ3h6EyNSiygoBAAEDW/Kcohx9SWr/V/UMmcy8RLIcyoTiGMJUzTO0hUeDFhBPITyQP/O3TBMr+8ECxBuHQ3bPl6iselx2P3Pd0jC7jQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAD4idjTDYMB0/8Mqa9G/bgm/1maapeTeQPGS9KIGaXpwBAgIAAQwCAAAAgJaYAAAAAAA=")
		require.NoError(t, err)
		got := convertTransaction(tx)

		require.Len(t, tx.Message.AccountKeys, len(got.Message.AccountKeys))
		for i := range tx.Message.AccountKeys {
			require.Equal(t, tx.Message.AccountKeys[i], solana.PublicKey(got.Message.AccountKeys[i]))
		}
		require.Equal(t, tx.Message.Header.NumReadonlySignedAccounts, got.Message.Header.NumReadonlySignedAccounts)
		require.Equal(t, tx.Message.Header.NumReadonlyUnsignedAccounts, got.Message.Header.NumReadonlyUnsignedAccounts)
		require.Equal(t, tx.Message.Header.NumRequiredSignatures, got.Message.Header.NumRequiredSignatures)
	})
}

func pk(i byte) solana.PublicKey {
	var p solana.PublicKey
	for j := range p {
		p[j] = i
	}
	return p
}

func cpk(i byte) commonsol.PublicKey { return commonsol.PublicKey(pk(i)) }

func sig(i byte) solana.Signature {
	var s solana.Signature
	for j := range s {
		s[j] = i
	}
	return s
}

func csig(i byte) commonsol.Signature { return commonsol.Signature(sig(i)) }

func uint64Ptr(v uint64) *uint64 { return &v }
