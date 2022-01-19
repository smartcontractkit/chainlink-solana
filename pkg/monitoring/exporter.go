package monitoring

import (
	"context"
)

type Exporter interface {
	Export(ctx context.Context, data interface{})
}

/*
// ExporterFactory is a top-level object. Use this to pass global dependencies shared by all Exporters, eg. a logger or a kafka client.
// This factory will be used to produce a separate Exporter for each feed monitored.
type ExporterFactory interface {
	MakeExporter(chainConfig ChainConfig, feedConfig FeedConfig) (Exporter, error)
}
*/
