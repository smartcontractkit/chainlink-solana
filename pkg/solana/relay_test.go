package solana

// import (
// 	"context"
// 	"fmt"
// 	"testing"
//
// 	"github.com/gagliardetto/solana-go"
// 	"github.com/gagliardetto/solana-go/rpc"
// 	"github.com/gagliardetto/solana-go/rpc/ws"
// 	"github.com/smartcontractkit/chainlink/core/logger"
// 	"github.com/stretchr/testify/assert"
// 	"github.com/stretchr/testify/require"
// 	"golang.org/x/sync/singleflight"
// )
//
// func Test_DeployedContract(t *testing.T) {
// 	tracker := ContractTracker{
// 		StateID:         solana.MustPublicKeyFromBase58("EBMqEKQAY6FERiUaxftESoy7nKXgkjW9bt4czhsrbdcm"),
// 		TransmissionsID: solana.MustPublicKeyFromBase58("9rCD23Ug3G5eoyJChGcPDJXugdZz4ptmvYMm2YFiTbr4"),
// 		client:          NewClient(rpc.LocalNet_RPC, &ws.Client{}),
// 		lggr:            logger.NullLogger,
// 		requestGroup:    &singleflight.Group{},
// 	}
//
// 	digester := OffchainConfigDigester{
// 		ProgramID: solana.MustPublicKeyFromBase58("CF6b2XF6BZw65aznGzXwzF5A8iGhDBoeNYQiXyH4MWdQ"),
// 	}
//
// 	cfg, err := tracker.LatestConfig(context.TODO(), 0)
// 	require.NoError(t, err)
//
// 	digest, err := digester.ConfigDigest(cfg)
// 	require.NoError(t, err)
// 	assert.Equal(t, cfg.ConfigDigest, digest)
//
// 	digest, epoch, round, answer, timestamp, err := tracker.LatestTransmissionDetails(context.TODO())
// 	require.NoError(t, err)
// 	fmt.Println(digest, epoch, round, answer, timestamp)
// }
