package client

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

func captureSlotDelta(ctx context.Context, lggr logger.Logger, clients []*Client, key solana.PublicKey) (uint64, error) {
	requestSignal := make(chan struct{})
	type result struct {
		slot uint64
		err  error
	}
	results := make(chan result, len(clients))
	var wg sync.WaitGroup
	for _, c := range clients {
		wg.Add(1)
		go func(c *Client) {
			defer wg.Done()
			<-requestSignal
			accInfo, err := c.GetAccountInfoWithOpts(ctx, key, &rpc.GetAccountInfoOpts{Commitment: rpc.CommitmentFinalized})
			if err != nil {
				results <- result{err: err}
				return
			}

			results <- result{slot: accInfo.Context.Slot}
		}(c)
	}

	// once all goroutines are ready, send the signal
	close(requestSignal)
	wg.Wait()
	close(results)
	minSlot := uint64(math.MaxUint64)
	maxSlot := uint64(0)
	resultsSlice := make([]uint64, 0, len(clients))
	for r := range results {
		if r.err != nil {
			return 0, fmt.Errorf("error fetching slot: %w", r.err)
		}
		if r.slot < minSlot {
			minSlot = r.slot
		}
		if r.slot > maxSlot {
			maxSlot = r.slot
		}

		resultsSlice = append(resultsSlice, r.slot)
	}

	lggr.Debugw("Slot results", "results", resultsSlice)
	return maxSlot - minSlot, nil
}

func TestPropagationDelay(t *testing.T) {
	lggr := logger.Test(t)
	cfg := config.NewDefault()

	urls := strings.Split(os.Getenv("RPCs"), ",")
	if len(urls) < 2 {
		panic(errors.New("must provide at least two urls"))
	}

	clients := make([]*Client, len(urls))
	for i, u := range urls {
		c, err := NewClient(u, cfg, 5*time.Second, lggr)
		require.NoError(t, err)
		clients[i] = c
	}

	const testDuration = 20 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()
	const testTick = time.Second
	ticker := time.NewTicker(testTick)
	defer ticker.Stop()
	var deltas []uint64
	key, err := solana.PublicKeyFromBase58("SysvarC1ock11111111111111111111111111111111")
	require.NoError(t, err)
	for {
		select {
		case <-ctx.Done():
			lggr.Infow("Propagation Lag Results",
				"samples", len(deltas),
				"p50", mustPercentile(deltas, 50.0),
				"p90", mustPercentile(deltas, 90.0),
				"p95", mustPercentile(deltas, 95.0),
				"p99", mustPercentile(deltas, 99.0),
			)
			return
		case <-ticker.C:
			delta, err := captureSlotDelta(ctx, lggr, clients, key)
			if err != nil {
				lggr.Errorf("error capturing slot delta: %s", err)
				continue
			}
			lggr.Infof("captured slot delta: %d", delta)
			deltas = append(deltas, delta)
		}
	}
}

func mustPercentile(input []uint64, percentile float64) float64 {
	result, err := CalculatePercentile(input, percentile)
	if err != nil {
		panic(err)
	}
	return result
}

// CalculatePercentile computes the linear interpolation of a percentile.
// input: slice of uint64, percentile (0.0 to 100.0)
// output: float64 (to maintain precision for interpolation), error
func CalculatePercentile(data []uint64, percentile float64) (float64, error) {
	if len(data) == 0 {
		return 0, errors.New("data slice is empty")
	}
	if percentile < 0 || percentile > 100 {
		return 0, errors.New("percentile must be between 0 and 100")
	}

	// Using sort.Slice for compatibility (or slices.Sort in Go 1.21+)
	sort.Slice(data, func(i, j int) bool { return data[i] < data[j] })

	// 2. Handle edge cases (0th and 100th percentiles)
	if percentile == 0.0 {
		return float64(data[0]), nil
	}
	if percentile == 100.0 {
		return float64(data[len(data)-1]), nil
	}

	// 3. Calculate the index
	// We use (N-1) because indices are 0-based.
	// Example: P50 of 3 items is index 1.0.
	index := (float64(len(data)) - 1) * (percentile / 100.0)

	// 4. Linear Interpolation
	lowerIndex := int(math.Floor(index))
	upperIndex := int(math.Ceil(index))

	if lowerIndex == upperIndex {
		return float64(data[lowerIndex]), nil
	}

	// Get the values at the lower and upper indices
	lowerValue := float64(data[lowerIndex])
	upperValue := float64(data[upperIndex])

	// The fractional part determines the weight
	fraction := index - float64(lowerIndex)

	// Formula: lower + (diff * fraction)
	return lowerValue + (upperValue-lowerValue)*fraction, nil
}
