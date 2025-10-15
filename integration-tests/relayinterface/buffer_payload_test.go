package relayinterface

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	commonconfig "github.com/smartcontractkit/chainlink-common/pkg/config"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services/servicetest"
	"github.com/smartcontractkit/chainlink-common/pkg/types"

	"github.com/smartcontractkit/chainlink-solana/contracts/generated/buffer_payload"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/utils"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainwriter"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/txm"
	keyMocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/mocks"
	solanautils "github.com/smartcontractkit/chainlink-solana/pkg/solana/utils"

	ccipsolana "github.com/smartcontractkit/chainlink-ccip/chains/solana"
)

// NOTE: Ensure the test contract maintains the same buffer code as the CCIP offramp execute report buffer for compatibility with this test
var testBufferContractIDL = chainwriter.FetchTestBufferContractIDL()

const (
	testBufferContractPubKey = "85bivLENWAX36kyWC9zemZu9H3D88J79wXdHgR6ZmZHX"
)

func Test_BufferPayload(t *testing.T) {
	sender, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)

	url, _ := utils.SetupTestValidatorWithAnchorPrograms(t, sender.PublicKey().String(), []string{"buffer-payload"})
	rpcClient := rpc.New(url)

	utils.FundAccounts(t, []solana.PrivateKey{sender}, rpcClient)

	lggr := logger.Test(t)

	cfg := config.NewDefault()
	cfg.Chain.TxRetentionTimeout = commonconfig.MustNewDuration(10 * time.Minute)
	solanaClient, err := client.NewClient(url, cfg, 5*time.Second, lggr)
	require.NoError(t, err)

	multiClient := *client.NewMultiClient(func(context.Context) (client.ReaderWriter, error) {
		return solanaClient, nil
	})

	loader := solanautils.NewLoader(func(ctx context.Context) (client.ReaderWriter, error) { return solanaClient, nil })
	mkey := keyMocks.NewSimpleKeystore(t)
	mkey.On("Sign", mock.Anything, sender.PublicKey().String(), mock.Anything).Return(func(_ context.Context, _ string, data []byte) []byte {
		sig, _ := sender.Sign(data)
		return sig[:]
	}, nil).Maybe()

	lgr, logs := logger.TestObserved(t, zapcore.DebugLevel)
	txmgr, err := txm.NewTxm("localnet", loader, nil, cfg, mkey, lgr)
	require.NoError(t, err)
	err = txmgr.Start(t.Context())
	require.NoError(t, err)

	programID := solana.MustPublicKeyFromBase58(testBufferContractPubKey)
	initializeTestContract(t, rpcClient, sender, programID)

	contractName := "buffer"
	methodName := "execute"

	cwConfig := chainwriter.ChainWriterConfig{
		Programs: map[string]chainwriter.ProgramConfig{
			contractName: {
				Methods: map[string]chainwriter.MethodConfig{
					methodName: {
						FromAddress:              sender.PublicKey().String(),
						ChainSpecificName:        "execute",
						ComputeUnitLimitOverhead: 150_000,
						BufferPayloadMethod:      "CCIPExecutionReportBuffer",
						Accounts: []chainwriter.Lookup{
							{
								AccountConstant: &chainwriter.AccountConstant{
									Address:    sender.PublicKey().String(),
									IsSigner:   true,
									IsWritable: true,
								},
							},
							{
								AccountConstant: &chainwriter.AccountConstant{
									Address:    solana.SystemProgramID.String(),
									IsSigner:   false,
									IsWritable: false,
								},
							},
						},
					},
				},
				IDL: testBufferContractIDL,
			},
		},
	}

	t.Run("happy path, writes payload to buffer, uses buffer for main transaction", func(t *testing.T) {
		methodConfig := cwConfig.Programs[contractName].Methods[methodName]
		methodConfig.InputModifications = []commoncodec.ModifierConfig{&commoncodec.HardCodeModifierConfig{OnChainValues: map[string]any{"Fail": false}}}
		cwConfig.Programs[contractName].Methods[methodName] = methodConfig
		cw := initializeAndRunCW(t, lggr, multiClient, txmgr, cwConfig)
		args := ccipsolana.SVMExecCallArgs{
			Report: make([]byte, 2000), // Requires 3 buffer transactions
		}
		txID := uuid.NewString()
		err = cw.SubmitTransaction(t.Context(), contractName, methodName, args, txID, testBufferContractPubKey, nil, nil)
		require.NoError(t, err)

		waitForStatus(t, cw, txID, true)
	})

	t.Run("sends main transaction without buffer when report is within size limit", func(t *testing.T) {
		methodConfig := cwConfig.Programs[contractName].Methods[methodName]
		methodConfig.InputModifications = []commoncodec.ModifierConfig{&commoncodec.HardCodeModifierConfig{OnChainValues: map[string]any{"Fail": false}}}
		cwConfig.Programs[contractName].Methods[methodName] = methodConfig
		cw := initializeAndRunCW(t, lggr, multiClient, txmgr, cwConfig)

		args := ccipsolana.SVMExecCallArgs{
			Report: make([]byte, 200), // Does not require buffer
		}
		txID := uuid.NewString()
		err = cw.SubmitTransaction(t.Context(), contractName, methodName, args, txID, testBufferContractPubKey, nil, nil)
		require.NoError(t, err)

		waitForStatus(t, cw, txID, true)
	})

	t.Run("writes payload to buffer, main transaction fails, buffer is closed in follow up transaction", func(t *testing.T) {
		methodConfig := cwConfig.Programs[contractName].Methods[methodName]
		// Configure the main transaction to fail
		methodConfig.InputModifications = []commoncodec.ModifierConfig{&commoncodec.HardCodeModifierConfig{OnChainValues: map[string]any{"Fail": true}}}
		cwConfig.Programs[contractName].Methods[methodName] = methodConfig

		cw := initializeAndRunCW(t, lggr, multiClient, txmgr, cwConfig)

		args := ccipsolana.SVMExecCallArgs{
			Report: make([]byte, 2000), // Requires 3 buffer transactions
		}
		txID := uuid.NewString()
		err = cw.SubmitTransaction(t.Context(), contractName, methodName, args, txID, testBufferContractPubKey, nil, nil)
		require.NoError(t, err)

		waitForStatus(t, cw, txID, false)

		require.Eventually(t, func() bool {
			// Check logs for dependent transactions getting queued
			depTxQueuedLogs := logs.FilterMessageSnippet("enqueued tx after dependencies reached desired status")
			// Check filtered logs to see if the close buffer transcation specifically was queued
			for _, log := range depTxQueuedLogs.All() {
				for key, field := range log.ContextMap() {
					if strings.Contains(key, "id") {
						value, ok := field.(string)
						require.True(t, ok)
						if strings.Contains(value, "CloseBuffer") {
							return true
						}
					}
				}
			}
			return false
		}, 30*time.Second, time.Second, "close buffer transaction never broadacasted")
	})
}

func initializeAndRunCW(t *testing.T, lggr logger.Logger, multiClient client.MultiClient, txm *txm.Txm, config chainwriter.ChainWriterConfig) *chainwriter.SolanaChainWriterService {
	cw, err := chainwriter.NewSolanaChainWriterService(lggr, multiClient, txm, nil, config)
	require.NoError(t, err)
	servicetest.Run(t, cw)
	return cw
}

func initializeTestContract(t *testing.T, client *rpc.Client, sender solana.PrivateKey, programID solana.PublicKey) {
	t.Helper()

	configPDA, _, err := solana.FindProgramAddress([][]byte{[]byte("config")}, programID)
	require.NoError(t, err)
	buffer_payload.SetProgramID(programID)
	initIx, err := buffer_payload.NewInitializeInstruction(configPDA, sender.PublicKey(), solana.SystemProgramID).ValidateAndBuild()
	require.NoError(t, err)
	utils.SendAndConfirm(t.Context(), t, client, []solana.Instruction{initIx}, sender, rpc.CommitmentFinalized)
}

func waitForStatus(t *testing.T, cw *chainwriter.SolanaChainWriterService, txID string, requireSuccess bool) {
	t.Helper()

	require.Eventually(t, func() bool {
		status, err := cw.GetTransactionStatus(t.Context(), txID)
		require.NoError(t, err)
		if requireSuccess {
			if status == types.Failed || status == types.Fatal {
				require.FailNow(t, "transaction failed when expecting success")
			}
			return status == types.Finalized
		}
		if status == types.Unconfirmed || status == types.Finalized {
			require.FailNow(t, "transaction succeeded when expecting failure")
		}
		return status == types.Failed || status == types.Fatal
	}, 5*time.Minute, time.Second, "transaction failed to reach expected status")
}
