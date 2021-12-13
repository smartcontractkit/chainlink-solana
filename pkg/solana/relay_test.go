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
// 		StateID:         solana.MustPublicKeyFromBase58("FYN2Exo7xNPS3i7WXM97QmeRj39z9qRqMYGSc31AtNG8"),
// 		TransmissionsID: solana.MustPublicKeyFromBase58("365VmdRjJGtAhwmPnw6LTSvctVoTNa7boAYCQFsFQcVr"),
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
//
// 	fmt.Println("LatestTransmissionDetails", digest, epoch, round, answer, timestamp)
// 	fmt.Println("OffchainConfig", tracker.state.Config.OffchainConfig.Data())
// 	fmt.Println("ValidatorConfig", tracker.state.Config.Validator, tracker.state.Config.FlaggingThreshold)
// 	fmt.Println("AccessControllers", tracker.state.Config.RequesterAccessController, tracker.state.Config.BillingAccessController)
// 	fmt.Println("BillingConfig", tracker.state.Config.Billing.ObservationPayment, tracker.state.Config.Billing.TransmissionPayment)
// 	fmt.Printf("OracleConfigs %+v\n", tracker.state.Oracles.Data())
// 	fmt.Printf("Transmissions %+v\n", tracker.answer)
// }
