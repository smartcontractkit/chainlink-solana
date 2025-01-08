package logpoller

import (
	"testing"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
	"github.com/stretchr/testify/mock"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	clientmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
)

type mockTypeProvider struct{}

func (tp mockTypeProvider) CreateType(eventIdl codec.IdlEvent, typedefSlice codec.IdlTypeDefSlice, subKeyPath []string) (any, error) {
	return nil, nil
}

func TestProcess(t *testing.T) {
	ctx := tests.Context(t)

	addr := newRandomPublicKey(t)
	sig := newRandomEventSignature(t)

	orm := newMockORM(t)
	cl := clientmocks.NewReaderWriter(t)
	loader := utils.NewLazyLoad(func() (client.Reader, error) { return cl, nil })
	lggr := logger.Sugared(logger.Test(t))
	tp := mockTypeProvider{}
	lp := New(lggr, orm, loader, tp)

	filter := Filter{
		Name:     "test filter",
		Address:  addr,
		EventSig: sig,
	}
	orm.EXPECT().SelectFilters(mock.Anything).Return([]Filter{filter}, nil)
	orm.EXPECT().SelectSeqNums(mock.Anything).Return(map[int64]int64{}, nil)
	orm.EXPECT().InsertFilter(mock.Anything, mock.Anything).Return(1, nil)
	lp.RegisterFilter(ctx, filter)

	ev := ProgramEvent{
		Program:   "myprog",
		Prefix:    "prefix",
		BlockData: BlockData{SlotNumber: 3, BlockHeight: 5},
	}
	lp.Process(ev)

	lp.UnregisterFilter(ctx, filter.Name)
}
