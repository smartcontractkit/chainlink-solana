package logpoller

import (
	"context"
	"errors"
	"fmt"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/sqlutil"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query"
)

var _ ORM = (*DSORM)(nil)

type DSORM struct {
	chainID string
	ds      sqlutil.DataSource
	lggr    logger.Logger
}

// NewORM creates an DSORM scoped to chainID.
func NewORM(chainID string, ds sqlutil.DataSource, lggr logger.Logger) *DSORM {
	return &DSORM{
		chainID: chainID,
		ds:      ds,
		lggr:    lggr,
	}
}

func (o *DSORM) Transact(ctx context.Context, fn func(*DSORM) error) (err error) {
	return sqlutil.Transact(ctx, o.new, o.ds, nil, fn)
}

// new returns a NewORM like o, but backed by ds.
func (o *DSORM) new(ds sqlutil.DataSource) *DSORM { return NewORM(o.chainID, ds, o.lggr) }

// InsertFilter is idempotent.
//
// Each address/event pair must have a unique job id, so it may be removed when the job is deleted.
// Returns ID for updated or newly inserted filter.
func (o *DSORM) InsertFilter(ctx context.Context, filter Filter) (id int64, err error) {
	args, err := newQueryArgs(o.chainID).
		withField("name", filter.Name).
		withRetention(filter.Retention).
		withMaxLogsKept(filter.MaxLogsKept).
		withName(filter.Name).
		withAddress(filter.Address).
		withEventName(filter.EventName).
		withEventSig(filter.EventSig).
		withStartingBlock(filter.StartingBlock).
		withEventIDL(filter.EventIDL).
		withSubkeyPaths(filter.SubkeyPaths).
		withIsBackfilled(filter.IsBackfilled).
		toArgs()
	if err != nil {
		return 0, err
	}

	// '::' has to be escaped in the query string
	// https://github.com/jmoiron/sqlx/issues/91, https://github.com/jmoiron/sqlx/issues/428
	query := `
		INSERT INTO solana.log_poller_filters
		    (chain_id, name, address, event_name, event_sig, starting_block, event_idl, subkey_paths, retention, max_logs_kept, is_backfilled)
	  		VALUES (:chain_id, :name, :address, :event_name, :event_sig, :starting_block, :event_idl, :subkey_paths, :retention, :max_logs_kept, :is_backfilled)
	  	ON CONFLICT (chain_id, name) WHERE NOT is_deleted DO UPDATE SET 
	  	                                                        event_name = EXCLUDED.event_name,
	  	                                                        starting_block = EXCLUDED.starting_block,
	  	                                                        retention = EXCLUDED.retention,
	  	                                                        max_logs_kept = EXCLUDED.max_logs_kept,
	  	                                                        is_backfilled = EXCLUDED.is_backfilled
		RETURNING id;`

	query, sqlArgs, err := o.ds.BindNamed(query, args)
	if err != nil {
		return 0, err
	}
	if err = o.ds.GetContext(ctx, &id, query, sqlArgs...); err != nil {
		return 0, err
	}
	return id, nil
}

// GetFilterByID returns filter by ID
func (o *DSORM) GetFilterByID(ctx context.Context, id int64) (Filter, error) {
	query := filtersQuery("WHERE id = $1")
	var result Filter
	err := o.ds.GetContext(ctx, &result, query, id)
	return result, err
}

func (o *DSORM) MarkFilterDeleted(ctx context.Context, id int64) (err error) {
	query := `UPDATE solana.log_poller_filters SET is_deleted = true WHERE id = $1`
	_, err = o.ds.ExecContext(ctx, query, id)
	return err
}

func (o *DSORM) MarkFilterBackfilled(ctx context.Context, id int64) (err error) {
	query := `UPDATE solana.log_poller_filters SET is_backfilled = true WHERE id = $1`
	_, err = o.ds.ExecContext(ctx, query, id)
	return err
}

func (o *DSORM) DeleteFilter(ctx context.Context, id int64) (err error) {
	query := `DELETE FROM solana.log_poller_filters WHERE id = $1`
	_, err = o.ds.ExecContext(ctx, query, id)
	return err
}

func (o *DSORM) DeleteFilters(ctx context.Context, filters map[int64]Filter) error {
	for _, filter := range filters {
		err := o.DeleteFilter(ctx, filter.ID)
		if err != nil {
			return fmt.Errorf("error deleting filter %s (%d): %w", filter.Name, filter.ID, err)
		}
	}

	return nil
}

func (o *DSORM) SelectFilters(ctx context.Context) ([]Filter, error) {
	query := filtersQuery("WHERE chain_id = $1")
	var filters []Filter
	err := o.ds.SelectContext(ctx, &filters, query, o.chainID)
	return filters, err
}

// InsertLogs is idempotent to support replays.
func (o *DSORM) InsertLogs(ctx context.Context, logs []Log) error {
	if err := o.validateLogs(logs); err != nil {
		return err
	}
	return o.Transact(ctx, func(orm *DSORM) error {
		return orm.insertLogsWithinTx(ctx, logs, orm.ds)
	})
}

func (o *DSORM) insertLogsWithinTx(ctx context.Context, logs []Log, tx sqlutil.DataSource) error {
	batchInsertSize := 4000
	for i := 0; i < len(logs); i += batchInsertSize {
		start, end := i, i+batchInsertSize
		if end > len(logs) {
			end = len(logs)
		}

		query := `INSERT INTO solana.logs
					(filter_id, chain_id, log_index, block_hash, block_number, block_timestamp, address, event_sig, subkey_values, tx_hash, data, created_at, expires_at, sequence_num)
				VALUES
					(:filter_id, :chain_id, :log_index, :block_hash, :block_number, :block_timestamp, :address, :event_sig, :subkey_values, :tx_hash, :data, NOW(), :expires_at, :sequence_num)
				ON CONFLICT DO NOTHING`

		_, err := tx.NamedExecContext(ctx, query, logs[start:end])
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) && batchInsertSize > 500 {
				// In case of DB timeouts, try to insert again with a smaller batch upto a limit
				batchInsertSize /= 2
				i -= batchInsertSize // counteract +=batchInsertSize on next loop iteration
				continue
			}
			return err
		}
	}
	return nil
}

func (o *DSORM) validateLogs(logs []Log) error {
	for _, log := range logs {
		if o.chainID != log.ChainID {
			return fmt.Errorf("invalid chainID in log got %v want %v", log.ChainID, o.chainID)
		}
	}
	return nil
}

// SelectLogs finds the logs in a given block range.
func (o *DSORM) SelectLogs(ctx context.Context, start, end int64, address PublicKey, eventSig EventSignature) ([]Log, error) {
	args, err := newQueryArgsForEvent(o.chainID, address, eventSig).
		withStartBlock(start).
		withEndBlock(end).
		toArgs()
	if err != nil {
		return nil, err
	}

	query := logsQuery(`
		WHERE chain_id = :chain_id
		AND address = :address
		AND event_sig = :event_sig
		AND block_number >= :start_block
		AND block_number <= :end_block
		ORDER BY block_number, log_index`)

	var logs []Log
	query, sqlArgs, err := o.ds.BindNamed(query, args)
	if err != nil {
		return nil, err
	}

	err = o.ds.SelectContext(ctx, &logs, query, sqlArgs...)
	if err != nil {
		return nil, err
	}
	return logs, nil
}

func (o *DSORM) FilteredLogs(ctx context.Context, filter []query.Expression, limitAndSort query.LimitAndSort, _ string) ([]Log, error) {
	qs, args, err := (&pgDSLParser{}).buildQuery(o.chainID, filter, limitAndSort)
	if err != nil {
		return nil, err
	}

	values, err := args.toArgs()
	if err != nil {
		return nil, err
	}

	query, sqlArgs, err := o.ds.BindNamed(qs, values)
	if err != nil {
		return nil, err
	}

	var logs []Log
	if err = o.ds.SelectContext(ctx, &logs, query, sqlArgs...); err != nil {
		return nil, err
	}

	return logs, nil
}
