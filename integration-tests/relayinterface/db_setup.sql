-- logpoller db table initialization for tests. loosely based on what's in chainlnk repo
CREATE SCHEMA solana;
CREATE TABLE solana.log_poller_filters ( id BIGINT,
    chain_id TEXT NOT NULL,
    name TEXT NOT NULL,
    address BYTEA NOT NULL,
    event_name TEXT NOT NULL,
    event_sig BYTEA NOT NULL,
    starting_block BIGINT NOT NULL,
    event_idl TEXT,
    subkey_paths json,
    retention BIGINT NOT NULL DEFAULT 0,
    max_logs_kept BIGINT NOT NULL DEFAULT 0,
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE solana.logs (
    id               BIGINT PRIMARY KEY,
    filter_id        BIGINT NOT NULL,
    chain_id         TEXT                     not null,
    log_index        bigint                    not null,
    block_hash       bytea                     not null,
    block_number     bigint                    not null CHECK (block_number > 0),
    block_timestamp  timestamp with time zone  not null,
    address          bytea                     not null,
    event_sig        bytea                     not null,
    subkey_values    bytea[]                   not null,
    tx_hash          bytea                     not null,
    data             bytea                     not null,
    created_at       timestamp with time zone  not null,
    expires_at       timestamp with time zone  null,
    sequence_num     bigint                    not null
);
ALTER TABLE solana.log_poller_filters ADD COLUMN is_backfilled BOOLEAN;
UPDATE solana.log_poller_filters SET is_backfilled = true;
ALTER TABLE solana.log_poller_filters ALTER COLUMN is_backfilled SET NOT NULL;
