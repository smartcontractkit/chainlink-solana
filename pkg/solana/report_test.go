package solana

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/big"
	"testing"
	"time"

	bin "github.com/gagliardetto/binary"
	"github.com/smartcontractkit/libocr/commontypes"
	"github.com/smartcontractkit/libocr/offchainreporting2/reportingplugin/median"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/utils"
)

func TestBuildReport(t *testing.T) {
	c := ReportCodec{}
	oo := []median.ParsedAttributedObservation{}

	// expected outputs
	n := 4
	observers := make([]byte, 32)
	v := big.NewInt(0)
	v.SetString("1000000000000000000", 10)

	for i := 0; i < n; i++ {
		oo = append(oo, median.ParsedAttributedObservation{
			Timestamp:       uint32(time.Now().Unix()),
			Value:           big.NewInt(1234567890),
			JuelsPerFeeCoin: v,
			Observer:        commontypes.OracleID(i),
		})

		// create expected outputs
		observers[i] = uint8(i)
	}

	report, err := c.BuildReport(oo)
	assert.NoError(t, err)

	// validate length
	assert.Equal(t, int(ReportLen), len(report), "validate length")

	// validate timestamp
	assert.Equal(t, oo[0].Timestamp, binary.BigEndian.Uint32(report[0:4]), "validate timestamp")

	// validate observer count
	assert.Equal(t, uint8(n), report[4], "validate observer count")

	// validate observers
	index := 4 + 1
	assert.Equal(t, observers, []byte(report[index:index+32]), "validate observers")

	// validate median observation
	index = 4 + 1 + 32
	assert.Equal(t, oo[0].Value.FillBytes(make([]byte, 16)), []byte(report[index:index+16]), "validate median observation")

	// validate juelsToEth
	assert.Equal(t, v.FillBytes(make([]byte, 8)), []byte(report[ReportLen-8:ReportLen]), "validate juelsToEth")
}

func TestMedianFromOnChainReport(t *testing.T) {
	c := ReportCodec{}

	report := types.Report{
		97, 91, 43, 83, // observations_timestamp
		2,                                                                                              // observer_count
		0, 1, 2, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // observers
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 73, 150, 2, 210, // observation 2
		13, 224, 182, 179, 167, 100, 0, 0, // juels per luna (1 with 18 decimal places)
	}

	res, err := c.MedianFromReport(report)
	assert.NoError(t, err)
	assert.Equal(t, "1234567890", res.String())
}

type medianTest struct {
	name           string
	obs            []*big.Int
	expectedMedian *big.Int
}

func TestMedianFromReport(t *testing.T) {
	cdc := ReportCodec{}
	// Requires at least one obs
	_, err := cdc.BuildReport(nil)
	require.Error(t, err)
	var tt = []medianTest{
		{
			name:           "2 positive one zero",
			obs:            []*big.Int{big.NewInt(0), big.NewInt(10), big.NewInt(20)},
			expectedMedian: big.NewInt(10),
		},
		{
			name:           "one zero",
			obs:            []*big.Int{big.NewInt(0)},
			expectedMedian: big.NewInt(0),
		},
		{
			name:           "two equal",
			obs:            []*big.Int{big.NewInt(1), big.NewInt(1)},
			expectedMedian: big.NewInt(1),
		},
		{
			name: "one negative one positive",
			obs:  []*big.Int{big.NewInt(-1), big.NewInt(1)},
			// sorts to -1, 1
			expectedMedian: big.NewInt(1),
		},
		{
			name: "two negative",
			obs:  []*big.Int{big.NewInt(-2), big.NewInt(-1)},
			// will sort to -2, -1
			expectedMedian: big.NewInt(-1),
		},
		{
			name: "three negative",
			obs:  []*big.Int{big.NewInt(-5), big.NewInt(-3), big.NewInt(-1)},
			// will sort to -5, -3, -1
			expectedMedian: big.NewInt(-3),
		},
	}

	// add cases for observation number from [1..31]
	for i := 1; i < 32; i++ {
		test := medianTest{
			name:           fmt.Sprintf("observations=%d", i),
			obs:            []*big.Int{},
			expectedMedian: big.NewInt(1),
		}
		for j := 0; j < i; j++ {
			test.obs = append(test.obs, big.NewInt(1))
		}
		tt = append(tt, test)
	}

	for _, tc := range tt {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var pos []median.ParsedAttributedObservation
			for i, obs := range tc.obs {
				pos = append(pos, median.ParsedAttributedObservation{
					Value:           obs,
					JuelsPerFeeCoin: obs,
					Observer:        commontypes.OracleID(uint8(i))},
				)
			}
			report, err := cdc.BuildReport(pos)
			require.NoError(t, err)
			max, err := cdc.MaxReportLength(len(tc.obs))
			require.NoError(t, err)
			assert.Equal(t, len(report), max)
			med, err := cdc.MedianFromReport(report)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedMedian.String(), med.String())
			count, err := cdc.ObserversCountFromReport(report)
			require.NoError(t, err)
			assert.Equal(t, len(tc.obs), int(count))
		})
	}

}

func TestHashReport(t *testing.T) {
	var mockDigest = [32]byte{
		0, 3, 94, 221, 213, 66, 228, 80, 239, 231, 7, 96,
		83, 156, 95, 165, 199, 168, 222, 107, 47, 238, 157, 46,
		65, 205, 71, 121, 195, 138, 77, 137,
	}
	var mockReportCtx = types.ReportContext{
		ReportTimestamp: types.ReportTimestamp{
			ConfigDigest: mockDigest,
			Epoch:        1,
			Round:        1,
		},
		ExtraHash: [32]byte{},
	}

	var mockReport = types.Report{
		97, 91, 43, 83, // observations_timestamp
		2,                                                                                              // observer_count
		0, 1, 2, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // observers
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 210, // median
		13, 224, 182, 179, 167, 100, 0, 0, // juels per sol (1 with 18 decimal places)
	}

	var mockHash = []byte{
		0x9a, 0xe0, 0xc3, 0x7d, 0x9d, 0x45, 0x58, 0xdc,
		0x1e, 0x8b, 0xbc, 0xf4, 0x7d, 0x6b, 0xc8, 0xb0,
		0x5, 0xbe, 0xbe, 0x5f, 0xd, 0x28, 0x33, 0x3b,
		0x27, 0x11, 0x33, 0x5f, 0xed, 0x43, 0x91, 0x60,
	}

	h, err := utils.HashReport(mockReportCtx, mockReport)
	assert.NoError(t, err)
	assert.Equal(t, mockHash, h)
}

func TestNegativeMedianValue(t *testing.T) {
	c := ReportCodec{}
	oo := []median.ParsedAttributedObservation{
		median.ParsedAttributedObservation{
			Timestamp:       uint32(time.Now().Unix()),
			Value:           big.NewInt(-2),
			JuelsPerFeeCoin: big.NewInt(1),
			Observer:        commontypes.OracleID(0),
		},
	}

	// create report
	report, err := c.BuildReport(oo)
	assert.NoError(t, err)

	// check report properly encoded negative number
	index := 4 + 1 + 32
	var medianFromRaw bin.Int128
	medianBytes := make([]byte, MedianLen)
	copy(medianBytes, report[index:index+int(MedianLen)])
	// flip order: bin decoder parses from little endian
	for i, j := 0, len(medianBytes)-1; i < j; i, j = i+1, j-1 {
		medianBytes[i], medianBytes[j] = medianBytes[j], medianBytes[i]
	}
	bin.NewBinDecoder(medianBytes).Decode(&medianFromRaw)
	assert.True(t, oo[0].Value.Cmp(medianFromRaw.BigInt()) == 0, "median observation in raw report does not match")

	// check report can be parsed properly with a negative number
	res, err := c.MedianFromReport(report)
	assert.NoError(t, err)
	assert.True(t, oo[0].Value.Cmp(res) == 0)
}

func TestReportHandleOverflow(t *testing.T) {
	// too large observation should not cause panic
	c := ReportCodec{}
	oo := []median.ParsedAttributedObservation{
		median.ParsedAttributedObservation{
			Timestamp:       uint32(time.Now().Unix()),
			Value:           big.NewInt(0).Lsh(big.NewInt(1), 127), // 1<<127
			JuelsPerFeeCoin: big.NewInt(0),
			Observer:        commontypes.OracleID(0),
		},
	}
	_, err := c.BuildReport(oo)
	assert.Error(t, err)

	// too large juelsPerFeeCoin should not cause panic
	oo = []median.ParsedAttributedObservation{
		median.ParsedAttributedObservation{
			Timestamp:       uint32(time.Now().Unix()),
			Value:           big.NewInt(0),
			JuelsPerFeeCoin: big.NewInt(0).Add(big.NewInt(math.MaxInt64), big.NewInt(1)),
			Observer:        commontypes.OracleID(0),
		},
	}
	_, err = c.BuildReport(oo)
	assert.Error(t, err)
}
