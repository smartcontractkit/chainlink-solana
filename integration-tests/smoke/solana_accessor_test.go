package smoke

import (
	"context"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip/pkg/consts"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/sqlutil/sqltest"
	"github.com/smartcontractkit/chainlink-common/pkg/types/ccipocr3"

	"github.com/smartcontractkit/chainlink-solana/contracts/generated/mock_ccip_events"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/utils"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/ccip/chainaccessor"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller"
)


func Test_SolanaAccessor(t *testing.T) {
	_, _ = createSolanaAccessor(t)


	// EVM to Solana Tests
	t.Run("CommitReportsGTETimestamp", func(t *testing.T) {

	})

	t.Run("ExecutedMessages", func(t *testing.T) {

	})

	t.Run("MessagesByTokenID", func(t *testing.T) {

	})

	// Solana to EVM Tests
	t.Run("MsgsBetweenSeqNums", func(t *testing.T) {

	})

	t.Run("LatestMessageTo", func(t *testing.T) {

	})
}

func createSolanaAccessor(t *testing.T) (*chainaccessor.SolanaAccessor, *client.Client) {
	t.Helper()

	sender, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)

	url, _ := utils.SetupTestValidatorWithAnchorPrograms(t, sender.PublicKey().String(), []string{"mock-ccip-events"})
	rpcClient := rpc.New(url)

	utils.FundAccounts(t, []solana.PrivateKey{sender}, rpcClient)

	lggr := logger.Test(t)

	cfg := config.NewDefault()
	solanaClient, err := client.NewClient(url, cfg, 5*time.Second, lggr)
	require.NoError(t, err)

	multiClient := *client.NewMultiClient(func(context.Context) (client.ReaderWriter, error) {
		return solanaClient, nil
	})

	dbx := sqltest.NewDB(t, sqltest.TestURL(t))
	orm := logpoller.NewORM("", dbx, lggr)
	lp := logpoller.New(logger.Sugared(lggr), orm, solanaClient, cfg) // LP started by chain accessor
	t.Cleanup(func() {
		_ = lp.Close
	})

	estimator, err := fees.NewFixedPriceEstimator(cfg)
	require.NoError(t, err)
	err = estimator.Start(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = estimator.Close
	})

	chainSel := ccipocr3.ChainSelector(rand.Uint64())
	accessor, err :=  chainaccessor.NewSolanaAccessor(t.Context(), lggr, chainSel, multiClient, lp, estimator, nil)
	require.NoError(t ,err)

	setupMockCCIPEventsProgram(t, accessor, solanaClient, sender.PublicKey())

	return accessor, solanaClient
}

func setupMockCCIPEventsProgram(t *testing.T, accessor *chainaccessor.SolanaAccessor, client *client.Client, sender solana.PublicKey) {
	t.Helper()

	mockCCIPEventsProgram := solana.MustPublicKeyFromBase58("CGn5MQX5GK9qKqERhjnADhd6i2LiSF6XUC2ewUHND1Mw")

	// Set mock ccip event program address in accessor to register LP filters for CCIP events
	err := accessor.Sync(t.Context(), consts.ContractNameOnRamp, mockCCIPEventsProgram.Bytes())
	require.NoError(t, err)
	err = accessor.Sync(t.Context(), consts.ContractNameOffRamp, mockCCIPEventsProgram.Bytes())
	require.NoError(t, err)
	err = accessor.Sync(t.Context(), consts.ContractNameUSDCTokenPool, mockCCIPEventsProgram.Bytes())
	require.NoError(t, err)

	mock_ccip_events.SetProgramID(mockCCIPEventsProgram)
	
	sentEvent := buildMockCCIPSentEvent(t)
	commitEvent := buildMockCommitEvent(t)
	executeEvent := buildMockExecuteEvent(t)
	cctpEvent := buildMockCCTPEvent(t)
	
	ix, err := mock_ccip_events.NewInitializeInstruction(sentEvent, commitEvent, executeEvent, cctpEvent).ValidateAndBuild()
	require.NoError(t, err)

	res, err := client.LatestBlockhash(t.Context())
	require.NoError(t, err)
	tx, err := solana.NewTransaction([]solana.Instruction{ix}, res.Value.Blockhash, solana.TransactionPayer(sender))
	require.NoError(t, err)

	sig, err := client.SendTx(t.Context(), tx)
	require.NoError(t, err)

	// Wait for transaction to be finalized since LogPoller only queries finalized events
	require.Eventually(t, func() bool{
		statuses, err := client.SignatureStatuses(t.Context(), []solana.Signature{sig})
		require.NoError(t, err)
		if statuses == nil {
			return false
		}
		status := statuses[0]
		return status.ConfirmationStatus == rpc.ConfirmationStatusFinalized
	}, 1*time.Minute, 1*time.Second)
}

func buildMockCCIPSentEvent(t *testing.T) mock_ccip_events.CCIPMessageSentObj{
	t.Helper()

	return mock_ccip_events.CCIPMessageSentObj{

	}
}

func buildMockCommitEvent(t *testing.T) mock_ccip_events.CommitReportAcceptedObj{
	t.Helper()

	return mock_ccip_events.CommitReportAcceptedObj{
		
	}
}

func buildMockExecuteEvent(t *testing.T) mock_ccip_events.ExecutionStateChangedObj{
	t.Helper()

	return mock_ccip_events.ExecutionStateChangedObj{

	}
}

func buildMockCCTPEvent(t *testing.T) mock_ccip_events.CcipCctpMessageSentEventObj{
	t.Helper()

	return mock_ccip_events.CcipCctpMessageSentEventObj{

	}
}

