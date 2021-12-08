package monitoring

/*
import (
	"math/big"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics interface {
	SetHeadTrackerCurrentHead(blockNumber uint64, networkName, chainID, networkID string)
	SetFeedContractMetadata(chainID, contractAddress, contractStatus, contractType, feedName, feedPath, networkID, networkName, symbol string)
	SetNodeMetadata(chainID, networkID, networkName, oracleName, sender string)
	SetOffchainAggregatorAnswers(answer *big.Int, contractAddress, chainID, contractStatus, contractType, feedName, feedPath, networkID, networkName string)
	IncOffchainAggregatorAnswersTotal(contractAddress, chainID, contractStatus, contractType, feedName, feedPath, networkID, networkName string)
	SetOffchainAggregatorSubmissionReceivedValues(value *big.Int, contractAddress, sender, chainID, contractStatus, contractType, feedName, feedPath, networkID, networkName string)
	SetOffchainAggregatorAnswerStalled(isSet bool, contractAddress, chainID, contractStatus, contractType, feedName, feedPath, networkID, networkName string)
}

var DefaultMetrics Metrics

// See the spec here: https://docs.google.com/document/d/1KRJlHmKXFp9sgfuflm4SDfK1PL8RSVLV-rJqNZVqvuU/edit#

var (
	headTrackerCurrentHead = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      "head_tracker_current_head",
			Namespace: "solana",
			Help:      "Tracks the current block height that the monitoring instance has processed.",
		},
		[]string{"network_name", "chain_id", "network_id"},
	)
	feedContractMetadata = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      "feed_contract_metadata",
			Namespace: "solana",
			Help:      "Exposes metadata for individual feeds. It should simply be set to 1, as the relevant info is in the labels.",
		},
		[]string{"chain_id", "contract_address", "contract_status", "contract_type", "feed_name", "feed_path", "network_id", "network_name", "symbol"},
	)
	// TODO (dru) currently there is no way of determining onchain which oracle reported a value.
	nodeMetadata = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      "node_metadata",
			Namespace: "solana",
			Help:      "Exposes metadata for node operators. It should simply be set to 1, as the relevant info is in the labels.",
		},
		[]string{"chain_id", "network_id", "network_name", "oracle_name", "sender"},
	)
	offchainAggregatorAnswers = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      "offchain_aggregator_answers",
			Namespace: "solana",
			Help:      "Reports the latest answer for a contract.",
		},
		[]string{"contract_address", "chain_id", "contract_status", "contract_type", "feed_name", "feed_path", "network_id", "network_name"},
	)
	offchainAggregatorAnswersTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      "offchain_aggregator_answers_total",
			Namespace: "solana",
			Help:      "Bump this metric every time there is a transmission on chain.",
		},
		[]string{"contract_address", "chain_id", "contract_status", "contract_type", "feed_name", "feed_path", "network_id", "networks_name"},
	)
	// TODO (dru) since we don't have access to the sender id, this metric becomes identical to offchainAggregatorAnswers.
	offchainAggregatorSubmissionReceivedValues = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      "offchain_aggregator_submission_received_values",
			Namespace: "solana",
			Help:      "Report individual node observations for the latest transmission on chain. (Should be 1 time series per node per contract)",
		},
		[]string{"contract_address", "sender", "chain_id", "contract_status", "contract_type", "feed_name", "feed_path", "network_id", "network_name"},
	)
	offchainAggregatorAnswerStalled = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      "offchain_aggregator_answer_stalled",
			Namespace: "Solana",
			Help:      "Set to 1 if the heartbeat interval has passed on a feed without a transmission. Set to 0 otherwise.",
		},
		[]string{"contract_address", "chain_id", "contract_status", "contract_type", "feed_name", "feed_path", "network_id", "network_name"},
	)
)

func init() {
	prometheus.MustRegister(headTrackerCurrentHead)
	prometheus.MustRegister(feedContractMetadata)
	prometheus.MustRegister(nodeMetadata)
	prometheus.MustRegister(offchainAggregatorAnswers)
	prometheus.MustRegister(offchainAggregatorAnswersTotal)
	prometheus.MustRegister(offchainAggregatorSubmissionReceivedValues)
	prometheus.MustRegister(offchainAggregatorAnswerStalled)

	DefaultMetrics = &defaultMetrics{}
}

type defaultMetrics struct{}

func (d *defaultMetrics) SetHeadTrackerCurrentHead(blockNumber uint64, networkName, chainID, networkID string) {
	headTrackerCurrentHead.WithLabelValues(networkName, chainID, networkID).Set(float64(blockNumber))
}

func (d *defaultMetrics) SetFeedContractMetadata(chainID, contractAddress, contractStatus, contractType, feedName, feedPath, networkID, networkName, symbol string) {
	feedContractMetadata.WithLabelValues(chainID, contractAddress, contractStatus, contractType, feedName, feedPath, networkID, networkName, symbol).Set(1)
}

func (d *defaultMetrics) SetNodeMetadata(chainID, networkID, networkName, oracleName, sender string) {
	nodeMetadata.WithLabelValues(chainID, networkID, networkName, oracleName, sender).Set(1)
}

func (d *defaultMetrics) SetOffchainAggregatorAnswers(answer *big.Int, contractAddress, chainID, contractStatus, contractType, feedName, feedPath, networkID, networkName string) {
	offchainAggregatorAnswers.WithLabelValues(contractAddress, chainID, contractStatus, contractType, feedName, feedPath, networkID, networkName).Set(float64(answer.Int64()))
}

func (d *defaultMetrics) IncOffchainAggregatorAnswersTotal(contractAddress, chainID, contractStatus, contractType, feedName, feedPath, networkID, networkName string) {
	offchainAggregatorAnswersTotal.WithLabelValues(contractAddress, chainID, contractStatus, contractType, feedName, feedPath, networkID, networkName).Inc()
}

func (d *defaultMetrics) SetOffchainAggregatorSubmissionReceivedValues(value *big.Int, contractAddress, sender, chainID, contractStatus, contractType, feedName, feedPath, networkID, networkName string) {
	offchainAggregatorSubmissionReceivedValues.WithLabelValues(contractAddress, sender, chainID, contractStatus, contractType, feedName, feedPath, networkID, networkName).Set(float64(value.Int64()))
}

func (d *defaultMetrics) SetOffchainAggregatorAnswerStalled(isSet bool, contractAddress, chainID, contractStatus, contractType, feedName, feedPath, networkID, networkName string) {
	var value float64 = 0
	if isSet {
		value = 1
	}
	offchainAggregatorAnswerStalled.WithLabelValues(contractAddress, chainID, contractStatus, contractType, feedName, feedPath, networkID, networkName).Set(value)
}
*/
