package monitoring

import (
	"context"
	"sync"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/ws"
	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
)

type Ingestor struct {
	chainConfig SolanaConfig
	client      *ws.Client
	commitment  rpc.CommitmentType
	producer    relayMonitoring.Producer

	stateSchema         relayMonitoring.Schema
	transmissionsSchema relayMonitoring.Schema
	eventsSchema        relayMonitoring.Schema

	log relayMonitoring.Logger
}

func NewIngestor(
	chainConfig SolanaConfig,
	client *ws.Client,
	commitment rpc.CommitmentType,
	producer relayMonitoring.Producer,

	stateSchema relayMonitoring.Schema,
	transmissionsSchema relayMonitoring.Schema,
	eventsSchema relayMonitoring.Schema,

	log relayMonitoring.Logger,
) *Ingestor {
	return &Ingestor{
		chainConfig,
		client,
		commitment,
		producer,
		stateSchema,
		transmissionsSchema,
		eventsSchema,
		log,
	}
}

func (i *Ingestor) Run(ctx context.Context, feeds []relayMonitoring.FeedConfig) {
	pipelines := i.createPipelines(feeds)
	wg := &sync.WaitGroup{}
	defer wg.Wait()
	wg.Add(2 * len(pipelines))
	for _, pipeline := range pipelines {
		go func(pipeline ingestorPipeline) {
			defer wg.Done()
			pipeline.Updater.Run(ctx)
		}(pipeline)
		// TODO (dru) Do I need to make sure updaters exit before the rest of the pipeline?
		go func(pipeline ingestorPipeline) {
			defer wg.Done()
			for {
				select {
				case update := <-pipeline.Updater.Updates():
					i.handleUpdate(update, pipeline)
				case <-ctx.Done():
					return
				}
			}
		}(pipeline)
	}
}

type ingestorPipeline struct {
	Updater     relayMonitoring.Updater
	Decoder     Decoder
	Mapper      Mapper
	Schema      relayMonitoring.Schema
	Topic       string
	ChainConfig SolanaConfig
	FeedConfig  SolanaFeedConfig
}

func (i *Ingestor) createPipelines(rawFeeds []relayMonitoring.FeedConfig) []ingestorPipeline {
	pipelines := []ingestorPipeline{}
	feeds := convertToSolanaFeeds(rawFeeds)
	// Subscribe to logs emitted by the feed contract. Since in Solana, all feeds in the network
	// use the same contract, we only need one subscription, so one pipeline.
	if len(feeds) > 0 {
		pipelines = append(pipelines,
			ingestorPipeline{
				NewProgramLogsUpdater(
					feeds[0].ContractAddress,
					i.client,
					i.commitment,
					i.log.With("component", "updater-logs", "program", feeds[0].ContractAddressBase58),
				),
				LogResultDecode,
				LogMapper,
				i.eventsSchema,
				i.chainConfig.EventsKafkaTopic,
				i.chainConfig,
				feeds[0],
			},
		)
	}
	// Add pipelines for state and transmissions accounts for every feed.
	for _, feed := range feeds {
		pipelines = append(pipelines, ingestorPipeline{
			NewAccountUpdater(
				feed.StateAccount,
				i.client,
				i.commitment,
				i.log.With("component", "updater-state", "account", feed.StateAccountBase58),
			),
			StateResultDecoder,
			StateMapper,
			i.stateSchema,
			i.chainConfig.StateKafkaTopic,
			i.chainConfig,
			feed,
		})
		pipelines = append(pipelines, ingestorPipeline{
			NewAccountUpdater(
				feed.TransmissionsAccount,
				i.client,
				i.commitment,
				i.log.With("component", "updater-transmissions", "account", feed.TransmissionsAccountBase58),
			),
			TransmissionResultDecoder,
			TransmissionsMapper,
			i.transmissionsSchema,
			i.chainConfig.TransmissionsKafkaTopic,
			i.chainConfig,
			feed,
		})
	}
	return pipelines
}

func (i *Ingestor) handleUpdate(
	update interface{},
	pipeline ingestorPipeline,
) {
	decoded, err := pipeline.Decoder(update, pipeline.ChainConfig, pipeline.FeedConfig)
	if err != nil {
		i.log.Errorw("failed to decode update", "error", err)
		return
	}
	mapped, err := pipeline.Mapper(decoded, pipeline.ChainConfig, pipeline.FeedConfig)
	if err != nil {
		i.log.Errorw("failed to map update", "error", err)
		return
	}
	encoded, err := pipeline.Schema.Encode(mapped)
	if err != nil {
		i.log.Errorw("failed to encode as Avro", "error", err)
		return
	}
	key := pipeline.FeedConfig.StateAccount.Bytes()
	if err = i.producer.Produce(key, encoded, pipeline.Topic); err != nil {
		i.log.Errorw("failed to push update to kafka", "error", err, "topic", pipeline.Topic, "key", key)
	}
}

// Helpers

func convertToSolanaFeeds(feeds []relayMonitoring.FeedConfig) []SolanaFeedConfig {
	output := make([]SolanaFeedConfig, len(feeds))
	for i, feed := range feeds {
		if typed, ok := feed.(SolanaFeedConfig); ok {
			output[i] = typed
		}
	}
	return output
}
