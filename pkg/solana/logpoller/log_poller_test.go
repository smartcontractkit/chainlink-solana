package logpoller

import (
	"context"
	"testing"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	clientmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/mocks"
)

func TestProcess(t *testing.T) {
	ctx := tests.Context(t)

	addr := newRandomPublicKey(t)
	sig := newRandomEventSignature(t)

	orm := newMockORM(t)
	cl := clientmocks.NewReaderWriter(t)
	loader := utils.NewLazyLoad(func() (client.Reader, error) { return cl, nil })
	lggr := logger.Sugared(logger.Test(t))
	lp := New(lggr, orm, loader)

	filter := Filter{
		Name:     "test filter",
		Address:  addr,
		EventSig: sig,
	}
	orm.EXPECT().SelectFilters(mock.Anything).Return([]Filter{filter}, nil)
	orm.EXPECT().SelectSeqNums(mock.Anything).Return(map[int64]int64{}, nil)
	orm.EXPECT().InsertFilter(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, f Filter) (int64, error) {
		require.Equal(t, f, filter)
		return 1, nil
	}).Once()
	// TODO orm.EXPECT().InsertLogs(mock.Anything, mock.Anything).RunAndReturn(nil) validate logs written properly
	lp.RegisterFilter(ctx, filter)

	ev := ProgramEvent{
		Program:   "myprog", // TODO: fix program address, so filters match
		Prefix:    "prefix",
		BlockData: BlockData{SlotNumber: 3, BlockHeight: 5},
	}
	err := lp.Process(ev)
	require.NoError(t, err)

	orm.EXPECT().MarkFilterDeleted(mock.Anything, mock.Anything).Return(nil).Once()
	lp.UnregisterFilter(ctx, filter.Name)
}
