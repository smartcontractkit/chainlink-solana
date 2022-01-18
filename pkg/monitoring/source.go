package monitoring

import (
	"context"
	"math/big"
	"time"

	"github.com/smartcontractkit/libocr/offchainreporting2/types"
)

type Source interface {
	Fetch(context.Context) (interface{}, error)
}

type Sources interface {
	NewTransmissionsSource() Source
	NewConfigSource() Source
}

type SourceFactory interface {
	NewSources(chainConfig SolanaConfig, feedConfig FeedConfig) (Sources, error)
}

type TransmissionEnvelope struct {
	ConfigDigest    types.ConfigDigest
	Epoch           uint32
	Round           uint8
	LatestAnswer    *big.Int
	LatestTimestamp time.Time
}

type ConfigEnvelope struct {
	types.ContractConfig
}
