package logpoller

import (
	"bytes"
	"runtime"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	solcommoncodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/common"
	codecv1 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v1"
	codecv2 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v2"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
)

// oversizedTestItemEvent builds a TestItem payload whose Accounts field, a
// vec of publicKey, claims math.MaxInt32 elements. Left unbounded, the 32 byte
// element size turns those four bytes into a 64 GiB allocation.
func oversizedTestItemEvent(t *testing.T) []byte {
	t.Helper()

	buf := new(bytes.Buffer)
	buf.Write(solcommoncodec.NewDiscriminatorHashPrefix("TestItem", false))
	buf.Write(make([]byte, 4))  // Field i32
	buf.Write(make([]byte, 1))  // OracleId u8
	buf.Write(make([]byte, 32)) // OracleIds [u8; 32]
	buf.Write(make([]byte, 64)) // AccountStruct, two publicKeys
	// Accounts: little endian length prefix claiming math.MaxInt32 entries
	buf.Write([]byte{0xFF, 0xFF, 0xFF, 0x7F})

	return buf.Bytes()
}

// TestFilters_DecodeSubKey_RejectsOversizedVec covers the log poller ingestion
// path reached from solanaService.RegisterLogTracking, where the IDL is supplied
// by the caller and event data comes straight off chain.
func TestFilters_DecodeSubKey_RejectsOversizedVec(t *testing.T) {
	for _, tt := range []struct {
		name string
		idl  string
		id   int64
	}{
		{"codec v1", codecv1.FetchLogpollerTypeTestIDL(), 1},
		{"codec v2", codecv2.FetchLogpollerTypeTestIDL(), 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			lggr := logger.Sugared(logger.Test(t))
			orm := mocks.NewMockORM(t)
			fs := newFilters(lggr, orm, nil)
			orm.On("SelectFilters", mock.Anything).Return(nil, nil).Once()
			orm.On("SelectSeqNums", mock.Anything).Return(map[int64]int64{}, nil).Once()
			orm.On("InsertFilter", mock.Anything, mock.Anything).Return(tt.id, nil).Once()

			require.NoError(t, fs.RegisterFilter(t.Context(), types.Filter{
				Name:        tt.name,
				ContractIdl: tt.idl,
				EventName:   "TestItem",
			}))

			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			_, err := fs.DecodeSubKey(t.Context(), lggr, oversizedTestItemEvent(t), tt.id, []string{"Field"})
			runtime.ReadMemStats(&after)

			require.Error(t, err)
			require.Contains(t, err.Error(), "exceeds the maximum")

			allocatedMB := (after.HeapAlloc - before.HeapAlloc) >> 20
			require.Less(t, allocatedMB, uint64(16), "decode allocated %d MB", allocatedMB)
		})
	}
}
