package monitor

import (
	"context"
	"time"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	"github.com/smartcontractkit/chainlink-common/pkg/utils"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/internal"
)

// Config defines the monitor configuration.
type Config interface {
	BalancePollPeriod() time.Duration
}

// Keystore provides the keys to be monitored.
type Keystore interface {
	Accounts(ctx context.Context) ([]string, error)
}

type BalanceClient interface {
	Balance(ctx context.Context, addr solana.PublicKey) (uint64, error)
}

// NewBalanceMonitor returns a balance monitoring services.Service which reports the SOL balance of all ks keys to prometheus.
func NewBalanceMonitor(chainID string, cfg Config, lggr logger.Logger, ks Keystore, newReader func() (BalanceClient, error)) services.Service {
	return newBalanceMonitor(chainID, cfg, lggr, ks, newReader)
}

func newBalanceMonitor(chainID string, cfg Config, lggr logger.Logger, ks Keystore, newReader func() (BalanceClient, error)) *balanceMonitor {
	b := balanceMonitor{
		chainID:   chainID,
		cfg:       cfg,
		lggr:      logger.Named(lggr, "BalanceMonitor"),
		ks:        ks,
		newReader: newReader,
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
	}
	b.updateFn = b.updateProm
	return &b
}

type balanceMonitor struct {
	services.StateMachine
	chainID   string
	cfg       Config
	lggr      logger.Logger
	ks        Keystore
	newReader func() (BalanceClient, error)
	updateFn  func(acc solana.PublicKey, lamports uint64) // overridable for testing

	reader internal.Loader[BalanceClient]

	stop services.StopChan
	done chan struct{}
}

func (b *balanceMonitor) Name() string {
	return b.lggr.Name()
}

func (b *balanceMonitor) Start(context.Context) error {
	return b.StartOnce("BalanceMonitor", func() error {
		go b.monitor()
		return nil
	})
}

func (b *balanceMonitor) Close() error {
	return b.StopOnce("BalanceMonitor", func() error {
		close(b.stop)
		<-b.done
		return nil
	})
}

func (b *balanceMonitor) HealthReport() map[string]error {
	return map[string]error{b.Name(): b.Healthy()}
}

func (b *balanceMonitor) monitor() {
	defer close(b.done)
	ctx, cancel := b.stop.NewCtx()
	defer cancel()

	tick := time.After(utils.WithJitter(b.cfg.BalancePollPeriod()))
	for {
		select {
		case <-b.stop:
			return
		case <-tick:
			b.updateBalances(ctx)
			tick = time.After(utils.WithJitter(b.cfg.BalancePollPeriod()))
		}
	}
}

func (b *balanceMonitor) updateBalances(ctx context.Context) {
	ctx, cancel := b.stop.Ctx(ctx)
	defer cancel()

	keys, err := b.ks.Accounts(ctx)
	if err != nil {
		b.lggr.Errorw("Failed to get keys", "err", err)
		return
	}
	if len(keys) == 0 {
		return
	}
	reader, err := b.reader.Get()
	if err != nil {
		b.lggr.Errorw("Failed to get client", "err", err)
		return
	}
	var gotSomeBals bool
	for _, k := range keys {
		// Check for shutdown signal, since Balance blocks and may be slow.
		select {
		case <-ctx.Done():
			return
		default:
		}
		pubKey, err := solana.PublicKeyFromBase58(k)
		if err != nil {
			b.lggr.Errorw("Failed parse public key", "account", k, "err", err)
			continue
		}
		lamports, err := reader.Balance(ctx, pubKey)
		if err != nil {
			b.lggr.Errorw("Failed to get balance", "account", k, "err", err)
			continue
		}
		gotSomeBals = true
		b.updateFn(pubKey, lamports)
	}
	if !gotSomeBals {
		// Try a new client next time.
		b.reader.Reset()
	}
}
