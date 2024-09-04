package solana

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	solanago "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/smartcontractkit/chainlink-common/pkg/chains"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/loop"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	relaytypes "github.com/smartcontractkit/chainlink-common/pkg/types"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	mn "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/multinode"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/monitor"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/txm"
)

func NewMultiNodeChain(cfg *config.TOMLConfig, opts ChainOpts) (Chain, error) {
	if !cfg.IsEnabled() {
		return nil, fmt.Errorf("cannot create new chain with ID %s: chain is disabled", *cfg.ChainID)
	}
	c, err := newMultiNodeChain(*cfg.ChainID, cfg, opts.KeyStore, opts.Logger)
	if err != nil {
		return nil, err
	}
	return c, nil
}

var _ Chain = (*multiNodeChain)(nil)

type multiNodeChain struct {
	services.StateMachine
	id             string
	cfg            *config.TOMLConfig
	multiNode      *mn.MultiNode[mn.StringID, *client.RpcClient]
	txSender       *mn.TransactionSender[*solanago.Transaction, mn.StringID, *client.RpcClient]
	txm            *txm.Txm
	balanceMonitor services.Service
	lggr           logger.Logger

	clientLock sync.RWMutex
}

func newMultiNodeChain(id string, cfg *config.TOMLConfig, ks loop.Keystore, lggr logger.Logger) (*multiNodeChain, error) {
	lggr = logger.With(lggr, "chainID", id, "chain", "solana")

	chainFamily := "solana"

	cfg.BlockHistoryPollPeriod()

	mnCfg := cfg.MultiNodeConfig()

	var nodes []mn.Node[mn.StringID, *client.RpcClient]

	for i, nodeInfo := range cfg.ListNodes() {
		// create client and check
		rpcClient, err := client.NewRpcClient(nodeInfo.URL.String(), cfg, DefaultRequestTimeout, logger.Named(lggr, "Client."+*nodeInfo.Name))
		if err != nil {
			lggr.Warnw("failed to create client", "name", *nodeInfo.Name, "solana-url", nodeInfo.URL.String(), "err", err.Error())
			continue
		}

		newNode := mn.NewNode[mn.StringID, *client.Head, *client.RpcClient](
			mnCfg, mnCfg, lggr, *nodeInfo.URL.URL(), nil, *nodeInfo.Name,
			int32(i), mn.StringID(id), 0, rpcClient, chainFamily)

		nodes = append(nodes, newNode)
	}

	multiNode := mn.NewMultiNode[mn.StringID, *client.RpcClient](
		lggr,
		mn.NodeSelectionModeRoundRobin,
		time.Duration(0), // TODO: set lease duration
		nodes,
		[]mn.SendOnlyNode[mn.StringID, *client.RpcClient]{}, // TODO: no send only nodes?
		mn.StringID(id),
		chainFamily,
		time.Duration(0), // TODO: set deathDeclarationDelay
	)

	classifySendError := func(tx *solanago.Transaction, err error) mn.SendTxReturnCode {
		return 0 // TODO ClassifySendError(err, clientErrors, logger.Sugared(logger.Nop()), tx, common.Address{}, false)
	}

	txSender := mn.NewTransactionSender[*solanago.Transaction, mn.StringID, *client.RpcClient](
		lggr,
		mn.StringID(id),
		chainFamily,
		multiNode,
		classifySendError,
		0, // use the default value provided by the implementation
	)

	var ch = multiNodeChain{
		id:        id,
		cfg:       cfg,
		multiNode: multiNode,
		txSender:  txSender,
		lggr:      logger.Named(lggr, "Chain"),
	}

	tc := func() (client.ReaderWriter, error) {
		return ch.multiNode.SelectRPC()
	}

	ch.txm = txm.NewTxm(ch.id, tc, cfg, ks, lggr)
	bc := func() (monitor.BalanceClient, error) {
		return ch.multiNode.SelectRPC()
	}
	ch.balanceMonitor = monitor.NewBalanceMonitor(ch.id, cfg, lggr, ks, bc)
	return &ch, nil
}

// ChainService interface
func (c *multiNodeChain) GetChainStatus(ctx context.Context) (relaytypes.ChainStatus, error) {
	toml, err := c.cfg.TOMLString()
	if err != nil {
		return relaytypes.ChainStatus{}, err
	}
	return relaytypes.ChainStatus{
		ID:      c.id,
		Enabled: c.cfg.IsEnabled(),
		Config:  toml,
	}, nil
}

func (c *multiNodeChain) ListNodeStatuses(ctx context.Context, pageSize int32, pageToken string) (stats []relaytypes.NodeStatus, nextPageToken string, total int, err error) {
	return chains.ListNodeStatuses(int(pageSize), pageToken, c.listNodeStatuses)
}

func (c *multiNodeChain) Transact(ctx context.Context, from, to string, amount *big.Int, balanceCheck bool) error {
	return c.sendTx(ctx, from, to, amount, balanceCheck)
}

func (c *multiNodeChain) listNodeStatuses(start, end int) ([]relaytypes.NodeStatus, int, error) {
	stats := make([]relaytypes.NodeStatus, 0)
	total := len(c.cfg.Nodes)
	if start >= total {
		return stats, total, chains.ErrOutOfRange
	}
	if end > total {
		end = total
	}
	nodes := c.cfg.Nodes[start:end]
	for _, node := range nodes {
		stat, err := config.NodeStatus(node, c.ChainID())
		if err != nil {
			return stats, total, err
		}
		stats = append(stats, stat)
	}
	return stats, total, nil
}

func (c *multiNodeChain) Name() string {
	return c.lggr.Name()
}

func (c *multiNodeChain) ID() string {
	return c.id
}

func (c *multiNodeChain) Config() config.Config {
	return c.cfg
}

func (c *multiNodeChain) TxManager() TxManager {
	return c.txm
}

func (c *multiNodeChain) Reader() (client.Reader, error) {
	return c.multiNode.SelectRPC()
}

func (c *multiNodeChain) ChainID() string {
	return c.id
}

func (c *multiNodeChain) Start(ctx context.Context) error {
	return c.StartOnce("Chain", func() error {
		c.lggr.Debug("Starting")
		c.lggr.Debug("Starting txm")
		c.lggr.Debug("Starting balance monitor")
		var ms services.MultiStart
		return ms.Start(ctx, c.txm, c.balanceMonitor)
	})
}

func (c *multiNodeChain) Close() error {
	return c.StopOnce("Chain", func() error {
		c.lggr.Debug("Stopping")
		c.lggr.Debug("Stopping txm")
		c.lggr.Debug("Stopping balance monitor")
		return services.CloseAll(c.txm, c.balanceMonitor)
	})
}

func (c *multiNodeChain) Ready() error {
	return errors.Join(
		c.StateMachine.Ready(),
		c.txm.Ready(),
	)
}

func (c *multiNodeChain) HealthReport() map[string]error {
	report := map[string]error{c.Name(): c.Healthy()}
	services.CopyHealth(report, c.txm.HealthReport())
	return report
}

func (c *multiNodeChain) sendTx(ctx context.Context, from, to string, amount *big.Int, balanceCheck bool) error {
	reader, err := c.Reader()
	if err != nil {
		return fmt.Errorf("chain unreachable: %w", err)
	}

	fromKey, err := solanago.PublicKeyFromBase58(from)
	if err != nil {
		return fmt.Errorf("failed to parse from key: %w", err)
	}
	toKey, err := solanago.PublicKeyFromBase58(to)
	if err != nil {
		return fmt.Errorf("failed to parse to key: %w", err)
	}
	if !amount.IsUint64() {
		return fmt.Errorf("amount %s overflows uint64", amount)
	}
	amountI := amount.Uint64()

	blockhash, err := reader.LatestBlockhash()
	if err != nil {
		return fmt.Errorf("failed to get latest block hash: %w", err)
	}
	tx, err := solanago.NewTransaction(
		[]solanago.Instruction{
			system.NewTransferInstruction(
				amountI,
				fromKey,
				toKey,
			).Build(),
		},
		blockhash.Value.Blockhash,
		solanago.TransactionPayer(fromKey),
	)
	if err != nil {
		return fmt.Errorf("failed to create tx: %w", err)
	}

	if balanceCheck {
		if err = solanaValidateBalance(reader, fromKey, amountI, tx.Message.ToBase64()); err != nil {
			return fmt.Errorf("failed to validate balance: %w", err)
		}
	}

	txm := c.TxManager()
	err = txm.Enqueue("", tx)
	if err != nil {
		return fmt.Errorf("transaction failed: %w", err)
	}
	return nil
}
