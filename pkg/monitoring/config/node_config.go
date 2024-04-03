package config

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/smartcontractkit/libocr/offchainreporting2/types"

	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"
)

type SolanaNodeConfig struct {
	ID          string   `json:"id,omitempty"`
	NodeAddress []string `json:"nodeAddress,omitempty"`
}

func (s SolanaNodeConfig) GetName() string {
	return s.ID
}

func (s SolanaNodeConfig) GetAccount() types.Account {
	address := ""
	if len(s.NodeAddress) != 0 {
		address = s.NodeAddress[0]
	}
	return types.Account(address)
}

func SolanaNodesParser(buf io.ReadCloser) ([]commonMonitoring.NodeConfig, error) {
	rawNodes := []SolanaNodeConfig{}
	decoder := json.NewDecoder(buf)
	if err := decoder.Decode(&rawNodes); err != nil {
		return nil, fmt.Errorf("unable to unmarshal nodes config data: %w", err)
	}
	nodes := make([]commonMonitoring.NodeConfig, len(rawNodes))
	for i, rawNode := range rawNodes {
		nodes[i] = rawNode
	}
	return nodes, nil
}
