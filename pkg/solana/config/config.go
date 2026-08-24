package config

import (
	"errors"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-common/pkg/config"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
)

type Config interface {
	// general chain properties
	BlockTime() time.Duration

	// tx mgr
	BalancePollPeriod() time.Duration
	ConfirmPollPeriod() time.Duration
	OCR2CachePollPeriod() time.Duration
	OCR2CacheTTL() time.Duration
	TxTimeout() time.Duration
	TxRetryTimeout() time.Duration
	TxConfirmTimeout() time.Duration
	TxExpirationRebroadcast() bool
	TxRetentionTimeout() time.Duration
	SkipPreflight() bool
	Commitment() rpc.CommitmentType
	MaxRetries() *uint

	// fee estimator
	FeeEstimatorMode() string
	ComputeUnitPriceMax() uint64
	ComputeUnitPriceMin() uint64
	ComputeUnitPriceDefault() uint64
	FeeBumpPeriod() time.Duration
	BlockHistoryPollPeriod() time.Duration
	BlockHistorySize() uint64
	BlockHistoryBatchLoadSize() uint64
	ComputeUnitLimitDefault() uint32
	EstimateComputeUnitLimit() bool

	// log poller
	LogPollerStartingLookback() time.Duration
	LogPollerCPIEventsEnabled() bool
	LogPollerSlotsBatchSize() int64

	// workflow
	WF() Workflow
}

type Workflow interface {
	IsEnabled() bool
	AcceptanceTimeout() time.Duration
	PollPeriod() time.Duration
	GasLimitDefault() *uint64
	TxAcceptanceState() *commontypes.TransactionStatus
	RequestSizeLimit() uint32
	Local() bool // shows if workflow is run against local network
}

type WorkflowConfig struct {
	AcceptanceTimeout *config.Duration
	// ForwarderAddress is accepted for TOML compatibility only; runtime does not use chain config for this.
	//
	// Deprecated: configure the forwarder program address via capability config at runtime, not TOML.
	ForwarderAddress *solana.PublicKey
	// ForwarderState is accepted for TOML compatibility only; runtime does not use chain config for this.
	//
	// Deprecated: configure the forwarder state account via capability config at runtime, not TOML.
	ForwarderState *solana.PublicKey
	// FromAddress is accepted for TOML compatibility only; runtime does not use chain config for this.
	//
	// Deprecated: use the workflow transmitter / capability-injected key material instead of TOML.
	FromAddress       *solana.PublicKey
	GasLimitDefault   *uint64
	Local             *bool
	PollPeriod        *config.Duration
	TxAcceptanceState *commontypes.TransactionStatus
	// RequestSizeLimit caps the size in bytes of HTTP responses from Solana RPC for requests
	// originated by workflow capabilities (external requests). Applied per request via
	// commonhttp.WithResponseSizeLimit on the request context. Zero disables the cap.
	RequestSizeLimit *uint32
}

// IsEnabled reports whether workflow-style settings are configured: both AcceptanceTimeout and
// PollPeriod must be present with positive durations. Used e.g. to start the log poller.
func (w *WorkflowConfig) IsEnabled() bool {
	if w.AcceptanceTimeout == nil || w.PollPeriod == nil {
		return false
	}
	return w.AcceptanceTimeout.Duration() > 0 && w.PollPeriod.Duration() > 0
}

func (w *WorkflowConfig) SetFrom(f *WorkflowConfig) {
	if f.AcceptanceTimeout != nil {
		w.AcceptanceTimeout = f.AcceptanceTimeout
	}
	if f.ForwarderAddress != nil {
		w.ForwarderAddress = f.ForwarderAddress
	}
	if f.ForwarderState != nil {
		w.ForwarderState = f.ForwarderState
	}
	if f.FromAddress != nil {
		w.FromAddress = f.FromAddress
	}
	if f.GasLimitDefault != nil {
		w.GasLimitDefault = f.GasLimitDefault
	}
	if f.Local != nil {
		w.Local = f.Local
	}
	if f.PollPeriod != nil {
		w.PollPeriod = f.PollPeriod
	}
	if f.TxAcceptanceState != nil {
		w.TxAcceptanceState = f.TxAcceptanceState
	}
	if f.RequestSizeLimit != nil {
		w.RequestSizeLimit = f.RequestSizeLimit
	}
}

type Chain struct {
	BlockTime                 *config.Duration
	BalancePollPeriod         *config.Duration
	ConfirmPollPeriod         *config.Duration
	OCR2CachePollPeriod       *config.Duration
	OCR2CacheTTL              *config.Duration
	TxTimeout                 *config.Duration
	TxRetryTimeout            *config.Duration
	TxConfirmTimeout          *config.Duration
	TxExpirationRebroadcast   *bool
	TxRetentionTimeout        *config.Duration
	SkipPreflight             *bool
	Commitment                *string
	MaxRetries                *int64
	FeeEstimatorMode          *string
	ComputeUnitPriceMax       *uint64
	ComputeUnitPriceMin       *uint64
	ComputeUnitPriceDefault   *uint64
	FeeBumpPeriod             *config.Duration
	BlockHistoryPollPeriod    *config.Duration
	BlockHistorySize          *uint64
	BlockHistoryBatchLoadSize *uint64
	ComputeUnitLimitDefault   *uint32
	EstimateComputeUnitLimit  *bool
	LogPollerStartingLookback *config.Duration
	LogPollerCPIEventsEnabled *bool
	LogPollerSlotsBatchSize   *int64
}

type Node struct {
	Name              *string
	URL               *config.URL
	SendOnly          bool
	Order             *int32
	IsLoadBalancedRPC *bool
}

func (n *Node) ValidateConfig() (err error) {
	if n.Name == nil {
		err = errors.Join(err, config.ErrMissing{Name: "Name", Msg: "required for all nodes"})
	} else if *n.Name == "" {
		err = errors.Join(err, config.ErrEmpty{Name: "Name", Msg: "required for all nodes"})
	}
	if n.URL == nil {
		err = errors.Join(err, config.ErrMissing{Name: "URL", Msg: "required for all nodes"})
	} else if n.URL.String() == "" {
		err = errors.Join(err, config.ErrEmpty{Name: "URL", Msg: "required for all nodes"})
	}
	if n.Order != nil && (*n.Order < 1 || *n.Order > 100) {
		err = errors.Join(err, config.ErrInvalid{Name: "Order", Value: *n.Order, Msg: "must be between 1 and 100"})
	} else if n.Order == nil {
		z := int32(100)
		n.Order = &z
	}
	if n.IsLoadBalancedRPC == nil {
		z := false
		n.IsLoadBalancedRPC = &z
	}
	return err
}

func ptr[T any](t T) *T {
	return &t
}
