package solana

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"sort"

	"github.com/smartcontractkit/libocr/offchainreporting2/reportingplugin/median"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"
)

func (c ContractTracker) BuildReport(paos []median.ParsedAttributedObservation) (types.Report, error) {
	if len(paos) == 0 {
		return nil, fmt.Errorf("cannot build report from empty attributed observations")
	}

	// copy so we can safely re-order subsequently
	paos = append([]median.ParsedAttributedObservation{}, paos...)

	// get median timestamp
	sort.Slice(paos, func(i, j int) bool {
		return paos[i].Timestamp < paos[j].Timestamp
	})
	timestamp := paos[len(paos)/2].Timestamp

	// get median juelsPerFeeCoin
	sort.Slice(paos, func(i, j int) bool {
		return paos[i].JuelsPerFeeCoin.Cmp(paos[j].JuelsPerFeeCoin) < 0
	})
	juelsPerFeeCoin := paos[len(paos)/2].JuelsPerFeeCoin

	// get median by value
	// solana program size tx execution limit prevents reporting all observations
	// reporting only median value
	sort.Slice(paos, func(i, j int) bool {
		return paos[i].Value.Cmp(paos[j].Value) < 0
	})
	median := paos[len(paos)/2].Value

	observers := [32]byte{}
	for i, pao := range paos {
		observers[i] = byte(pao.Observer)
	}

	// report encoding
	reportBytes := []byte{}

	timeBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(timeBytes, timestamp)
	reportBytes = append(reportBytes, timeBytes[:]...)

	reportBytes = append(reportBytes, observers[:]...)

	medianBytes := make([]byte, 16)
	reportBytes = append(reportBytes, median.FillBytes(medianBytes)[:]...)

	jBytes := make([]byte, 16)
	reportBytes = append(reportBytes, juelsPerFeeCoin.FillBytes(jBytes)[:]...)

	return types.Report(reportBytes), nil
}

func (c ContractTracker) MedianFromReport(report types.Report) (*big.Int, error) {
	// report should contain timestamp + observers + median + juels per eth
	rLen := len(report)
	if rLen != 4+32+16+16 {
		return nil, fmt.Errorf("report length is too short: %d (received), 68 (expected)", rLen)
	}

	// unpack median observation
	index := 4 + 32
	median := big.NewInt(0)
	median.SetBytes(report[index : index+16])

	return median, nil
}
