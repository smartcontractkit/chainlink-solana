package monitor

import (
	"strconv"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/smartcontractkit/chainlink-framework/metrics"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/internal"
)

var (
	// Deprecated: use github.com/smartcontractkit/chainlink-framework/metrics.AccountBalance instead.
	promSolanaBalance = promauto.NewGaugeVec(
		prometheus.GaugeOpts{Name: "solana_balance", Help: "Solana account balances"},
		[]string{"account", "chainID", "chainSet", "denomination"},
	)
	promCacheTimestamp = promauto.NewGaugeVec(
		prometheus.GaugeOpts{Name: "solana_cache_last_update_unix", Help: "Solana relayer cache last update timestamp"},
		[]string{"type", "chainID", "account"},
	)
	// Deprecated: Use github.com/smartcontractkit/chainlink-framework/metrics.RPCCallLatency instead.
	promClientReq = promauto.NewGaugeVec(
		prometheus.GaugeOpts{Name: "solana_client_latency_ms", Help: "Solana client request latency"},
		[]string{"request", "url"},
	)
)

func (b *balanceMonitor) updateProm(acc solana.PublicKey, lamports uint64) {
	v := internal.LamportsToSol(lamports) // convert from lamports to SOL
	metrics.NodeBalance.WithLabelValues(acc.String(), b.chainID, metrics.Solana).Set(v)
	promSolanaBalance.WithLabelValues(acc.String(), b.chainID, "solana", "SOL").Set(v)
}

func SetCacheTimestamp(t time.Time, cacheType, chainID, account string) {
	promCacheTimestamp.With(prometheus.Labels{
		"type":    cacheType,
		"chainID": chainID,
		"account": account,
	}).Set(float64(t.Unix()))
}

func SetClientLatency(chainID string, d time.Duration, request, url string, err error) {
	metrics.RPCCallLatency.WithLabelValues(
		metrics.Solana,
		chainID,
		url,
		"false",                        // is send only
		strconv.FormatBool(err == nil), // is successful
		request,                        // rpc call name
	).Observe(float64(d))

	// TODO: Remove deprecated metric
	promClientReq.With(prometheus.Labels{
		"request": request,
		"url":     url,
	}).Set(float64(d.Milliseconds()))
}
