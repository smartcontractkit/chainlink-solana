package types

import (
	"encoding/json"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getTestTxResult(t *testing.T) *rpc.GetTransactionResult {
	out := &rpc.GetTransactionResult{}
	require.NoError(t, json.Unmarshal([]byte(SampleTxResultJSON), out))
	return out
}

func getTestTx(t *testing.T) *solana.Transaction {
	tx, err := getTestTxResult(t).Transaction.GetTransaction()
	require.NoError(t, err)
	require.NotNil(t, tx)
	return tx
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
	res, err := ParseTxResult(getTestTxResult(t), SampleTxResultProgram)
	require.NoError(t, err)

	assert.Equal(t, nil, res.Err)
	assert.Equal(t, uint64(5000), res.Fee)
}

func TestParseTx(t *testing.T) {
	_, err := ParseTx(nil, SampleTxResultProgram)
	require.ErrorContains(t, err, "tx is nil")

	txMissingSig := getTestTx(t) // copy
	txMissingSig.Signatures = []solana.Signature{}
	_, err = ParseTx(txMissingSig, SampleTxResultProgram)
	require.ErrorContains(t, err, "invalid number of signatures")

	txMissingAccounts := getTestTx(t) // copy
	txMissingAccounts.Message.AccountKeys = []solana.PublicKey{}
	_, err = ParseTx(txMissingAccounts, SampleTxResultProgram)
	require.ErrorContains(t, err, "invalid number of signatures")

	txInvalidProgramIndex := getTestTx(t)                              // copy
	txInvalidProgramIndex.Message.Instructions[1].ProgramIDIndex = 100 // index 1 is ocr transmit call
	out, err := ParseTx(txInvalidProgramIndex, SampleTxResultProgram)
	require.Error(t, err)

	// don't match program
	out, err = ParseTx(getTestTx(t), solana.PublicKey{})
	require.Error(t, err)

	// invalid length transmit instruction + compute budget instruction
	txInvalidTransmitInstruction := getTestTx(t)
	txInvalidTransmitInstruction.Message.Instructions[0].Data = []byte{}
	txInvalidTransmitInstruction.Message.Instructions[1].Data = []byte{}
	_, err = ParseTx(txInvalidTransmitInstruction, SampleTxResultProgram)
	require.ErrorContains(t, err, "transmit: invalid instruction length")

	require.ErrorContains(t, err, "computeUnitPrice")

	// happy path
	out, err = ParseTx(getTestTx(t), SampleTxResultProgram)
	require.NoError(t, err)
	assert.Equal(t, sampleTxResultSigner, out.Sender)
	assert.Equal(t, uint8(4), out.ObservationCount)
	assert.Equal(t, fees.ComputeUnitPrice(0), out.ComputeUnitPrice)

	// multiple instructions - currently not the case
	txMultipleTransmit := getTestTx(t)
	txMultipleTransmit.Message.Instructions = append(txMultipleTransmit.Message.Instructions, getTestTx(t).Message.Instructions[1])
	out, err = ParseTx(txMultipleTransmit, SampleTxResultProgram)
	require.Error(t, err)
}
