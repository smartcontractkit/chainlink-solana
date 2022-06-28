package ingestor

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// Subscriptions
	receivedObjectFromChain = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "received_object_from_chain",
			Help: "counts the number of objects of different types received from the chain via subscription or polling",
		},
		[]string{"topic", "network_name", "network_id", "chain_id"},
	)
)

type IngestorMetrics interface {
	IncReceivedObjectFromChain(topic string)

	// Exposes the accumulated metrics to HTTP.
	HTTPHandler() http.Handler
}

func NewIngestorMetrics(chainConfig SolanaConfig) IngestorMetrics {
	return &ingestorMetrics{chainConfig}
}

type ingestorMetrics struct {
	chainConfig SolanaConfig
}

func (i *ingestorMetrics) IncReceivedObjectFromChain(topic string) {
	receivedObjectFromChain.With(prometheus.Labels{
		"topic":        topic,
		"network_name": i.chainConfig.GetNetworkName(),
		"network_id":   i.chainConfig.GetNetworkID(),
		"chain_id":     i.chainConfig.GetChainID(),
	}).Inc()
}

func (i *ingestorMetrics) HTTPHandler() http.Handler {
	return promhttp.Handler()
}
