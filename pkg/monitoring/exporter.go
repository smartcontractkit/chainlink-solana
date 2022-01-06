package monitoring

import (
	"context"
)

type Exporter interface {
	Export(ctx context.Context, data interface{})
}
