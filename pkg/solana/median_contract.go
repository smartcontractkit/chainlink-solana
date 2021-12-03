package solana

import (
	"context"
	"math/big"
	"time"

	"github.com/smartcontractkit/libocr/offchainreporting2/types"
)

func (c ContractTracker) LatestTransmissionDetails(
	ctx context.Context,
) (
	configDigest types.ConfigDigest,
	epoch uint32,
	round uint8,
	latestAnswer *big.Int,
	latestTimestamp time.Time,
	err error,
) {
	if err := fetchWrap(ctx, c.fetchState, &c.lockStateChan); err != nil {
		return types.ConfigDigest{}, 0, 0, big.NewInt(0), time.Unix(0, 0), err
	}

	if err := fetchWrap(ctx, c.fetchTransmissions, &c.lockTransmissionsChan); err != nil {
		return types.ConfigDigest{}, 0, 0, big.NewInt(0), time.Unix(0, 0), err
	}

	configDigest = c.state.Config.LatestConfigDigest
	epoch = c.state.Config.Epoch
	round = c.state.Config.Round
	latestAnswer = c.answer.Answer
	latestTimestamp = time.Unix(int64(c.answer.Timestamp), 0)
	return configDigest, epoch, round, latestAnswer, latestTimestamp, nil
}

// LatestRoundRequested returns the configDigest, epoch, and round from the latest
// RoundRequested event emitted by the contract. LatestRoundRequested may or may not
// return a result if the latest such event was emitted in a block b such that
// b.timestamp < tip.timestamp - lookback.
//
// If no event is found, LatestRoundRequested should return zero values, not an error.
// An error should only be returned if an actual error occurred during execution,
// e.g. because there was an error querying the blockchain or the database.
//
// As an optimization, this function may also return zero values, if no
// RoundRequested event has been emitted after the latest NewTransmission event.
func (c ContractTracker) LatestRoundRequested(
	ctx context.Context,
	lookback time.Duration,
) (
	configDigest types.ConfigDigest,
	epoch uint32,
	round uint8,
	err error,
) {
	return c.state.Config.LatestConfigDigest, 0, 0, nil
}
