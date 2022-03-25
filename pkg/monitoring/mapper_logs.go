package monitoring

import relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"

type LogsMapper func(
	update interface{},
	chainConfig relayMonitoring.ChainConfig,
	feedConfig relayMonitoring.FeedConfig,
) map[string]interface{}
