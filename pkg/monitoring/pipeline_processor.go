package monitoring

import (
	"context"
	"sync"

	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
	monitoringConfig "github.com/smartcontractkit/chainlink-relay/pkg/monitoring/config"
)

type IngesterPipeline struct {
	Subscriber  Subscriber
	Mapper      Mapper
	Schema      relayMonitoring.Schema
	Topic       string
	ChainConfig relayMonitoring.ChainConfig
	FeedConfig  relayMonitoring.FeedConfig
}

type PipelineProcessor struct {
	chainConfig relayMonitoring.ChainConfig
	kafkaConfig monitoringConfig.Kafka
	producer    relayMonitoring.Producer
	factory     SubscriberFactory
	log         relayMonitoring.Logger
}

func NewPipelineProcessor(
	chainConfig relayMonitoring.ChainConfig,
	kafkaConfig monitoringConfig.Kafka,
	producer relayMonitoring.Producer,
	factory SubscriberFactory,
	log relayMonitoring.Logger,
) *PipelineProcessor {
	return &PipelineProcessor{
		chainConfig,
		kafkaConfig,
		producer,
		factory,
		log,
	}
}

func (p *PipelineProcessor) Run(ctx context.Context, feeds []SolanaFeedConfig) {
	pipelines := p.createPipelines(feeds)
	wg := &sync.WaitGroup{}
	wg.Add(2 * len(pipelines))
	defer wg.Wait()
	for _, pipeline := range pipelines {
		go func() {
			defer wg.Done()
			pipeline.Subscriber.Run(ctx)
		}()
		// TODO (dru) Do I need to make sure subscribers exit before the rest of the pipeline?
		go func(pipeline IngesterPipeline) {
			defer wg.Done()
			for {
				select {
				case update := <-pipeline.Subscriber.Updates():
					p.handleUpdate(update, pipeline)
				case <-ctx.Done():
					return
				}
			}
		}(pipeline)
	}
}

func (p *PipelineProcessor) handleUpdate(
	update interface{},
	pipeline IngesterPipeline,
) {
	mapped, err := pipeline.Mapper(update, pipeline.ChainConfig, pipeline.FeedConfig)
	if err != nil {
		p.log.Errorw("failed to map update", "error", err)
		continue
	}
	encoded, err := pipeline.Schema.Encode(mapped)
	if err != nil {
		p.log.Errorw("failed to Avro-encode the udpate", "error", err)
	}
	key := pipeline.FeedConfig.ContractAddress.Bytes()
	err := p.producer.Produce(key, pipeline.Topic, encoded)
	if err != nil {
		p.log.Errorw("failed to push update to kafka", "error", err, "topic", pipeline.Topic, "key", pipeline.Key)
	}
}

func (p *PipelineProcessor) createPipelines(feeds []SolanaFeedConfig) []IngesterPipeline {
	pipelines := []IngesterPipeline{}
	for _, feed := range feeds {
		pipelines = append(pipelines, IngesterPipeline{
			p.factory.NewAccountSubscriber(feed.StateAccount),
			StateAccountMapper,
			SolanaStateSchema,
			p.kafkaConfig.SolanaStatesTopic,
			p.chainConfig,
			feed,
		}, IngesterPipeline{
			p.factory.NewAccountSubscriber(feed.TransmissionsAccount),
			TransmissionsAccountMapper,
			SolanaTransmissionsSchema,
			p.kafkaConfig.SolanaTransmissionsTopic,
			p.chainConfig,
			feed,
		}, IngesterPipeline{
			p.factory.NewLogsSubscriber(feed.ContractAddress),
			LogsMapper,
			SolanaLogsSchema,
			p.kafkaConfig.SolanaLogsTopic,
			p.chainConfig,
			feed,
		})
	}
	return pipelines
}
