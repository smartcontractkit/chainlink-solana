package utils

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
)

type TxState int

// tx not found
// < tx errored
// < tx broadcasted
// < tx processed
// < tx confirmed
// < tx finalized
// < tx fatallyErrored
const (
	NotFound TxState = iota
	Errored
	AwaitingBroadcast
	Broadcasted
	Processed
	Confirmed
	Finalized
	FatallyErrored
)

func (s TxState) String() string {
	switch s {
	case NotFound:
		return "NotFound"
	case Errored:
		return "Errored"
	case AwaitingBroadcast:
		return "AwaitingBroadcast"
	case Broadcasted:
		return "Broadcasted"
	case Processed:
		return "Processed"
	case Confirmed:
		return "Confirmed"
	case Finalized:
		return "Finalized"
	case FatallyErrored:
		return "FatallyErrored"
	default:
		return fmt.Sprintf("TxState(%d)", s)
	}
}

type statuses struct {
	sigs []solana.Signature
	res  []*rpc.SignatureStatusesResult
}

func (s statuses) Len() int {
	return len(s.res)
}

func (s statuses) Swap(i, j int) {
	// no-op if indexes are ever out of bounds
	if i >= len(s.sigs) || j >= len(s.sigs) {
		return
	}
	if i >= len(s.res) || j >= len(s.res) {
		return
	}
	s.sigs[i], s.sigs[j] = s.sigs[j], s.sigs[i]
	s.res[i], s.res[j] = s.res[j], s.res[i]
}

func (s statuses) Less(i, j int) bool {
	// no-op if indexes are ever out of bounds
	if i >= len(s.res) || j >= len(s.res) {
		return true
	}
	return ConvertStatus(s.res[i]) > ConvertStatus(s.res[j]) // returns list with highest first
}

func SortSignaturesAndResults(sigs []solana.Signature, res []*rpc.SignatureStatusesResult) ([]solana.Signature, []*rpc.SignatureStatusesResult, error) {
	if len(sigs) != len(res) {
		return []solana.Signature{}, []*rpc.SignatureStatusesResult{}, fmt.Errorf("signatures and results lengths do not match")
	}

	s := statuses{
		sigs: sigs,
		res:  res,
	}
	sort.Sort(s)
	return s.sigs, s.res, nil
}

func ConvertStatus(res *rpc.SignatureStatusesResult) TxState {
	if res == nil {
		return NotFound
	}

	if res.ConfirmationStatus == rpc.ConfirmationStatusProcessed {
		return Processed
	}

	if res.ConfirmationStatus == rpc.ConfirmationStatusConfirmed {
		// If result contains error, consider the transaction errored to avoid wasted resources on re-org and expiration protection
		if res.Err != nil {
			return Errored
		}
		return Confirmed
	}

	if res.ConfirmationStatus == rpc.ConfirmationStatusFinalized {
		// If result contains error, consider the transaction errored
		// Should be caught earlier but checked here in case confirmed is skipped due to delays or slow polling
		if res.Err != nil {
			return Errored
		}
		return Finalized
	}

	return NotFound
}

type SignatureList struct {
	sigs []solana.Signature
	lock sync.RWMutex
	wg   []*sync.WaitGroup
}

// internal function that should be called using the proper lock
func (s *SignatureList) get(index int) (sig solana.Signature, err error) {
	if index >= len(s.sigs) {
		return sig, errors.New("invalid index")
	}
	return s.sigs[index], nil
}

func (s *SignatureList) Get(index int) (sig solana.Signature, err error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.get(index)
}

func (s *SignatureList) List() []solana.Signature {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.sigs
}

func (s *SignatureList) Length() int {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return len(s.sigs)
}

func (s *SignatureList) Allocate() (index int) {
	s.lock.Lock()
	defer s.lock.Unlock()

	var wg sync.WaitGroup
	wg.Add(1)

	s.sigs = append(s.sigs, solana.Signature{})
	s.wg = append(s.wg, &wg)

	return len(s.sigs) - 1
}

func (s *SignatureList) Set(index int, sig solana.Signature) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	v, err := s.get(index)
	if err != nil {
		return err
	}

	if !v.IsZero() {
		return fmt.Errorf("trying to set signature when already set - index: %d, existing: %s, new: %s", index, v, sig)
	}

	s.sigs[index] = sig
	s.wg[index].Done()
	return nil
}

func (s *SignatureList) Wait(index int) {
	wg := &sync.WaitGroup{}
	s.lock.RLock()
	if index < len(s.wg) {
		wg = s.wg[index]
	}
	s.lock.RUnlock()

	wg.Wait()
}

type TxConfig struct {
	Timeout time.Duration // transaction broadcast timeout

	// compute unit price config
	FeeBumpPeriod        time.Duration // how often to bump fee
	BaseComputeUnitPrice uint64        // starting price
	ComputeUnitPriceMin  uint64        // min price
	ComputeUnitPriceMax  uint64        // max price

	EstimateComputeUnitLimit bool   // enable compute limit estimations using simulation
	ComputeUnitLimit         uint32 // compute unit limit

	DependencyTxMeta DependencyTxMeta // transaction IDs to wait for before broadcasting
}

type DependencyTxMeta struct {
	// List of transactions and their desired statuses this transaction is dependent on
	DependencyTxs []DependencyTx
	// Flag to ignore dependency errors. Used by clean up transactions that are normally expected to be dropped
	IgnoreDependencyError bool
}

type DependencyTx struct {
	// ID the transaction is dependent on
	TxID string
	// Desired status of the dependency transaction. Note: Failed and Fatal will be treated as the same.
	DesiredStatus commontypes.TransactionStatus
}

type SetTxConfig func(*TxConfig)

func SetTimeout(t time.Duration) SetTxConfig {
	return func(cfg *TxConfig) {
		cfg.Timeout = t
	}
}
func SetFeeBumpPeriod(t time.Duration) SetTxConfig {
	return func(cfg *TxConfig) {
		cfg.FeeBumpPeriod = t
	}
}
func SetBaseComputeUnitPrice(v uint64) SetTxConfig {
	return func(cfg *TxConfig) {
		cfg.BaseComputeUnitPrice = v
	}
}
func SetComputeUnitPriceMin(v uint64) SetTxConfig {
	return func(cfg *TxConfig) {
		cfg.ComputeUnitPriceMin = v
	}
}
func SetComputeUnitPriceMax(v uint64) SetTxConfig {
	return func(cfg *TxConfig) {
		cfg.ComputeUnitPriceMax = v
	}
}
func SetComputeUnitLimit(v uint32) SetTxConfig {
	return func(cfg *TxConfig) {
		cfg.ComputeUnitLimit = v
	}
}
func SetEstimateComputeUnitLimit(v bool) SetTxConfig {
	return func(cfg *TxConfig) {
		cfg.EstimateComputeUnitLimit = v
	}
}
func AppendDependencyTxs(v []DependencyTx) SetTxConfig {
	return func(cfg *TxConfig) {
		cfg.DependencyTxMeta.DependencyTxs = append(cfg.DependencyTxMeta.DependencyTxs, v...)
	}
}
func SetDependencyTxMetaIgnoreError(v bool) SetTxConfig {
	return func(cfg *TxConfig) {
		cfg.DependencyTxMeta.IgnoreDependencyError = v
	}
}
