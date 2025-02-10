package chainreader

import (
	"sync"

	"github.com/smartcontractkit/chainlink-common/pkg/types"
)

type readValues struct {
	address  string
	contract string
	// First read in multi read has type info that other sequential reads are filling out.
	// this works by having hard coder codec modifier define fields that are filled out by subsequent reads.
	multiRead []string
}

// lookup provides basic utilities for mapping a complete readIdentifier to
// finite contract read information
type lookup struct {
	mu sync.RWMutex
	// contractReadNames maps a program name to all available reads (accounts, PDAs, logs).
	// Every key (generic read name) can be composed of multiple reads of the same program. Right now all of them have to be of same type (account, PDA or log).
	contractReadNames map[string]map[string][]string
	// readIdentifiers maps from a complete readIdentifier string to finite read data
	// a readIdentifier is a combination of address, contract, and chainSpecificName as a concatenated string
	readIdentifiers map[string]readValues
}

func newLookup() *lookup {
	return &lookup{
		contractReadNames: make(map[string]map[string][]string),
		readIdentifiers:   make(map[string]readValues),
	}
}

func (l *lookup) addReadNameForContract(contract, genericName string, multiRead []string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	readNames, exists := l.contractReadNames[contract]
	if !exists {
		readNames = make(map[string][]string)
	}

	readNames[genericName] = multiRead

	l.contractReadNames[contract] = readNames
}

func (l *lookup) bindAddressForContract(contract, address string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, multiRead := range l.contractReadNames[contract] {
		readIdentifier := ""
		if len(multiRead) > 0 {
			readIdentifier = types.BoundContract{
				Address: address,
				Name:    contract,
			}.ReadIdentifier(multiRead[0])
		}

		l.readIdentifiers[readIdentifier] = readValues{
			address:   address,
			contract:  contract,
			multiRead: multiRead,
		}
	}
}

func (l *lookup) unbindAddressForContract(contract, address string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, multiRead := range l.contractReadNames[contract] {
		readIdentifier := ""
		if len(multiRead) > 0 {
			readIdentifier = types.BoundContract{
				Address: address,
				Name:    contract,
			}.ReadIdentifier(multiRead[0])
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
