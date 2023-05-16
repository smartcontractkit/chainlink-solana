package txm

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

// tx not found
// < tx processed
// < tx confirmed/finalized + revert
// < tx confirmed/finalized + success
const (
	NotFound = iota
	Processed
	ConfirmedRevert
	ConfirmedSuccess
)

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

func convertStatus(res *rpc.SignatureStatusesResult) uint {
	if res == nil {
		return NotFound
	}

	if res.ConfirmationStatus == rpc.ConfirmationStatusProcessed {
		return Processed
	}

	if res.ConfirmationStatus == rpc.ConfirmationStatusConfirmed ||
		res.ConfirmationStatus == rpc.ConfirmationStatusFinalized {
		if res.Err != nil {
			return ConfirmedRevert
		}
		return ConfirmedSuccess
	}

	return NotFound
}

type signatureList struct {
	sigs []solana.Signature
	lock sync.RWMutex
	wg   []*sync.WaitGroup
}

func (s *signatureList) Get(index int) (sig solana.Signature, err error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	if index >= len(s.sigs) {
		return sig, errors.New("invalid index")
	}
	return s.sigs[index], nil
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
	v, err := s.Get(index)
	if err != nil {
		return err
	}

	if !v.IsZero() {
		return fmt.Errorf("trying to set signature when already set - index: %d, existing: %s, new: %s", index, v, sig)
	}

	s.lock.Lock()
	defer s.lock.Unlock()
	s.sigs[index] = sig
	s.wg[index].Done()
	return nil
}

func (s *signatureList) Wait(index int) {
	if index < len(s.wg) {
		s.wg[index].Wait()
	}
}
