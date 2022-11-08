package solana

import (
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/libocr/offchainreporting2/reportingplugin/median"
)

// go test -fuzz ./...
func FuzzReportCodecMedianFromReport(f *testing.F) {
	cdc := ReportCodec{}
	report, err := cdc.BuildReport([]median.ParsedAttributedObservation{
		{Timestamp: uint32(time.Now().Unix()), Value: big.NewInt(10), JuelsPerFeeCoin: big.NewInt(100000)},
		{Timestamp: uint32(time.Now().Unix()), Value: big.NewInt(10), JuelsPerFeeCoin: big.NewInt(200000)},
		{Timestamp: uint32(time.Now().Unix()), Value: big.NewInt(11), JuelsPerFeeCoin: big.NewInt(300000)}})
	require.NoError(f, err)

	// Seed with valid report
	f.Add([]byte(report))
	f.Fuzz(func(t *testing.T, report []byte) {
		med, err := cdc.MedianFromReport(report)
		if err == nil {
			// Should always be able to build a report from the medians extracted
			// Note however that juelsPerFeeCoin is only 8 bytes, so we can use the median for it
			_, err = cdc.BuildReport([]median.ParsedAttributedObservation{{Timestamp: uint32(time.Now().Unix()), Value: med, JuelsPerFeeCoin: big.NewInt(100000)}})
			require.NoError(t, err)
		}
	})
}
