package logpoller

import (
	"bytes"
	"encoding/binary"
	"fmt"
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
// vec of publicKey, claims math.MaxInt32 elements. Without length validation,
// the 32 byte element size turns those four bytes into a 64 GiB allocation.
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

var oversizedFieldIDLs = []struct {
	name string
	idl  string
}{
	{
		"codec v1",
		`{"version":"0.1.0","name":"test","events":[{"name":"TestItem","fields":[{"name":"field","type":%s,"index":false}]}]}`,
	},
	{
		"codec v2",
		`{
			"address":"ECL8142j2YQAvs9R9geSsRnkVH2wLEi7soJCRyJ74cfL",
			"metadata":{"name":"test","version":"0.1.0","spec":"0.1.0"},
			"instructions":[],
			"events":[{"name":"TestItem","discriminator":[119,183,160,247,84,104,222,251]}],
			"types":[{"name":"TestItem","type":{"kind":"struct","fields":[{"name":"field","type":%s}]}}]
		}`,
	},
}

func TestFilters_DecodeSubKey_RejectsOversizedString(t *testing.T) {
	for _, tt := range oversizedFieldIDLs {
		t.Run(tt.name, func(t *testing.T) {
			lggr := logger.Sugared(logger.Test(t))
			orm := mocks.NewMockORM(t)
			fs := newFilters(lggr, orm, nil)
			orm.On("SelectFilters", mock.Anything).Return(nil, nil).Once()
			orm.On("SelectSeqNums", mock.Anything).Return(map[int64]int64{}, nil).Once()
			orm.On("InsertFilter", mock.Anything, mock.Anything).Return(int64(1), nil).Once()

			require.NoError(t, fs.RegisterFilter(t.Context(), types.Filter{
				Name:        tt.name,
				ContractIdl: fmt.Sprintf(tt.idl, `"string"`),
				EventName:   "TestItem",
			}))

			prefix := solcommoncodec.NewDiscriminatorHashPrefix("TestItem", false)
			raw := binary.LittleEndian.AppendUint32(prefix, solcommoncodec.MaxAccountBytes+1)
			_, err := fs.DecodeSubKey(t.Context(), lggr, raw, 1, []string{"Field"})
			require.ErrorContains(t, err, fmt.Sprintf("element count %d exceeds the maximum", solcommoncodec.MaxAccountBytes+1))

			raw = append(prefix, 3, 0, 0, 0, 'f', 'o', 'o')
			value, err := fs.DecodeSubKey(t.Context(), lggr, raw, 1, []string{"Field"})
			require.NoError(t, err)
			require.Equal(t, "foo", value)
		})
	}
}

func TestFilters_RegisterFilter_RejectsOversizedArray(t *testing.T) {
	for _, tt := range oversizedFieldIDLs {
		t.Run(tt.name, func(t *testing.T) {
			lggr := logger.Sugared(logger.Test(t))
			orm := mocks.NewMockORM(t)
			fs := newFilters(lggr, orm, nil)
			orm.On("SelectFilters", mock.Anything).Return(nil, nil).Once()
			orm.On("SelectSeqNums", mock.Anything).Return(map[int64]int64{}, nil).Once()

			fieldType := fmt.Sprintf(`{"array":["u64",%d]}`, solcommoncodec.MaxAccountBytes/8+1)
			err := fs.RegisterFilter(t.Context(), types.Filter{
				Name:        tt.name,
				ContractIdl: fmt.Sprintf(tt.idl, fieldType),
				EventName:   "TestItem",
			})
			require.ErrorContains(t, err, "exceeds the maximum")
			require.ErrorContains(t, err, "for solana data")
		})
	}
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

			_, err := fs.DecodeSubKey(t.Context(), lggr, oversizedTestItemEvent(t), tt.id, []string{"Field"})

			// The account bound rejects the length prefix before the slice
			// decoder can allocate any element storage.
			require.Error(t, err)
			require.Contains(t, err.Error(), "exceeds the maximum")
			require.NotContains(t, err.Error(), "not enough bytes")
		})
	}
}
