package testing

import (
	"context"

	"github.com/smartcontractkit/chainlink-common/pkg/sqlutil"
)

type TestDSORM struct {
	ds sqlutil.DataSource
}

// NewTestORM creates a test DSORM which contains method only used by tests
func NewTestORM(ds sqlutil.DataSource) *TestDSORM {
	return &TestDSORM{
		ds: ds,
	}
}

// HasFilterByEventName checks if a filter exists for the provided event name
func (o *TestDSORM) HasFilterByEventName(ctx context.Context, chainID, eventName string, address []byte) (bool, error) {
	query := `
		SELECT COUNT(1) FROM solana.log_poller_filters 
			WHERE is_deleted = false AND chain_id = $1 AND event_name = $2 AND address = $3 LIMIT 1`

	var exists int
	if err := o.ds.GetContext(ctx, &exists, query, chainID, eventName, address); err != nil {
		return false, err
	}

	return exists != 0, nil
}
