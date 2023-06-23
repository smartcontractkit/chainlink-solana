package types_test

import (
	"strconv"
	"testing"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/smartcontractkit/chainlink-solana/pkg/internal/cltest"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/headtracker/types"
	"github.com/stretchr/testify/assert"
)

func TestHead_NewHead(t *testing.T) {
	emptyBlockResult := cltest.ConfigureBlockResult()
	t.Parallel()

	tests := []struct {
		slot     int64
		block    rpc.GetBlockResult
		parent   *types.Head
		id       types.ChainID
		wantSlot int64
	}{
		// with no parent
		{10, emptyBlockResult, nil, types.Mainnet, 10},
		// with parent
		{20, emptyBlockResult,
			types.NewHead(10, emptyBlockResult, nil, types.Mainnet),
			types.Mainnet, 20},
		{30, emptyBlockResult,
			types.NewHead(20, emptyBlockResult,
				types.NewHead(10, emptyBlockResult, nil, types.Mainnet),
				types.Mainnet),
			types.Mainnet, 30},
	}

	for _, test := range tests {
		t.Run(
			strconv.FormatInt(test.wantSlot, 10), // convert to base 10
			func(t *testing.T) {
				head := types.NewHead(test.slot, test.block, test.parent, test.id)
				assert.Equal(t, test.wantSlot, head.Slot)
				assert.Equal(t, test.block, head.Block)
				assert.Equal(t, test.parent, head.Parent)
				assert.Equal(t, test.id, head.ID)
			})
	}
}

func TestHead_ChainLength(t *testing.T) {
	blockResult := cltest.ConfigureBlockResult()
	id := types.Mainnet

	head := types.NewHead(0, blockResult,
		types.NewHead(0, blockResult,
			types.NewHead(0, blockResult, nil, id), id), id)

	assert.Equal(t, uint32(3), head.ChainLength())

	var head2 *types.Head
	assert.Equal(t, uint32(0), head2.ChainLength())
}

func TestHead_EarliestHeadInChain(t *testing.T) {
	blockResult := cltest.ConfigureBlockResult()
	id := types.Mainnet

	head := types.NewHead(3, blockResult,
		types.NewHead(2, blockResult,
			types.NewHead(1, blockResult, nil, id), id), id)

	assert.Equal(t, int64(1), head.EarliestHeadInChain().BlockNumber())
}

func TestHead_GetParentHash(t *testing.T) {
	blockResult := cltest.ConfigureBlockResult()
	id := types.Mainnet

	head := types.NewHead(3, blockResult,
		types.NewHead(2, blockResult,
			types.NewHead(1, blockResult, nil, id), id), id)

	assert.Equal(t, head.Parent.BlockHash(), head.GetParentHash())
}

func TestHead_GetParent(t *testing.T) {
	blockResult := cltest.ConfigureBlockResult()
	id := types.Mainnet

	head := types.NewHead(3, blockResult,
		types.NewHead(2, blockResult,
			types.NewHead(1, blockResult, nil, id), id), id)

	assert.Equal(t, head.Parent, head.GetParent())
}

func TestHead_HasChainID(t *testing.T) {
	t.Parallel()
	blockResult := cltest.ConfigureBlockResult() // Assuming this function creates a mock rpc.GetBlockResult

	tests := []struct {
		name    string
		chainID types.ChainID
		want    bool
	}{
		{
			"HasChainID returns true when ChainID is not 'unknown'",
			types.Devnet, // replace with correct initialization
			true,
		},
		{
			"HasChainID returns false when ChainID is 'unknown'",
			99,
			false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			head := types.NewHead(0, blockResult, nil, test.chainID)
			assert.Equal(t, test.want, head.HasChainID())
		})
	}

	t.Run("HasChainID returns false when Head is nil", func(t *testing.T) {
		var head *types.Head
		assert.False(t, head.HasChainID())
	})
}
