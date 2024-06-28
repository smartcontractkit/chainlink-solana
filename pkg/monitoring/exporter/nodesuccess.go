package exporter

import (
	"context"

	"github.com/gagliardetto/solana-go"
	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/metrics"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
)

func NewNodeSuccessFactory(
	log commonMonitoring.Logger,
	metrics metrics.NodeSuccess,
) commonMonitoring.ExporterFactory {
	return &nodeSuccessFactory{
		log,
		metrics,
	}
}

type nodeSuccessFactory struct {
	log     commonMonitoring.Logger
	metrics metrics.NodeSuccess
}

func (p *nodeSuccessFactory) NewExporter(
	params commonMonitoring.ExporterParams,
) (commonMonitoring.Exporter, error) {
	nodes, err := config.MakeSolanaNodeConfigs(params.Nodes)
	if err != nil {
		return nil, err
	}

	nodesMap := map[solana.PublicKey]string{}
	for _, v := range nodes {
		pubkey, err := v.PublicKey()
		if err != nil {
			return nil, err
		}
		nodesMap[pubkey] = v.GetName()
	}

	return &nodeSuccess{
		metrics.FeedInput{
			AccountAddress: params.FeedConfig.GetContractAddress(),
			FeedID:         params.FeedConfig.GetContractAddress(),
			ChainID:        params.ChainConfig.GetChainID(),
			ContractStatus: params.FeedConfig.GetContractStatus(),
			ContractType:   params.FeedConfig.GetContractType(),
			FeedName:       params.FeedConfig.GetName(),
			FeedPath:       params.FeedConfig.GetPath(),
			NetworkID:      params.ChainConfig.GetNetworkID(),
			NetworkName:    params.ChainConfig.GetNetworkName(),
		},
		nodesMap,
		p.log,
		p.metrics,
	}, nil
}

type nodeSuccess struct {
	feedLabel metrics.FeedInput // static for each feed
	nodes     map[solana.PublicKey]string
	log       commonMonitoring.Logger
	metrics   metrics.NodeSuccess
}

func (p *nodeSuccess) Export(ctx context.Context, data interface{}) {
	details, err := types.MakeTxDetails(data)
	if err != nil {
		return // skip if input could not be parsed
	}

	// skip on no updates
	if len(details) == 0 {
		return
	}

	// calculate count
	count := map[solana.PublicKey]int{}
	for _, d := range details {
		count[d.Sender]++
	}

	for k, v := range count {
		name, isOperator := p.nodes[k]
		if !isOperator {
			p.log.Debugw("Sender does not match known operator", "sender", k)
			continue // skip if not known operator
		}

		p.metrics.Add(v, metrics.NodeFeedInput{
			NodeAddress:  k.String(),
			NodeOperator: name,
			FeedInput:    p.feedLabel,
		})
	}
}

func (p *nodeSuccess) Cleanup(_ context.Context) {
	for k, v := range p.nodes {
		p.metrics.Cleanup(metrics.NodeFeedInput{
			NodeAddress:  k.String(),
			NodeOperator: v,
			FeedInput:    p.feedLabel,
		})
	}
}
