package monitoring

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/ws"
	"github.com/smartcontractkit/chainlink-relay/pkg/logger"
	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
)

type Ingestor struct {
	chainConfig SolanaConfig
	client      *ws.Client
	commitment  rpc.CommitmentType
	producer    relayMonitoring.Producer

	stateSchema        relayMonitoring.Schema
	transmissionSchema relayMonitoring.Schema
	eventsSchema       relayMonitoring.Schema
	blockSchema        relayMonitoring.Schema

	metrics IngestorMetrics
	log     relayMonitoring.Logger
}

func NewIngestor(
	chainConfig SolanaConfig,
	client *ws.Client,
	commitment rpc.CommitmentType,
	producer relayMonitoring.Producer,

	stateSchema relayMonitoring.Schema,
	transmissionSchema relayMonitoring.Schema,
	eventsSchema relayMonitoring.Schema,
	blockSchema relayMonitoring.Schema,

	metrics IngestorMetrics,
	log relayMonitoring.Logger,
) *Ingestor {
	return &Ingestor{
		chainConfig,
		client,
		commitment,
		producer,
		stateSchema,
		transmissionSchema,
		eventsSchema,
		blockSchema,
		metrics,
		log,
	}
}

func (i *Ingestor) Run(ctx context.Context, data relayMonitoring.RDDData) {
	feeds := data.Feeds
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
					i.metrics.IncReceivedObjectFromChain(pipeline.Topic)
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
	pipelines = append(pipelines, ingestorPipeline{
		NewProgramLogsUpdater(
			feeds[0].ContractAddress,
			i.client,
			i.commitment,
			logger.With(i.log, "component", "updater-logs", "program", feeds[0].ContractAddressBase58),
		),
		LogResultDecode,
		LogMapper,
		i.eventsSchema,
		i.chainConfig.EventsKafkaTopic,
		i.chainConfig,
		// The only data needed by LogResultDecode is the aggregator
		// address (ie. ProgramAddress) which is the same for all feeds on Solana.
		SolanaFeedConfig{
			ContractAddressBase58: feeds[0].ContractAddressBase58,
			ContractAddress:       feeds[0].ContractAddress,
		},
	})
	// Subscribe to blocks which contain transactions mentioning the aggregator contract.
	pipelines = append(pipelines, ingestorPipeline{
		NewBlocksUpdater(
			feeds[0].ContractAddress,
			i.client,
			i.commitment,
			logger.With(i.log, "component", "updater-blocks", "program", feeds[0].ContractAddressBase58),
		),
		BlockResultDecode,
		BlockMapper,
		i.blockSchema,
		i.chainConfig.BlocksKafkaTopic,
		i.chainConfig,
		SolanaFeedConfig{}, // not needed
	})
	// Add pipelines for state and transmissions accounts for every feed.
	for _, feed := range feeds {
		pipelines = append(pipelines, ingestorPipeline{
			NewAccountUpdater(
				feed.StateAccount,
				i.client,
				i.commitment,
				logger.With(i.log, "component", "updater-state", "account", feed.StateAccountBase58),
			),
			StateResultDecoder,
			StateMapper,
			i.stateSchema,
			i.chainConfig.StatesKafkaTopic,
			i.chainConfig,
			feed,
		})
		pipelines = append(pipelines, ingestorPipeline{
			NewAccountUpdater(
				feed.TransmissionsAccount,
				i.client,
				i.commitment,
				logger.With(i.log, "component", "updater-transmissions", "account", feed.TransmissionsAccountBase58),
			),
			TransmissionResultDecoder,
			TransmissionsMapper,
			i.transmissionSchema,
			i.chainConfig.TransmissionsKafkaTopic,
			i.chainConfig,
			feed,
		})
	}
	return pipelines
}

var errNoResults = fmt.Errorf("no relevant results detected")

func (i *Ingestor) handleUpdate(
	update interface{},
	pipeline ingestorPipeline,
) {
	decoded, err := pipeline.Decoder(update, pipeline.ChainConfig, pipeline.FeedConfig)
	if errors.Is(err, errNoResults) {
		return
	}
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
