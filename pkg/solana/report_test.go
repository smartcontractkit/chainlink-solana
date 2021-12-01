package solana

import (
	"encoding/binary"
	"math/big"
	"testing"
	"time"

	"github.com/smartcontractkit/libocr/commontypes"
	"github.com/smartcontractkit/libocr/offchainreporting2/reportingplugin/median"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"
	"github.com/stretchr/testify/assert"
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

	// validate observers
	assert.Equal(t, observers, []byte(report[4:4+32]), "validate observers")

	// validate median observation
	index := 4 + 32
	assert.Equal(t, oo[0].Value.FillBytes(make([]byte, 16)), []byte(report[index:index+16]), "validate median observation")

	// validate juelsToEth
	assert.Equal(t, v.FillBytes(make([]byte, 16)), []byte(report[ReportLen-16:ReportLen]), "validate juelsToEth")
}

func TestMedianFromReport(t *testing.T) {
	c := ReportCodec{}

	report := types.Report{
		97, 91, 43, 83, // observations_timestamp
		0, 1, 2, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // observers
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 73, 150, 2, 210, // observation 2
		0, 0, 0, 0, 0, 0, 0, 0, 13, 224, 182, 179, 167, 100, 0, 0, // juels per luna (1 with 18 decimal places)
	}

	res, err := c.MedianFromReport(report)
	assert.NoError(t, err)
	assert.Equal(t, "1234567890", res.String())
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
		0, 1, 2, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // observers
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 210, // median
		0, 0, 0, 0, 0, 0, 0, 0, 13, 224, 182, 179, 167, 100, 0, 0, // juels per sol (1 with 18 decimal places)
	}

	var mockHash = []byte{
		124, 158, 204, 40, 181, 54, 124,
		38, 196, 146, 13, 14, 178, 47,
		254, 150, 111, 21, 42, 181, 191,
		132, 111, 236, 216, 151, 233, 110,
		86, 216, 154, 169,
	}

	h, err := HashReport(mockReportCtx, mockReport)
	assert.NoError(t, err)
	assert.Equal(t, mockHash, h)
}
