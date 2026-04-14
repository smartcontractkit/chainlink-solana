package tests

import (
	"context"
	"encoding/binary"
	"math/big"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	chainsel "github.com/smartcontractkit/chain-selectors"

	"github.com/smartcontractkit/chainlink-common/pkg/types/ccip/consts"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/sqlutil/sqltest"
	"github.com/smartcontractkit/chainlink-common/pkg/types/ccipocr3"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query/primitives"

	"github.com/smartcontractkit/chainlink-solana/contracts/generated/mock_ccip_events"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/utils"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/ccip/chainaccessor"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller"
)

func Test_SolanaAccessor(t *testing.T) {
	ts := time.Now()
	mockCCIPEventsProgram := solana.MustPublicKeyFromBase58("CGn5MQX5GK9qKqERhjnADhd6i2LiSF6XUC2ewUHND1Mw")
	sender := utils.GetRandomPubKey(t)
	receiver := utils.GetRandomPubKey(t)
	feeToken := utils.GetRandomPubKey(t)
	accessor := createSolanaAccessor(t, mockCCIPEventsProgram, sender, receiver, feeToken)

	// EVM to Solana Tests
	t.Run("CommitReportsGTETimestamp", func(t *testing.T) {
		reports, err := accessor.CommitReportsGTETimestamp(t.Context(), ts, primitives.Finalized, 10)
		require.NoError(t, err)
		require.Len(t, reports, 1)
		report := reports[0]

		require.Len(t, report.Report.BlessedMerkleRoots, 0)
		require.Len(t, report.Report.RMNSignatures, 0)
		require.Len(t, report.Report.PriceUpdates.GasPriceUpdates, 0)
		require.Len(t, report.Report.PriceUpdates.TokenPriceUpdates, 0)

		// Validate merkle root
		require.Len(t, report.Report.UnblessedMerkleRoots, 1)
		merkleRoot := report.Report.UnblessedMerkleRoots[0]
		require.Equal(t, ccipocr3.ChainSelector(chainsel.TEST_33333333333333333333333333333333333333333333.Selector), merkleRoot.ChainSel)
		require.Equal(t, ccipocr3.Bytes32{1}, merkleRoot.MerkleRoot)
		require.Equal(t, ccipocr3.UnknownAddress(mockCCIPEventsProgram.Bytes()), merkleRoot.OnRampAddress)
		require.Equal(t, ccipocr3.SeqNumRange{1, 1}, merkleRoot.SeqNumsRange)
	})

	t.Run("ExecutedMessages", func(t *testing.T) {
		ranges := make(map[ccipocr3.ChainSelector][]ccipocr3.SeqNumRange)
		ranges[ccipocr3.ChainSelector(chainsel.TEST_33333333333333333333333333333333333333333333.Selector)] = []ccipocr3.SeqNumRange{{1, 1}}
		msgsMap, err := accessor.ExecutedMessages(t.Context(), ranges, primitives.Finalized)
		require.NoError(t, err)

		require.Len(t, msgsMap, 1)
		require.Contains(t, msgsMap, ccipocr3.ChainSelector(chainsel.TEST_33333333333333333333333333333333333333333333.Selector))
		seqNums := msgsMap[ccipocr3.ChainSelector(chainsel.TEST_33333333333333333333333333333333333333333333.Selector)]
		require.Len(t, seqNums, 1)
		seqNum := seqNums[0]
		require.Equal(t, ccipocr3.SeqNum(1), seqNum)
	})

	// Solana to EVM Tests
	t.Run("MessagesByTokenID", func(t *testing.T) {
		srcSelector := ccipocr3.ChainSelector(chainsel.TEST_22222222222222222222222222222222222222222222.Selector)
		destSelector := ccipocr3.ChainSelector(chainsel.TEST_33333333333333333333333333333333333333333333.Selector)

		tokensMap := make(map[ccipocr3.MessageTokenID]ccipocr3.RampTokenAmount)
		messageTokenID := ccipocr3.NewMessageTokenID(1, 0)
		extraData := make([]byte, 64)
		binary.BigEndian.PutUint64(extraData[24:], 1) // nonce
		binary.BigEndian.PutUint32(extraData[60:], 0) // source domain
		tokensMap[messageTokenID] = ccipocr3.RampTokenAmount{ExtraData: ccipocr3.Bytes(extraData)}
		results, err := accessor.MessagesByTokenID(t.Context(), srcSelector, destSelector, tokensMap)
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.Contains(t, results, messageTokenID)

		bytes := results[messageTokenID]
		require.Equal(t, ccipocr3.Bytes("test message sent bytes"), bytes)
	})

	t.Run("MsgsBetweenSeqNums", func(t *testing.T) {
		srcSelector := ccipocr3.ChainSelector(chainsel.TEST_22222222222222222222222222222222222222222222.Selector)
		destSelector := ccipocr3.ChainSelector(chainsel.TEST_33333333333333333333333333333333333333333333.Selector)
		msgs, err := accessor.MsgsBetweenSeqNums(t.Context(), destSelector, ccipocr3.SeqNumRange{1, 1})
		require.NoError(t, err)
		require.Len(t, msgs, 1)

		msg := msgs[0]
		require.Equal(t, ccipocr3.Bytes("ccip message sent data"), msg.Data)
		require.Nil(t, msg.ExtraArgs)
		require.Equal(t, ccipocr3.UnknownAddress(feeToken.Bytes()), msg.FeeToken)
		require.Equal(t, ccipocr3.NewBigInt(big.NewInt(0)), msg.FeeTokenAmount)
		require.Equal(t, ccipocr3.NewBigInt(big.NewInt(0)), msg.FeeValueJuels)
		require.Equal(t, ccipocr3.UnknownAddress(sender.Bytes()), msg.Sender)
		require.Equal(t, ccipocr3.UnknownAddress(receiver.Bytes()), msg.Receiver)
		require.Equal(t, []ccipocr3.RampTokenAmount{}, msg.TokenAmounts)

		require.Equal(t, srcSelector, msg.Header.SourceChainSelector)
		require.Equal(t, destSelector, msg.Header.DestChainSelector)
		require.Equal(t, ccipocr3.SeqNum(1), msg.Header.SequenceNumber)
		require.Equal(t, uint64(0), msg.Header.Nonce)
		require.Equal(t, ccipocr3.Bytes32{1}, msg.Header.MessageID)
		require.Equal(t, "", msg.Header.TxHash)
		require.Equal(t, ccipocr3.UnknownAddress(mockCCIPEventsProgram.Bytes()), msg.Header.OnRamp)
		require.Equal(t, ccipocr3.Bytes32{}, msg.Header.MsgHash)
	})

	t.Run("LatestMessageTo", func(t *testing.T) {
		destSelector := ccipocr3.ChainSelector(chainsel.TEST_33333333333333333333333333333333333333333333.Selector)
		seqNum, err := accessor.LatestMessageTo(t.Context(), destSelector)
		require.NoError(t, err)
		require.Equal(t, ccipocr3.SeqNum(1), seqNum)
	})
}

func createSolanaAccessor(t *testing.T, mockProgram, sender, receiver, feeToken solana.PublicKey) *chainaccessor.SolanaAccessor {
	t.Helper()

	authority, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)

	url, _ := utils.SetupTestValidatorWithAnchorPrograms(t, authority.PublicKey().String(), []string{"mock-ccip-events"})
	rpcClient := rpc.New(url)

	utils.FundAccounts(t, []solana.PrivateKey{authority}, rpcClient)

	lggr := logger.Test(t)

	cfg := config.NewDefault()
	cpiEnabled := false
	cfg.Chain.LogPollerCPIEventsEnabled = &cpiEnabled
	solanaClient, err := client.NewClient(url, cfg, 5*time.Second, lggr)
	require.NoError(t, err)

	multiClient := *client.NewMultiClient(func(context.Context) (client.ReaderWriter, error) {
		return solanaClient, nil
	})

	dbx := sqltest.NewDB(t, sqltest.TestURL(t))
	orm := logpoller.NewORM(chainsel.TEST_22222222222222222222222222222222222222222222.ChainID, dbx, lggr)
	lp, err := logpoller.New(logger.Sugared(lggr), orm, solanaClient, cfg, "test-chain-id") // LP started by chain accessor
	require.NoError(t, err)

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

	chainSel := ccipocr3.ChainSelector(chainsel.TEST_22222222222222222222222222222222222222222222.Selector)
	accessor, err := chainaccessor.NewSolanaAccessor(t.Context(), lggr, chainSel, multiClient, lp, estimator, nil)
	require.NoError(t, err)

	setupMockCCIPEventsProgram(t, accessor, solanaClient, authority, cfg, mockProgram, sender, receiver, feeToken)

	return accessor
}

func setupMockCCIPEventsProgram(t *testing.T, accessor *chainaccessor.SolanaAccessor, client *client.Client, authority solana.PrivateKey, cfg *config.TOMLConfig, mockCCIPEventsProgram, sender, receiver, feeToken solana.PublicKey) {
	t.Helper()

	// Set mock ccip event program address in accessor to register LP filters for CCIP events
	err := accessor.Sync(t.Context(), consts.ContractNameOnRamp, mockCCIPEventsProgram.Bytes())
	require.NoError(t, err)
	err = accessor.Sync(t.Context(), consts.ContractNameOffRamp, mockCCIPEventsProgram.Bytes())
	require.NoError(t, err)
	err = accessor.Sync(t.Context(), consts.ContractNameUSDCTokenPool, mockCCIPEventsProgram.Bytes())
	require.NoError(t, err)

	// Wait one LogPoller loop to ensure filters are registered
	time.Sleep(2 * cfg.BlockTime())

	mock_ccip_events.SetProgramID(mockCCIPEventsProgram)

	sentEvent := buildMockCCIPSentEvent(t, 1, sender, receiver, feeToken)
	commitEvent := buildMockCommitEvent(t, mockCCIPEventsProgram)
	executeEvent := buildMockExecuteEvent(t)
	cctpEvent := buildMockCCTPEvent(t)

	ix, err := mock_ccip_events.NewInitializeInstruction(sentEvent, commitEvent, executeEvent, cctpEvent).ValidateAndBuild()
	require.NoError(t, err)

	res, err := client.LatestBlockhash(t.Context())
	require.NoError(t, err)
	tx, err := solana.NewTransaction([]solana.Instruction{ix}, res.Value.Blockhash, solana.TransactionPayer(authority.PublicKey()))
	require.NoError(t, err)

	sigs, err := tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(authority.PublicKey()) {
			return &authority
		}
		t.Error("unexpected signer key")
		return nil
	})
	require.NoError(t, err)
	if len(sigs) != 1 {
		t.Error("expected exactly 1 signature after signing")
	}
	sig := sigs[0]

	_, err = client.SendTx(t.Context(), tx)
	require.NoError(t, err)

	// Wait for transaction to be finalized since LogPoller only queries finalized events
	require.Eventually(t, func() bool {
		statuses, err := client.SignatureStatuses(t.Context(), []solana.Signature{sig})
		require.NoError(t, err)
		if len(statuses) == 0 || statuses[0] == nil {
			return false
		}
		status := statuses[0]
		return status.ConfirmationStatus == rpc.ConfirmationStatusFinalized
	}, 1*time.Minute, 1*time.Second)

	// Wait one LogPoller loop to ensure it picked up on the events
	time.Sleep(2 * cfg.BlockTime())
}

func buildMockCCIPSentEvent(t *testing.T, seqNum uint64, sender, receiver, feeToken solana.PublicKey) mock_ccip_events.CCIPMessageSentObj {
	t.Helper()

	return mock_ccip_events.CCIPMessageSentObj{
		DestChainSelector: chainsel.TEST_33333333333333333333333333333333333333333333.Selector,
		SequenceNumber:    seqNum,
		Message: mock_ccip_events.SVM2AnyRampMessage{
			Header: mock_ccip_events.RampMessageHeader{
				Nonce:               0,
				MessageId:           [32]byte{1}, // message id cannot be empty
				SourceChainSelector: chainsel.TEST_22222222222222222222222222222222222222222222.Selector,
				DestChainSelector:   chainsel.TEST_33333333333333333333333333333333333333333333.Selector,
				SequenceNumber:      seqNum,
			},
			Sender:         sender,
			Data:           []byte("ccip message sent data"),
			Receiver:       receiver.Bytes(),
			ExtraArgs:      nil,
			FeeToken:       feeToken,
			TokenAmounts:   nil,
			FeeTokenAmount: mock_ccip_events.CrossChainAmount{},
			FeeValueJuels:  mock_ccip_events.CrossChainAmount{},
		},
	}
}

func buildMockCommitEvent(t *testing.T, onRamp solana.PublicKey) mock_ccip_events.CommitReportAcceptedObj {
	t.Helper()

	return mock_ccip_events.CommitReportAcceptedObj{
		MerkleRoot: &mock_ccip_events.MerkleRoot{
			SourceChainSelector: chainsel.TEST_33333333333333333333333333333333333333333333.Selector,
			OnRampAddress:       onRamp.Bytes(),
			MinSeqNr:            1,
			MaxSeqNr:            1,
			MerkleRoot:          [32]byte{1}, // merkle root cannot be empty
		},
		PriceUpdates: mock_ccip_events.PriceUpdates{
			TokenPriceUpdates: nil,
			GasPriceUpdates:   nil,
		},
	}
}

func buildMockExecuteEvent(t *testing.T) mock_ccip_events.ExecutionStateChangedObj {
	t.Helper()

	return mock_ccip_events.ExecutionStateChangedObj{
		SourceChainSelector: chainsel.TEST_33333333333333333333333333333333333333333333.Selector,
		SequenceNumber:      1,
		MessageId:           [32]byte{1}, // message id cannot be empty
		MessageHash:         [32]byte{1}, // message hash cannot be empty
		State:               mock_ccip_events.Success_MessageExecutionState,
	}
}

func buildMockCCTPEvent(t *testing.T) mock_ccip_events.CcipCctpMessageSentEventObj {
	t.Helper()

	return mock_ccip_events.CcipCctpMessageSentEventObj{
		OriginalSender:      utils.GetRandomPubKey(t),
		RemoteChainSelector: chainsel.TEST_33333333333333333333333333333333333333333333.Selector,
		MsgTotalNonce:       1,
		EventAddress:        utils.GetRandomPubKey(t),
		SourceDomain:        0,
		CctpNonce:           1,
		MessageSentBytes:    []byte("test message sent bytes"),
	}
}
