package logpoller

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/sqlutil"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
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

func (o *DSORM) ChainID() string {
	return o.chainID
}

func (o *DSORM) Transact(ctx context.Context, fn func(*DSORM) error) (err error) {
	return sqlutil.Transact(ctx, o.new, o.ds, nil, fn)
}

// new returns a NewORM like o, but backed by ds.
func (o *DSORM) new(ds sqlutil.DataSource) *DSORM { return NewORM(o.chainID, ds, o.lggr) }

func (o *DSORM) HasFilter(ctx context.Context, name string) (bool, error) {
	args, err := newQueryArgs(o.chainID).withField("name", name).toArgs()
	if err != nil {
		return false, err
	}

	query := `
		SELECT id FROM solana.log_poller_filters
			WHERE is_deleted = false AND chain_id = :chain_id AND name = :name LIMIT 1`

	query, sqlArgs, err := o.ds.BindNamed(query, args)
	if err != nil {
		return false, err
	}

	var id int64
	if err = o.ds.GetContext(ctx, &id, query, sqlArgs...); err != nil {
		return false, err
	}

	return id >= 0, nil
}

// InsertFilter is idempotent.
//
// Each address/event pair must have a unique job id, so it may be removed when the job is deleted.
// Returns ID for updated or newly inserted filter.
func (o *DSORM) InsertFilter(ctx context.Context, filter types.Filter) (id int64, err error) {
	args, err := newQueryArgs(o.chainID).
		withField("name", filter.Name).
		withRetention(filter.Retention).
		withMaxLogsKept(filter.MaxLogsKept).
		withName(filter.Name).
		withAddress(filter.Address).
		withEventName(filter.EventName).
		withEventSig(filter.EventSig).
		withStartingBlock(filter.StartingBlock).
		withEventIDL(filter.EventIdl).
		withSubKeyPaths(filter.SubkeyPaths).
		withIsBackfilled(filter.IsBackfilled).
		withIncludeReverted(filter.IncludeReverted).
		withExtraFilterConfig(filter.ExtraFilterConfig).
		toArgs()
	if err != nil {
		return 0, err
	}

	// '::' has to be escaped in the query string
	// https://github.com/jmoiron/sqlx/issues/91, https://github.com/jmoiron/sqlx/issues/428
	query := `
		INSERT INTO solana.log_poller_filters
		    (chain_id, name, address, event_name, event_sig, starting_block, event_idl, subkey_paths, retention, max_logs_kept, is_backfilled, include_reverted, extra_filter_config)
			VALUES (:chain_id, :name, :address, :event_name, :event_sig, :starting_block, :event_idl, :subkey_paths, :retention, :max_logs_kept, :is_backfilled, :include_reverted, :extra_filter_config)
	  	ON CONFLICT (chain_id, name) WHERE NOT is_deleted DO UPDATE SET 
	  	                                                        event_name = EXCLUDED.event_name,
	  	                                                        starting_block = EXCLUDED.starting_block,
	  	                                                        retention = EXCLUDED.retention,
	  	                                                        max_logs_kept = EXCLUDED.max_logs_kept,
	  	                                                        is_backfilled = EXCLUDED.is_backfilled,
	  	                                                        include_reverted = EXCLUDED.include_reverted,
	  	                                                        extra_filter_config = EXCLUDED.extra_filter_config
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
func (o *DSORM) GetFilterByID(ctx context.Context, id int64) (types.Filter, error) {
	query := filtersQuery("WHERE id = $1")
	var result types.Filter
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

func (o *DSORM) DeleteFilters(ctx context.Context, filters map[int64]types.Filter) error {
	for _, filter := range filters {
		err := o.DeleteFilter(ctx, filter.ID)
		if err != nil {
			return fmt.Errorf("error deleting filter %s (%d): %w", filter.Name, filter.ID, err)
		}
	}

	return nil
}

func (o *DSORM) SelectFilters(ctx context.Context) ([]types.Filter, error) {
	query := filtersQuery("WHERE chain_id = $1")
	var filters []types.Filter
	err := o.ds.SelectContext(ctx, &filters, query, o.chainID)
	return filters, err
}

// InsertLogs is idempotent to support replays.
func (o *DSORM) InsertLogs(ctx context.Context, logs []types.Log) error {
	if err := o.validateLogs(logs); err != nil {
		return err
	}
	return o.Transact(ctx, func(orm *DSORM) error {
		return orm.insertLogsWithinTx(ctx, logs, orm.ds)
	})
}

func (o *DSORM) insertLogsWithinTx(ctx context.Context, logs []types.Log, tx sqlutil.DataSource) error {
	batchInsertSize := 4000
	for i := 0; i < len(logs); i += batchInsertSize {
		start, end := i, i+batchInsertSize
		if end > len(logs) {
			end = len(logs)
		}

		query := `INSERT INTO solana.logs
					(filter_id, chain_id, log_index, block_hash, block_number, block_timestamp, address, event_sig, subkey_values, tx_hash, data, created_at, expires_at, sequence_num, error)
				VALUES
					(:filter_id, :chain_id, :log_index, :block_hash, :block_number, :block_timestamp, :address, :event_sig, :subkey_values, :tx_hash, :data, NOW(), :expires_at, :sequence_num, :error)
				ON CONFLICT DO NOTHING`

		res, err := tx.NamedExecContext(ctx, query, logs[start:end])
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) && batchInsertSize > 500 {
				// In case of DB timeouts, try to insert again with a smaller batch upto a limit
				batchInsertSize /= 2
				i -= batchInsertSize // counteract +=batchInsertSize on next loop iteration
				continue
			}
			return err
		}
		numRows, err := res.RowsAffected()
		if err == nil {
			if numRows != int64(len(logs)) {
				// This probably just means we're trying to insert the same log twice, but could also be an indication
				// of other constraint violations
				o.lggr.Debugf("attempted to insert %d logs, but could only insert %d", end-start, numRows)
			}
		}
	}
	return nil
}

func (o *DSORM) validateLogs(logs []types.Log) error {
	for _, log := range logs {
		if o.chainID != log.ChainID {
			return fmt.Errorf("invalid chainID in log got %v want %v", log.ChainID, o.chainID)
		}
	}
	return nil
}

// SelectLogs finds the logs in a given block range.
func (o *DSORM) SelectLogs(ctx context.Context, start, end int64, address types.PublicKey, eventSig types.EventSignature) ([]types.Log, error) {
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

	var logs []types.Log
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

func (o *DSORM) FilteredLogs(ctx context.Context, filter []query.Expression, limitAndSort query.LimitAndSort, _ string) ([]types.Log, error) {
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

	var logs []types.Log
	if err = o.ds.SelectContext(ctx, &logs, query, sqlArgs...); err != nil {
		return nil, err
	}

	// We want each log returned to have a unique (BlockNumber, LogIndex)
	// There can be duplicates if more than one filter is tracking the same log events.
	// Keeping both and deduping here greatly simplifies log pruning & retention management
	type Key struct {
		blockNumber int64
		logIndex    int64
	}

	seen := make(map[Key]struct{}, len(logs))
	res := make([]types.Log, 0, len(logs))
	for _, log := range logs {
		key := Key{log.BlockNumber, log.LogIndex}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		res = append(res, log)
	}
	return res, nil
}

func (o *DSORM) GetLatestBlock(ctx context.Context) (int64, error) {
	q := `SELECT block_number FROM solana.logs WHERE chain_id = $1 ORDER BY block_number DESC LIMIT 1`
	var result int64
	err := o.ds.GetContext(ctx, &result, q, o.chainID)
	return result, err
}

func (o *DSORM) SelectSeqNums(ctx context.Context) (map[int64]int64, error) {
	results := make([]struct {
		FilterID    int64
		SequenceNum int64
	}, 0)
	query := "SELECT filter_id, MAX(sequence_num) AS sequence_num FROM solana.logs WHERE chain_id=$1 GROUP BY filter_id"
	err := o.ds.SelectContext(ctx, &results, query, o.chainID)
	if err != nil {
		return nil, err
	}
	seqNums := make(map[int64]int64)
	for _, row := range results {
		seqNums[row.FilterID] = row.SequenceNum
	}
	return seqNums, nil
}

func (o *DSORM) PruneLogsForFilter(ctx context.Context, filter types.Filter) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	query := `DELETE FROM solana.logs AS l
		  		 WHERE chain_id = $1 AND filter_id = $2 AND
		  		       ( l.expires_at <= NOW() OR $3 > 0 AND
		  		        	( SELECT MAX(sequence_num) FROM solana.logs
		  		    			WHERE chain_id = $1 AND filter_id = $2
							) - l.sequence_num >= $3
					   )`
	res, err := o.ds.ExecContext(ctx, query, o.chainID, filter.ID, filter.MaxLogsKept)
	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}
