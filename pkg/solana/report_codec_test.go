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
	c := ContractTracker{}
	paos := []median.ParsedAttributedObservation{}

	// expected outputs
	n := 4
	observers := make([]byte, 32)
	v := big.NewInt(0)
	v.SetString("1000000000000000000", 10)

	for i := 0; i < n; i++ {
		paos = append(paos, median.ParsedAttributedObservation{
			Timestamp:       uint32(time.Now().Unix()),
			Value:           big.NewInt(1234567890),
			JuelsPerFeeCoin: v,
			Observer:        commontypes.OracleID(i),
		})

		// create expected outputs
		observers[i] = uint8(i)
	}

	report, err := c.BuildReport(paos)
	assert.NoError(t, err)

	// validate length
	totalLen := 4 + 32 + 16 + 16
	assert.Equal(t, totalLen, len(report), "validate length")

	// validate timestamp
	assert.Equal(t, paos[0].Timestamp, binary.BigEndian.Uint32(report[0:4]), "validate timestamp")

	// validate observers
	assert.Equal(t, observers, []byte(report[4:4+32]), "validate observers")

	// validate median observation
	index := 4 + 32
	assert.Equal(t, paos[0].Value.FillBytes(make([]byte, 16)), []byte(report[index:index+16]), "validate median observation")

	// validate juelsToEth
	assert.Equal(t, v.FillBytes(make([]byte, 16)), []byte(report[totalLen-16:totalLen]), "validate juelsToEth")
}

func TestMedianFromReport(t *testing.T) {
	c := ContractTracker{}

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
