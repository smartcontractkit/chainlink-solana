package monitoring

import relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"

type Mapper func(interface{}, relayMonitoring.ChainConfig, relayMonitoring.FeedConfig) (map[string]interface{}, error)
