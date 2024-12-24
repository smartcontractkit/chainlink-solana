package chainreader

import (
	"sync"

	"github.com/smartcontractkit/chainlink-common/pkg/types"
)

type readValues struct {
	address     string
	contract    string
	genericName string
}

// lookup provides basic utilities for mapping a complete readIdentifier to
// finite contract read information
type lookup struct {
	mu sync.RWMutex
	// contractReadNames maps a contract name to all available namePairs (method, log, event, etc.)
	contractReadNames map[string][]string
	// readIdentifiers maps from a complete readIdentifier string to finite read data
	// a readIdentifier is a combination of address, contract, and chainSpecificName as a concatenated string
	readIdentifiers map[string]readValues
}

func newLookup() *lookup {
	return &lookup{
		contractReadNames: make(map[string][]string),
		readIdentifiers:   make(map[string]readValues),
	}
}

func (l *lookup) addReadNameForContract(contract string, genericName string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	readNames, exists := l.contractReadNames[contract]
	if !exists {
		readNames = []string{}
	}

	l.contractReadNames[contract] = append(readNames, genericName)
}

func (l *lookup) bindAddressForContract(contract, address string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, genericName := range l.contractReadNames[contract] {
		readIdentifier := types.BoundContract{
			Address: address,
			Name:    contract,
		}.ReadIdentifier(genericName)

		l.readIdentifiers[readIdentifier] = readValues{
			address:     address,
			contract:    contract,
			genericName: genericName,
		}
	}
}

func (l *lookup) unbindAddressForContract(contract, address string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, genericName := range l.contractReadNames[contract] {
		readIdentifier := types.BoundContract{
			Address: address,
			Name:    contract,
		}.ReadIdentifier(genericName)

		delete(l.readIdentifiers, readIdentifier)
	}
}

func (l *lookup) getContractForReadIdentifiers(readIdentifier string) (readValues, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	contract, ok := l.readIdentifiers[readIdentifier]

	return contract, ok
}
