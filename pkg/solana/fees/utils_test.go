package fees

import (
	"fmt"
	"testing"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculateFee(t *testing.T) {
	inputs := []struct {
		base, max, min uint64
		count          uint
		expected       uint64
	}{
		{0, 0, 0, 100, 0},    // test max
		{0, 10, 1, 0, 1},     // test min
		{0, 10, 0, 0, 0},     // test 0 count should return base
		{0, 10, 0, 1, 1},     // test 1 count on 0 base should return 1
		{0, 10, 0, 2, 2},     // test 2 count on 0 base should return 2
		{0, 10, 0, 3, 4},     // test 3 count on 0 base should return 4
		{0, 10, 0, 4, 8},     // test 4 count on 0 base should return 8
		{1, 10, 0, 0, 1},     // test 0 count on 1 base should return 1
		{1, 10, 0, 1, 2},     // test 1 count on 1 base should return 2
		{1, 100, 0, 64, 100}, // test 64 bcount on 1 base should return max (overflow)
	}

	for i, v := range inputs {
		t.Run(fmt.Sprintf("inputs[%d]", i), func(t *testing.T) {
			assert.Equal(t, v.expected, CalculateFee(v.base, v.max, v.min, v.count))
		})
	}
}

func TestParseBlock(t *testing.T) {
	testBlocks := readMultipleBlocksFromFile(t, "./multiple_blocks_data.json")
	lastBlock := testBlocks[len(testBlocks)-1]
	assert.Equal(t, 3, len(lastBlock.Transactions))

	// happy path - filtered for non-vote txs
	out, err := ParseBlock(lastBlock)
	require.NoError(t, err)
	assert.Equal(t, len(out.Prices), len(out.Fees))
	assert.Equal(t, 2, len(out.Prices))

	// fail nil
	_, err = ParseBlock(nil)
	require.Error(t, err)

	// skip on nil meta
	out, err = ParseBlock(&rpc.GetBlockResult{
		Transactions: []rpc.TransactionWithMeta{{}},
	})
	require.NoError(t, err)
	assert.Equal(t, 0, len(out.Prices))

	// error on failed tx parsing
	_, err = ParseBlock(&rpc.GetBlockResult{
		Transactions: []rpc.TransactionWithMeta{{
			Transaction: &rpc.DataBytesOrJSON{},
			Meta:        &rpc.TransactionMeta{},
		}},
	})
	assert.Error(t, err)
}
