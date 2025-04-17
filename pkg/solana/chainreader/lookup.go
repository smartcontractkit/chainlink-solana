package chainreader

import (
	"sync"

	"github.com/smartcontractkit/chainlink-common/pkg/types"
)

type read struct {
	readName string
	// useParams is used when this read is part of a multi read to determine if it should use parent read params.
	useParams, errOnMissingAccountData bool
}

type readValues struct {
	address  string
	contract string
	// reads of size one is a regular Account read.
	// When multiple read reads are present, first read has type info that other sequential reads are filling out.
	// This works by having hard coder codec modifier define fields that are filled out by subsequent reads.
	reads []read
}

// lookup provides basic utilities for mapping a complete readIdentifier to
// finite contract read information
type lookup struct {
	mu sync.RWMutex
	// contractReadNames maps a program name to all available reads (accounts, PDAs, logs).
	// Every key (generic read name) can be composed of multiple reads of the same program. Right now all of them have to be of same type (account, PDA or log).
	contractReadNames map[string]map[string][]read
	// readIdentifiers maps from a complete readIdentifier string to finite read data
	// a readIdentifier is a combination of address, contract, and chainSpecificName as a concatenated string
	readIdentifiers map[string]readValues
}

func newLookup() *lookup {
	return &lookup{
		contractReadNames: make(map[string]map[string][]read),
		readIdentifiers:   make(map[string]readValues),
	}
}

func (l *lookup) addReadNameForContract(contract, genericName string, reads []read) {
	l.mu.Lock()
	defer l.mu.Unlock()

	readNames, exists := l.contractReadNames[contract]
	if !exists {
		readNames = make(map[string][]read)
	}

	readNames[genericName] = reads

	l.contractReadNames[contract] = readNames
}

func (l *lookup) bindAddressForContract(contract, address string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, reads := range l.contractReadNames[contract] {
		readIdentifier := ""
		if len(reads) > 0 {
			readIdentifier = types.BoundContract{
				Address: address,
				Name:    contract,
			}.ReadIdentifier(reads[0].readName)
		}

		l.readIdentifiers[readIdentifier] = readValues{
			address:  address,
			contract: contract,
			reads:    reads,
		}
	}
}

func (l *lookup) hasAddress(contract, address string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()

	for _, reads := range l.contractReadNames[contract] {
		readIdentifier := ""
		if len(reads) > 0 {
			readIdentifier = types.BoundContract{
				Address: address,
				Name:    contract,
			}.ReadIdentifier(reads[0].readName)
		}

		if val, ok := l.readIdentifiers[readIdentifier]; ok && val.address == address {
			return true
		}
	}

	return false
}

func (l *lookup) unbindAddressForContract(contract, address string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, reads := range l.contractReadNames[contract] {
		readIdentifier := ""
		if len(reads) > 0 {
			readIdentifier = types.BoundContract{
				Address: address,
				Name:    contract,
			}.ReadIdentifier(reads[0].readName)
		}

		delete(l.readIdentifiers, readIdentifier)
	}
}

func (l *lookup) getContractForReadIdentifiers(readIdentifier string) (readValues, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	contract, ok := l.readIdentifiers[readIdentifier]

	return contract, ok
}
