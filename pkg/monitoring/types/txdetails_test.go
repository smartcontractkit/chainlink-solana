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
	sampleTxResultSigner  = solana.MustPublicKeyFromBase58("9YR7YttJFfptQJSo5xrnYoAw1fJyVonC1vxUSqzAgyjY")
	sampleTxResultProgram = solana.MustPublicKeyFromBase58("cjg3oHmg9uuPsP8D6g29NWvhySJkdYdAo9D25PRbKXJ")
	sampleTxResultNodes   = map[solana.PublicKey]string{
		sampleTxResultSigner: "test-signer",
	}

	sampleTxResult     = rpc.GetTransactionResult{}
	sampleTxResultJSON = `{"blockTime":1712887149,"meta":{"computeUnitsConsumed":64949,"err":null,"fee":5000,"innerInstructions":[{"index":1,"instructions":[{"accounts":[2,4],"data":"6y43XFem5gk9n8ESJ4pGFboagJiimTtvvy2VCjAUur3y","programIdIndex":3,"stackHeight":2}]}],"loadedAddresses":{"readonly":[],"writable":[]},"logMessages":["Program ComputeBudget111111111111111111111111111111 invoke [1]","Program ComputeBudget111111111111111111111111111111 success","Program cjg3oHmg9uuPsP8D6g29NWvhySJkdYdAo9D25PRbKXJ invoke [1]","Program data: gjbLTR5rT6hW4eUAAAN30/iLBm0GRKxe6y9hGtvvKCPLmscA16aVgw6AKe17ouFpAAAAAAAAAAAAAAAAA2uVGGYEAwECAAAAAAAAAAAAAAAAAAAAAKom6kICAAAAsr0AAAAAAAA=","Program HEvSKofvBgfaexv23kMabbYqxasxU3mQ4ibBMEmJWHny invoke [2]","Program log: Instruction: Submit","Program HEvSKofvBgfaexv23kMabbYqxasxU3mQ4ibBMEmJWHny consumed 4427 of 140121 compute units","Program HEvSKofvBgfaexv23kMabbYqxasxU3mQ4ibBMEmJWHny success","Program cjg3oHmg9uuPsP8D6g29NWvhySJkdYdAo9D25PRbKXJ consumed 64799 of 199850 compute units","Program cjg3oHmg9uuPsP8D6g29NWvhySJkdYdAo9D25PRbKXJ success"],"postBalances":[1019067874127,49054080,2616960,1141440,0,0,1141440,1],"postTokenBalances":[],"preBalances":[1019067879127,49054080,2616960,1141440,0,0,1141440,1],"preTokenBalances":[],"rewards":[],"status":{"Ok":null}},"slot":291748793,"transaction":{"message":{"accountKeys":["9YR7YttJFfptQJSo5xrnYoAw1fJyVonC1vxUSqzAgyjY","Ghm1a2c2NGPg6pKGG3PP1GLAuJkHm1RKMPqqJwPM7JpJ","HXoZZBWv25N4fm2vfSKnHXTeDJ31qaAcWZe3ZKeM6dQv","HEvSKofvBgfaexv23kMabbYqxasxU3mQ4ibBMEmJWHny","3u6T92C2x18s39a7WNM8NGaQK1YEtstTtqamZGsLvNZN","Sysvar1nstructions1111111111111111111111111","cjg3oHmg9uuPsP8D6g29NWvhySJkdYdAo9D25PRbKXJ","ComputeBudget111111111111111111111111111111"],"header":{"numReadonlySignedAccounts":0,"numReadonlyUnsignedAccounts":5,"numRequiredSignatures":1},"instructions":[{"accounts":[],"data":"3DTZbgwsozUF","programIdIndex":7,"stackHeight":null},{"accounts":[1,0,2,3,4,5],"data":"4W4pS7SH6dugDLwXWijhmW3dGTP7WENQa9vbUjvati1j95ghou2jUJHxvPUoowhZk2bHk21uKk4uFRQrpVF5e54NejQLtAT4DeZPC8n3QudjXhAHgBvFjYvDZDhCKRBK4nvdysDh7aKSE4nb3RiampwUo4u5WsKFfXYZnzbn8edC6jwuJVju1DczQPiLuzuCUps99C8rxwE9XkonGMrjc3Pj4cArMggk5fitRkfdaUn4mGRXDHzPFSg63YTZEn7tnnJd8pWEu9v9H8wBKcN1ptLiY5QmKSnayRcfYvd8MZ9wWf8bD7iVGSNUnwJToyFBVyBNabibozthXSDNmxr3yz1uR9vE3HFq6C2i1LX32a2aqZWzJjmvgdVNfNZZxqDxR6GvWYMw35","programIdIndex":6,"stackHeight":null}],"recentBlockhash":"BKUsMxK39LcgXKm8j5LuYyhig2kgQtRBkxR89szEzaSU"},"signatures":["2eEb8FeJyhczELJ3XKc6yvNLi3jYoC9vdpaR6WUN5vJ3f15ZV1d7LGZZqrqseQFEedgE4cxwcd3S3jYLmvJWBrNg"]}}`
)

func init() {
	if err := json.Unmarshal([]byte(sampleTxResultJSON), &sampleTxResult); err != nil {
		panic("unable to unmarshal sampleTxResult")
	}
}

func TestParseTxResult(t *testing.T) {
	// nil transaction result
	_, err := ParseTxResult(nil, map[solana.PublicKey]string{}, solana.PublicKey{})
	require.ErrorContains(t, err, "txResult is nil")
	// nil tx result meta
	_, err = ParseTxResult(&rpc.GetTransactionResult{}, map[solana.PublicKey]string{}, solana.PublicKey{})
	require.ErrorContains(t, err, "txResult.Meta")
	// nil tx result transaction
	_, err = ParseTxResult(&rpc.GetTransactionResult{
		Meta: &rpc.TransactionMeta{},
	}, map[solana.PublicKey]string{}, solana.PublicKey{})
	require.ErrorContains(t, err, "txResult.Transaction")

	// happy path
	res, err := ParseTxResult(&sampleTxResult, sampleTxResultNodes, sampleTxResultProgram)
	require.NoError(t, err)

	assert.Equal(t, nil, res.Err)
	assert.Equal(t, uint64(5000), res.Fee)
}

func TestParseTx(t *testing.T) {
	_, err := ParseTx(nil, sampleTxResultNodes, sampleTxResultProgram)
	require.ErrorContains(t, err, "tx is nil")

	_, err = ParseTx(&solana.Transaction{}, nil, sampleTxResultProgram)
	require.ErrorContains(t, err, "nodes is nil")

	tx, err := sampleTxResult.Transaction.GetTransaction()
	require.NoError(t, err)
	require.NotNil(t, tx)

	txMissingSig := *tx // copy
	txMissingSig.Signatures = []solana.Signature{}
	_, err = ParseTx(&txMissingSig, sampleTxResultNodes, sampleTxResultProgram)
	require.ErrorContains(t, err, "invalid number of signatures")

	txMissingAccounts := *tx // copy
	txMissingAccounts.Message.AccountKeys = []solana.PublicKey{}
	_, err = ParseTx(&txMissingAccounts, sampleTxResultNodes, sampleTxResultProgram)
	require.ErrorContains(t, err, "invalid number of signatures")

	_, err = ParseTx(tx, map[solana.PublicKey]string{}, sampleTxResultProgram)
	require.ErrorContains(t, err, "unknown public key")

	prevIndex := tx.Message.Instructions[1].ProgramIDIndex
	txInvalidProgramIndex := *tx                                       // copy
	txInvalidProgramIndex.Message.Instructions[1].ProgramIDIndex = 100 // index 1 is ocr transmit call
	out, err := ParseTx(&txInvalidProgramIndex, sampleTxResultNodes, sampleTxResultProgram)
	require.NoError(t, err)
	assert.Equal(t, 0, len(out.ObservationCount))
	tx.Message.Instructions[1].ProgramIDIndex = prevIndex // reset - something shares memory underneath

	// don't match program
	out, err = ParseTx(tx, sampleTxResultNodes, solana.PublicKey{})
	require.NoError(t, err)
	assert.Equal(t, 0, len(out.ObservationCount))

	// happy path
	out, err = ParseTx(tx, sampleTxResultNodes, sampleTxResultProgram)
	require.NoError(t, err)
	assert.Equal(t, sampleTxResultSigner, out.Sender)
	assert.Equal(t, sampleTxResultNodes[sampleTxResultSigner], out.Operator)
	assert.Equal(t, 1, len(out.ObservationCount))
	assert.Equal(t, uint8(4), out.ObservationCount[0])

	// multiple decodable instructions - currently not the case
	txMultipleTransmit := *tx
	txMultipleTransmit.Message.Instructions = append(tx.Message.Instructions, tx.Message.Instructions[1])
	out, err = ParseTx(&txMultipleTransmit, sampleTxResultNodes, sampleTxResultProgram)
	require.NoError(t, err)
	assert.Equal(t, 2, len(out.ObservationCount))
}
