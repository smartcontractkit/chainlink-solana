package logpoller

var (
	logsFields   = [...]string{"id", "filter_id", "chain_id", "log_index", "block_hash", "block_number", "block_timestamp", "address", "event_sig", "subkey_values", "tx_hash", "data", "created_at", "expires_at", "sequence_num"}
	filterFields = [...]string{"id", "name", "address", "event_name", "event_sig", "starting_block", "event_idl", "subkey_paths", "retention", "max_logs_kept", "is_deleted", "is_backfilled"}
)
