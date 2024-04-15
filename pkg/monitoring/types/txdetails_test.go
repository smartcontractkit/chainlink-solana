package types

import (
	"encoding/json"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	sampleTxResultSigner = solana.MustPublicKeyFromBase58("9YR7YttJFfptQJSo5xrnYoAw1fJyVonC1vxUSqzAgyjY")

	sampleTxResult = rpc.GetTransactionResult{}
)

func init() {
	if err := json.Unmarshal([]byte(SampleTxResultJSON), &sampleTxResult); err != nil {
		panic("unable to unmarshal sampleTxResult")
	}
}

func TestParseTxResult(t *testing.T) {
	// nil transaction result
	_, err := ParseTxResult(nil, solana.PublicKey{})
	require.ErrorContains(t, err, "txResult is nil")
	// nil tx result meta
	_, err = ParseTxResult(&rpc.GetTransactionResult{}, solana.PublicKey{})
	require.ErrorContains(t, err, "txResult.Meta")
	// nil tx result transaction
	_, err = ParseTxResult(&rpc.GetTransactionResult{
		Meta: &rpc.TransactionMeta{},
	}, solana.PublicKey{})
	require.ErrorContains(t, err, "txResult.Transaction")

	// happy path
	res, err := ParseTxResult(&sampleTxResult, SampleTxResultProgram)
	require.NoError(t, err)

	assert.Equal(t, nil, res.Err)
	assert.Equal(t, uint64(5000), res.Fee)
}

func TestParseTx(t *testing.T) {
	_, err := ParseTx(nil, SampleTxResultProgram)
	require.ErrorContains(t, err, "tx is nil")

	tx, err := sampleTxResult.Transaction.GetTransaction()
	require.NoError(t, err)
	require.NotNil(t, tx)

	txMissingSig := *tx // copy
	txMissingSig.Signatures = []solana.Signature{}
	_, err = ParseTx(&txMissingSig, SampleTxResultProgram)
	require.ErrorContains(t, err, "invalid number of signatures")

	txMissingAccounts := *tx // copy
	txMissingAccounts.Message.AccountKeys = []solana.PublicKey{}
	_, err = ParseTx(&txMissingAccounts, SampleTxResultProgram)
	require.ErrorContains(t, err, "invalid number of signatures")

	prevIndex := tx.Message.Instructions[1].ProgramIDIndex
	txInvalidProgramIndex := *tx                                       // copy
	txInvalidProgramIndex.Message.Instructions[1].ProgramIDIndex = 100 // index 1 is ocr transmit call
	out, err := ParseTx(&txInvalidProgramIndex, SampleTxResultProgram)
	require.Error(t, err)
	tx.Message.Instructions[1].ProgramIDIndex = prevIndex // reset - something shares memory underneath

	// don't match program
	out, err = ParseTx(tx, solana.PublicKey{})
	require.Error(t, err)

	// happy path
	out, err = ParseTx(tx, SampleTxResultProgram)
	require.NoError(t, err)
	assert.Equal(t, sampleTxResultSigner, out.Sender)
	assert.Equal(t, uint8(4), out.ObservationCount)

	// multiple instructions - currently not the case
	txMultipleTransmit := *tx
	txMultipleTransmit.Message.Instructions = append(tx.Message.Instructions, tx.Message.Instructions[1])
	out, err = ParseTx(&txMultipleTransmit, SampleTxResultProgram)
	require.Error(t, err)
}
