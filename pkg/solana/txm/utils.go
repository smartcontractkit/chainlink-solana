package txm

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
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
	s.sigs[i], s.sigs[j] = s.sigs[j], s.sigs[i]
	s.res[i], s.res[j] = s.res[j], s.res[i]
}

func (s statuses) Less(i, j int) bool {
	return convertStatus(s.res[i]) > convertStatus(s.res[j]) // returns list with highest first
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

func convertStatus(res *rpc.SignatureStatusesResult) TxState {
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

type signatureList struct {
	sigs []solana.Signature
	lock sync.RWMutex
	wg   []*sync.WaitGroup
}

// internal function that should be called using the proper lock
func (s *signatureList) get(index int) (sig solana.Signature, err error) {
	if index >= len(s.sigs) {
		return sig, errors.New("invalid index")
	}
	return s.sigs[index], nil
}

func (s *signatureList) Get(index int) (sig solana.Signature, err error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.get(index)
}

func (s *signatureList) List() []solana.Signature {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.sigs
}

func (s *signatureList) Length() int {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return len(s.sigs)
}

func (s *signatureList) Allocate() (index int) {
	s.lock.Lock()
	defer s.lock.Unlock()

	var wg sync.WaitGroup
	wg.Add(1)

	s.sigs = append(s.sigs, solana.Signature{})
	s.wg = append(s.wg, &wg)

	return len(s.sigs) - 1
}

func (s *signatureList) Set(index int, sig solana.Signature) error {
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

func (s *signatureList) Wait(index int) {
	wg := &sync.WaitGroup{}
	s.lock.RLock()
	if index < len(s.wg) {
		wg = s.wg[index]
	}
	s.lock.RUnlock()

	wg.Wait()
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
