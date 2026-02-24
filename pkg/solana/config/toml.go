package config

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/pelletier/go-toml/v2"
	"golang.org/x/exp/slices"

	"github.com/smartcontractkit/chainlink-common/pkg/config"
	"github.com/smartcontractkit/chainlink-common/pkg/config/configtest"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	mnCfg "github.com/smartcontractkit/chainlink-framework/multinode/config"
)

var defaults TOMLConfig

func init() {
	if err := configtest.DocDefaultsOnly(strings.NewReader(docsTOML), &defaults, config.DecodeTOML); err != nil {
		log.Fatalf("Failed to initialize defaults from docs: %v", err)
	}
}

func Defaults() (c TOMLConfig) {
	c.SetFrom(&defaults)
	return
}

type TOMLConfigs []*TOMLConfig

func (cs TOMLConfigs) ValidateConfig() (err error) {
	return cs.validateKeys()
}

func (cs TOMLConfigs) validateKeys() (err error) {
	// Unique chain IDs
	chainIDs := config.UniqueStrings{}
	for i, c := range cs {
		if chainIDs.IsDupe(c.ChainID) {
			err = errors.Join(err, config.NewErrDuplicate(fmt.Sprintf("%d.ChainID", i), *c.ChainID))
		}
	}

	// Unique node names
	names := config.UniqueStrings{}
	for i, c := range cs {
		for j, n := range c.Nodes {
			if names.IsDupe(n.Name) {
				err = errors.Join(err, config.NewErrDuplicate(fmt.Sprintf("%d.Nodes.%d.Name", i, j), *n.Name))
			}
		}
	}

	// Unique URLs
	urls := config.UniqueStrings{}
	for i, c := range cs {
		for j, n := range c.Nodes {
			u := (*url.URL)(n.URL)
			if urls.IsDupeFmt(u) {
				err = errors.Join(err, config.NewErrDuplicate(fmt.Sprintf("%d.Nodes.%d.URL", i, j), u.String()))
			}
		}
	}
	return
}

func (cs *TOMLConfigs) SetFrom(fs *TOMLConfigs) (err error) {
	if err1 := fs.validateKeys(); err1 != nil {
		return err1
	}
	for _, f := range *fs {
		if f.ChainID == nil {
			*cs = append(*cs, f)
		} else if i := slices.IndexFunc(*cs, func(c *TOMLConfig) bool {
			return c.ChainID != nil && *c.ChainID == *f.ChainID
		}); i == -1 {
			*cs = append(*cs, f)
		} else {
			(*cs)[i].SetFrom(f)
		}
	}
	return
}

func NodeStatus(n *Node, id string) (types.NodeStatus, error) {
	var s types.NodeStatus
	s.ChainID = id
	s.Name = *n.Name
	b, err := toml.Marshal(n)
	if err != nil {
		return types.NodeStatus{}, err
	}
	s.Config = string(b)
	return s, nil
}

type Nodes []*Node

func (ns *Nodes) SetFrom(fs *Nodes) {
	for _, f := range *fs {
		if f.Name == nil {
			*ns = append(*ns, f)
		} else if i := slices.IndexFunc(*ns, func(n *Node) bool {
			return n.Name != nil && *n.Name == *f.Name
		}); i == -1 {
			*ns = append(*ns, f)
		} else {
			(*ns)[i].SetFrom(f)
		}
	}
}

func (n *Node) SetFrom(f *Node) {
	if f.Name != nil {
		n.Name = f.Name
	}
	if f.URL != nil {
		n.URL = f.URL
	}
	if f.Order != nil {
		n.Order = f.Order
	}
	n.SendOnly = f.SendOnly
}

type TOMLConfig struct {
	ChainID *string
	// Do not access directly, use [IsEnabled]
	Enabled *bool
	Chain
	Workflow  WorkflowConfig `toml:",omitempty"`
	MultiNode mnCfg.MultiNodeConfig
	Nodes     Nodes
}

func (c *TOMLConfig) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

func (c *TOMLConfig) SetDefaults() {
	d := Defaults()
	d.SetFrom(c)
	*c = d
}

func (c *TOMLConfig) SetFrom(f *TOMLConfig) {
	if f.ChainID != nil {
		c.ChainID = f.ChainID
	}
	if f.Enabled != nil {
		c.Enabled = f.Enabled
	}
	c.Chain.SetFrom(&f.Chain)
	c.MultiNode.SetFrom(&f.MultiNode)
	c.Workflow.SetFrom(&f.Workflow)
	c.Nodes.SetFrom(&f.Nodes)
}

func (c *Chain) SetFrom(f *Chain) {
	if f.BlockTime != nil {
		c.BlockTime = f.BlockTime
	}
	if f.BalancePollPeriod != nil {
		c.BalancePollPeriod = f.BalancePollPeriod
	}
	if f.ConfirmPollPeriod != nil {
		c.ConfirmPollPeriod = f.ConfirmPollPeriod
	}
	if f.OCR2CachePollPeriod != nil {
		c.OCR2CachePollPeriod = f.OCR2CachePollPeriod
	}
	if f.OCR2CacheTTL != nil {
		c.OCR2CacheTTL = f.OCR2CacheTTL
	}
	if f.TxTimeout != nil {
		c.TxTimeout = f.TxTimeout
	}
	if f.TxRetryTimeout != nil {
		c.TxRetryTimeout = f.TxRetryTimeout
	}
	if f.TxConfirmTimeout != nil {
		c.TxConfirmTimeout = f.TxConfirmTimeout
	}
	if f.TxExpirationRebroadcast != nil {
		c.TxExpirationRebroadcast = f.TxExpirationRebroadcast
	}
	if f.TxRetentionTimeout != nil {
		c.TxRetentionTimeout = f.TxRetentionTimeout
	}
	if f.SkipPreflight != nil {
		c.SkipPreflight = f.SkipPreflight
	}
	if f.Commitment != nil {
		c.Commitment = f.Commitment
	}
	if f.MaxRetries != nil {
		c.MaxRetries = f.MaxRetries
	}
	if f.FeeEstimatorMode != nil {
		c.FeeEstimatorMode = f.FeeEstimatorMode
	}
	if f.ComputeUnitPriceMax != nil {
		c.ComputeUnitPriceMax = f.ComputeUnitPriceMax
	}
	if f.ComputeUnitPriceMin != nil {
		c.ComputeUnitPriceMin = f.ComputeUnitPriceMin
	}
	if f.ComputeUnitPriceDefault != nil {
		c.ComputeUnitPriceDefault = f.ComputeUnitPriceDefault
	}
	if f.FeeBumpPeriod != nil {
		c.FeeBumpPeriod = f.FeeBumpPeriod
	}
	if f.BlockHistoryPollPeriod != nil {
		c.BlockHistoryPollPeriod = f.BlockHistoryPollPeriod
	}
	if f.BlockHistorySize != nil {
		c.BlockHistorySize = f.BlockHistorySize
	}
	if f.BlockHistoryBatchLoadSize != nil {
		c.BlockHistoryBatchLoadSize = f.BlockHistoryBatchLoadSize
	}
	if f.ComputeUnitLimitDefault != nil {
		c.ComputeUnitLimitDefault = f.ComputeUnitLimitDefault
	}
	if f.EstimateComputeUnitLimit != nil {
		c.EstimateComputeUnitLimit = f.EstimateComputeUnitLimit
	}
	if f.LogPollerStartingLookback != nil {
		c.LogPollerStartingLookback = f.LogPollerStartingLookback
	}
	if f.LogPollerCPIEventsEnabled != nil {
		c.LogPollerCPIEventsEnabled = f.LogPollerCPIEventsEnabled
	}
}

func (c *TOMLConfig) ValidateConfig() (err error) {
	if c.ChainID == nil {
		err = errors.Join(err, config.ErrMissing{Name: "ChainID", Msg: "required for all chains"})
	} else if *c.ChainID == "" {
		err = errors.Join(err, config.ErrEmpty{Name: "ChainID", Msg: "required for all chains"})
	}

	if len(c.Nodes) == 0 {
		err = errors.Join(err, config.ErrMissing{Name: "Nodes", Msg: "must have at least one node"})
	}

	if c.BlockTime() <= 0 {
		err = errors.Join(err, config.ErrInvalid{Name: "BlockTime", Msg: "must be greater than 0"})
	}

	return
}

func (c *TOMLConfig) TOMLString() (string, error) {
	b, err := toml.Marshal(c)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

var _ Config = &TOMLConfig{}

func (c *TOMLConfig) WF() Workflow {
	return &workflowConfig{
		conf: c.Workflow,
	}
}

func (c *TOMLConfig) AcceptanceTimeout() time.Duration {
	return c.Workflow.AcceptanceTimeout.Duration()
}

func (c *TOMLConfig) Local() bool {
	return *c.Workflow.Local
}

func (c *TOMLConfig) PollPeriod() time.Duration {
	return c.Workflow.PollPeriod.Duration()
}

func (c *TOMLConfig) ForwarderAddress() *solana.PublicKey {
	return c.Workflow.ForwarderAddress
}

func (c *TOMLConfig) FromAddress() *solana.PublicKey {
	return c.Workflow.FromAddress
}

func (c *TOMLConfig) ForwarderState() *solana.PublicKey {
	return c.Workflow.ForwarderState
}

func (c *TOMLConfig) GasLimitDefault() *uint64 {
	return c.Workflow.GasLimitDefault
}
func (c *TOMLConfig) TxAcceptanceState() *types.TransactionStatus {
	return c.Workflow.TxAcceptanceState
}

type workflowConfig struct {
	conf WorkflowConfig
}

func (wc *workflowConfig) IsEnabled() bool {
	return wc.conf.IsEnabled()
}

func (wc *workflowConfig) AcceptanceTimeout() time.Duration {
	return wc.conf.AcceptanceTimeout.Duration()
}
func (wc *workflowConfig) PollPeriod() time.Duration {
	return wc.conf.PollPeriod.Duration()
}
func (wc *workflowConfig) ForwarderAddress() *solana.PublicKey {
	return wc.conf.ForwarderAddress
}
func (wc *workflowConfig) FromAddress() *solana.PublicKey {
	return wc.conf.FromAddress
}
func (wc *workflowConfig) ForwarderState() *solana.PublicKey {
	return wc.conf.ForwarderState
}
func (wc *workflowConfig) GasLimitDefault() *uint64 {
	return wc.conf.GasLimitDefault
}
func (wc *workflowConfig) TxAcceptanceState() *types.TransactionStatus {
	return wc.conf.TxAcceptanceState
}
func (wc *workflowConfig) Local() bool {
	return *wc.conf.Local
}

func (c *TOMLConfig) BlockTime() time.Duration {
	return c.Chain.BlockTime.Duration()
}

func (c *TOMLConfig) BalancePollPeriod() time.Duration {
	return c.Chain.BalancePollPeriod.Duration()
}

func (c *TOMLConfig) ConfirmPollPeriod() time.Duration {
	return c.Chain.ConfirmPollPeriod.Duration()
}

func (c *TOMLConfig) OCR2CachePollPeriod() time.Duration {
	return c.Chain.OCR2CachePollPeriod.Duration()
}

func (c *TOMLConfig) OCR2CacheTTL() time.Duration {
	return c.Chain.OCR2CacheTTL.Duration()
}

func (c *TOMLConfig) TxTimeout() time.Duration {
	return c.Chain.TxTimeout.Duration()
}

func (c *TOMLConfig) TxRetryTimeout() time.Duration {
	return c.Chain.TxRetryTimeout.Duration()
}

func (c *TOMLConfig) TxConfirmTimeout() time.Duration {
	return c.Chain.TxConfirmTimeout.Duration()
}

func (c *TOMLConfig) TxExpirationRebroadcast() bool {
	return *c.Chain.TxExpirationRebroadcast
}

func (c *TOMLConfig) TxRetentionTimeout() time.Duration {
	return c.Chain.TxRetentionTimeout.Duration()
}
func (c *TOMLConfig) SkipPreflight() bool {
	return *c.Chain.SkipPreflight
}

func (c *TOMLConfig) Commitment() rpc.CommitmentType {
	return rpc.CommitmentType(*c.Chain.Commitment)
}

func (c *TOMLConfig) MaxRetries() *uint {
	if c.Chain.MaxRetries == nil {
		return nil
	}
	if *c.Chain.MaxRetries < 0 {
		return nil // interpret negative numbers as nil (prevents unlikely case of overflow)
	}
	mr := uint(*c.Chain.MaxRetries) //nolint:gosec // overflow check is handled above
	return &mr
}

func (c *TOMLConfig) FeeEstimatorMode() string {
	return *c.Chain.FeeEstimatorMode
}

func (c *TOMLConfig) ComputeUnitPriceMax() uint64 {
	return *c.Chain.ComputeUnitPriceMax
}

func (c *TOMLConfig) ComputeUnitPriceMin() uint64 {
	return *c.Chain.ComputeUnitPriceMin
}

func (c *TOMLConfig) ComputeUnitPriceDefault() uint64 {
	return *c.Chain.ComputeUnitPriceDefault
}

func (c *TOMLConfig) FeeBumpPeriod() time.Duration {
	return c.Chain.FeeBumpPeriod.Duration()
}

func (c *TOMLConfig) BlockHistoryPollPeriod() time.Duration {
	return c.Chain.BlockHistoryPollPeriod.Duration()
}

func (c *TOMLConfig) BlockHistorySize() uint64 {
	return *c.Chain.BlockHistorySize
}

func (c *TOMLConfig) BlockHistoryBatchLoadSize() uint64 {
	return *c.Chain.BlockHistoryBatchLoadSize
}

func (c *TOMLConfig) ComputeUnitLimitDefault() uint32 {
	return *c.Chain.ComputeUnitLimitDefault
}

func (c *TOMLConfig) EstimateComputeUnitLimit() bool {
	return *c.Chain.EstimateComputeUnitLimit
}

func (c *TOMLConfig) LogPollerStartingLookback() time.Duration {
	return c.Chain.LogPollerStartingLookback.Duration()
}

func (c *TOMLConfig) LogPollerCPIEventsEnabled() bool {
	return *c.Chain.LogPollerCPIEventsEnabled
}

func (c *TOMLConfig) ListNodes() Nodes {
	return c.Nodes
}

func NewDefault() *TOMLConfig {
	cfg := &TOMLConfig{}
	cfg.SetDefaults()
	return cfg
}

func NewDefaultMultiNodeConfig() mnCfg.MultiNodeConfig {
	return NewDefault().MultiNode
}
