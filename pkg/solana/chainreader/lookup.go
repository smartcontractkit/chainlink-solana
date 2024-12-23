package chainreader

import (
	"sync"

	"github.com/smartcontractkit/chainlink-common/pkg/types"
)

type readValues struct {
	address  string
	contract string
	name     namePair
}

type namePair struct {
	onChainName, genericName string
}

// lookup provides basic utilities for mapping a complete readIdentifier to
// finite contract read information
type lookup struct {
	mu sync.RWMutex
	// contractReadNames maps a contract name to all available readNames (method, log, event, etc.)
	contractReadNames map[string][]namePair
	// readIdentifiers maps from a complete readIdentifier string to finite read data
	// a readIdentifier is a combination of address, contract, and namePair as a concatenated string
	readIdentifiers map[string]readValues
}

func newLookup() *lookup {
	return &lookup{
		contractReadNames: make(map[string][]namePair),
		readIdentifiers:   make(map[string]readValues),
	}
}

func (l *lookup) addReadNameForContract(contract string, name namePair) {
	l.mu.Lock()
	defer l.mu.Unlock()

	readNames, exists := l.contractReadNames[contract]
	if !exists {
		readNames = []namePair{}
	}

	l.contractReadNames[contract] = append(readNames, name)
}

func (l *lookup) bindAddressForContract(contract, address string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, namePair := range l.contractReadNames[contract] {
		readIdentifier := types.BoundContract{
			Address: address,
			Name:    contract,
		}.ReadIdentifier(namePair.onChainName)

		l.readIdentifiers[readIdentifier] = readValues{
			address:  address,
			contract: contract,
			name:     namePair,
		}
	}
}

func (l *lookup) unbindAddressForContract(contract, address string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, namePair := range l.contractReadNames[contract] {
		readIdentifier := types.BoundContract{
			Address: address,
			Name:    contract,
		}.ReadIdentifier(namePair.onChainName)

		delete(l.readIdentifiers, readIdentifier)
	}
}

func (l *lookup) getContractForReadIdentifiers(readIdentifier string) (readValues, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	contract, ok := l.readIdentifiers[readIdentifier]

	return contract, ok
}
