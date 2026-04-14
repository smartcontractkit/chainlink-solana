package tests

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/ws"
	"github.com/gagliardetto/solana-go/text"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
	"golang.org/x/sync/errgroup"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	contract "github.com/smartcontractkit/chainlink-solana/contracts/generated/log_read_test"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller"
	logpollermocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/mocks"
	logpollertypes "github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
	solanatesting "github.com/smartcontractkit/chainlink-solana/pkg/solana/testing"

	"github.com/smartcontractkit/chainlink-solana/integration-tests/solclient"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/utils"
)

const programPubKey = "J1zQwrBNBngz26jRPNWsUSZMHJwBwpkoDitXRV95LdK4"

func TestEventLoader(t *testing.T) {
	t.Parallel()

	deadline, ok := t.Deadline()
	if !ok {
		deadline = time.Now().Add(time.Minute)
	}

	ctx, cancel := context.WithDeadline(t.Context(), deadline)
	defer cancel()

	// Getting the default localnet private key
	privateKey, err := solana.PrivateKeyFromBase58(solclient.DefaultPrivateKeysSolValidator[1])
	require.NoError(t, err)

	rpcURL, wsURL := setupTestValidator(t, privateKey.PublicKey().String())
	cl, rpcClient, err := client.NewTestClient(rpcURL, config.NewDefault(), 1*time.Second, logger.Nop())
	require.NoError(t, err)
	wsClient, err := ws.Connect(ctx, wsURL)
	require.NoError(t, err)

	defer wsClient.Close()

	require.NoError(t, err)
	solanatesting.FundTestAccounts(t, []solana.PublicKey{privateKey.PublicKey()}, rpcURL)

	totalLogsToSend := 30
	parser := &printParser{t: t}
	sender := newLogSender(t, rpcClient, wsClient)
	orm := logpollermocks.NewMockORM(t) // TODO: replace with real DB, when available
	programPubKey, err := solana.PublicKeyFromBase58(programPubKey)
	require.NoError(t, err)
	orm.EXPECT().SelectFilters(mock.Anything).Return([]logpollertypes.Filter{{ID: 1, IsBackfilled: false, Address: logpollertypes.PublicKey(programPubKey)}}, nil).Once()
	orm.EXPECT().MarkFilterBackfilled(mock.Anything, mock.Anything).Return(nil).Once()
	orm.EXPECT().GetLatestBlock(mock.Anything).Return(0, sql.ErrNoRows)
	orm.EXPECT().SelectSeqNums(mock.Anything).Return(map[int64]int64{1: 0}, nil).Once()
	orm.EXPECT().PruneLogsForFilter(mock.Anything, mock.Anything).Return(int64(0), nil).Maybe()
	lp, err := logpoller.NewWithCustomProcessor(logger.TestSugared(t), orm, cl, config.NewDefault(), "test-chain-id", parser.ProcessBlocks)
	require.NoError(t, err)

	require.NoError(t, lp.Start(ctx))
	t.Cleanup(func() {
		require.NoError(t, lp.Close())
	})

	go func(ctx context.Context, sender *logSender, privateKey *solana.PrivateKey) {
		var idx int

		for {
			idx++
			if idx > totalLogsToSend {
				return
			}

			timer := time.NewTimer(time.Second)

			select {
			case <-ctx.Done():
				timer.Stop()

				return
			case <-timer.C:
				if err := sender.sendLog(ctx, func(_ solana.PublicKey) *solana.PrivateKey {
					return privateKey
				}, privateKey.PublicKey(), uint64(idx)); err != nil {
					t.Logf("failed to send log: %s", err)
				}
			}

			timer.Stop()
		}
	}(ctx, sender, &privateKey)

	expectedSumOfLogValues := uint64((totalLogsToSend / 2) * (totalLogsToSend + 1))

	// eventually process all logs
	tests.AssertEventually(t, func() bool {
		return parser.Sum() == expectedSumOfLogValues
	})
}

func TestTruncatedLogs(t *testing.T) {
	t.Parallel()

	deadline, ok := t.Deadline()
	if !ok {
		deadline = time.Now().Add(time.Minute)
	}

	ctx, cancel := context.WithDeadline(t.Context(), deadline)
	defer cancel()

	// Getting the default localnet private key
	privateKey, err := solana.PrivateKeyFromBase58(solclient.DefaultPrivateKeysSolValidator[1])
	require.NoError(t, err)

	rpcURL, wsURL := setupTestValidator(t, privateKey.PublicKey().String())
	cl, rpcClient, err := client.NewTestClient(rpcURL, config.NewDefault(), 1*time.Second, logger.Nop())
	require.NoError(t, err)
	wsClient, err := ws.Connect(ctx, wsURL)
	require.NoError(t, err)

	defer wsClient.Close()

	require.NoError(t, err)
	solanatesting.FundTestAccounts(t, []solana.PublicKey{privateKey.PublicKey()}, rpcURL)

	sender := newLogSender(t, rpcClient, wsClient)
	orm := logpollermocks.NewMockORM(t) // TODO: replace with real DB, when available
	programPubKey, err := solana.PublicKeyFromBase58(programPubKey)
	require.NoError(t, err)
	orm.EXPECT().SelectFilters(mock.Anything).Return([]logpollertypes.Filter{{ID: 1, IsBackfilled: false, Address: logpollertypes.PublicKey(programPubKey)}}, nil).Once()
	orm.EXPECT().MarkFilterBackfilled(mock.Anything, mock.Anything).Return(nil).Once()
	orm.EXPECT().GetLatestBlock(mock.Anything).Return(0, sql.ErrNoRows)
	orm.EXPECT().SelectSeqNums(mock.Anything).Return(map[int64]int64{1: 0}, nil).Once()
	orm.EXPECT().PruneLogsForFilter(mock.Anything, mock.Anything).Return(int64(0), nil).Maybe()
	lggr, observed := logger.TestObservedSugared(t, zapcore.DebugLevel)
	lp, err := logpoller.New(lggr, orm, cl, config.NewDefault(), "test-chain-id")
	require.NoError(t, err)

	require.NoError(t, lp.Start(ctx))
	t.Cleanup(func() {
		require.NoError(t, lp.Close())
	})

	signerFunc := func(_ solana.PublicKey) *solana.PrivateKey {
		return &privateKey
	}

	err = sender.sendTruncatedLog(t.Context(), signerFunc, privateKey.PublicKey(), 0)
	require.NoError(t, err)

	// eventually process all logs
	tests.AssertLogEventually(t, observed, "Encountered truncated logs")
}

// upgradeAuthority is admin solana.PrivateKey as string
func setupTestValidator(t *testing.T, upgradeAuthority string) (string, string) {
	t.Helper()

	soPath := filepath.Join(utils.ContractsDir, "log_read_test.so")

	_, err := os.Stat(soPath)
	if err != nil {
		t.Log(err.Error())
		t.FailNow()
	}

	flags := []string{
		"--warp-slot", "42",
		"--upgradeable-program",
		programPubKey,
		soPath,
		upgradeAuthority,
	}

	return solanatesting.SetupLocalSolNodeWithFlags(t, flags...)
}

type testEvent struct {
	StrVal   string
	U64Value uint64
}

type printParser struct {
	t *testing.T

	mu     sync.RWMutex
	values []uint64
}

func (p *printParser) ProcessBlocks(_ context.Context, blocks []logpollertypes.Block) error {
	for _, b := range blocks {
		err := p.process(b)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *printParser) process(block logpollertypes.Block) error {
	p.t.Helper()

	sum := sha256.Sum256([]byte("event:TestEvent"))
	sig := sum[:8]

	for _, evt := range block.Events {
		data, err := base64.StdEncoding.DecodeString(evt.Data)
		if err != nil {
			return err
		}

		if bytes.Equal(sig, data[:8]) {
			var event testEvent
			if err := bin.UnmarshalBorsh(&event, data[8:]); err != nil {
				return nil
			}

			p.mu.Lock()
			p.values = append(p.values, event.U64Value)
			p.mu.Unlock()
		}
	}

	return nil
}

func (p *printParser) Sum() uint64 {
	p.t.Helper()

	p.mu.RLock()
	defer p.mu.RUnlock()

	var sum uint64

	for _, value := range p.values {
		sum += value
	}

	return sum
}

type logSender struct {
	t          *testing.T
	client     *rpc.Client
	wsClient   *ws.Client
	txErrGroup errgroup.Group
}

func newLogSender(t *testing.T, client *rpc.Client, wsClient *ws.Client) *logSender {
	return &logSender{
		t:          t,
		client:     client,
		wsClient:   wsClient,
		txErrGroup: errgroup.Group{},
	}
}

func (s *logSender) sendLog(
	ctx context.Context,
	signerFunc func(key solana.PublicKey) *solana.PrivateKey,
	payer solana.PublicKey,
	value uint64,
) error {
	s.t.Helper()

	pubKey, err := solana.PublicKeyFromBase58(programPubKey)
	require.NoError(s.t, err)
	contract.SetProgramID(pubKey)

	inst, err := contract.NewCreateLogInstruction(value, payer, solana.SystemProgramID).ValidateAndBuild()
	if err != nil {
		return err
	}

	return s.sendInstruction(ctx, inst, signerFunc, payer)
}

func (s *logSender) sendTruncatedLog(
	ctx context.Context,
	signerFunc func(key solana.PublicKey) *solana.PrivateKey,
	payer solana.PublicKey,
	value uint64,
) error {
	s.t.Helper()

	pubKey, err := solana.PublicKeyFromBase58(programPubKey)
	require.NoError(s.t, err)
	contract.SetProgramID(pubKey)

	inst, err := contract.NewCreateTruncatedLogInstruction(value, payer, solana.SystemProgramID).ValidateAndBuild()
	if err != nil {
		return err
	}

	return s.sendInstruction(ctx, inst, signerFunc, payer)
}

func (s *logSender) sendInstruction(
	ctx context.Context,
	inst *contract.Instruction,
	signerFunc func(key solana.PublicKey) *solana.PrivateKey,
	payer solana.PublicKey,
) error {
	s.t.Helper()

	recent, err := s.client.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return err
	}

	tx, err := solana.NewTransaction(
		[]solana.Instruction{
			inst,
		},
		recent.Value.Blockhash,
		solana.TransactionPayer(payer),
	)
	if err != nil {
		return err
	}

	if _, err = tx.EncodeTree(text.NewTreeEncoder(os.Stdout, "Send test log")); err != nil {
		return err
	}

	if _, err = tx.Sign(signerFunc); err != nil {
		return err
	}

	sig, err := s.client.SendTransactionWithOpts(
		ctx,
		tx,
		rpc.TransactionOpts{
			PreflightCommitment: rpc.CommitmentConfirmed,
		},
	)

	if err != nil {
		return err
	}

	s.queueTX(sig, rpc.CommitmentConfirmed)

	return nil
}

func (s *logSender) queueTX(sig solana.Signature, commitment rpc.CommitmentType) {
	s.t.Helper()

	s.txErrGroup.Go(func() error {
		sub, err := s.wsClient.SignatureSubscribe(
			sig,
			commitment,
		)
		if err != nil {
			return err
		}

		defer sub.Unsubscribe()

		res, err := sub.Recv(context.Background())
		if err != nil {
			return err
		}

		if res.Value.Err != nil {
			return fmt.Errorf("transaction confirmation failed: %v", res.Value.Err)
		}

		return nil
	})
}
