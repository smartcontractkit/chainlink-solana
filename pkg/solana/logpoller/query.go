package logpoller

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/lib/pq"
)

// queryArgs is a helper for building the arguments to a postgres query created by DSORM
// Besides the convenience methods, it also keeps track of arguments validation and sanitization.
type queryArgs struct {
	args      map[string]any
	idxLookup map[string]uint8
	err       []error
}

func newQueryArgs(chainId string) *queryArgs {
	return &queryArgs{
		args: map[string]any{
			"chain_id": chainId,
		},
		idxLookup: make(map[string]uint8),
		err:       []error{},
	}
}

func (q *queryArgs) withField(fieldName string, value any) *queryArgs {
	_, args := q.withIndexableField(fieldName, value, false)

	return args
}

func (q *queryArgs) withIndexedField(fieldName string, value any) string {
	field, _ := q.withIndexableField(fieldName, value, true)

	return field
}

func (q *queryArgs) withIndexableField(fieldName string, value any, addIndex bool) (string, *queryArgs) {
	if addIndex {
		idx := q.nextIdx(fieldName)
		idxName := fmt.Sprintf("%s_%d", fieldName, idx)

		q.idxLookup[fieldName] = uint8(idx)
		fieldName = idxName
	}

	q.args[fieldName] = value

	return fieldName, q
}

func (q *queryArgs) nextIdx(baseFieldName string) int {
	idx, ok := q.idxLookup[baseFieldName]
	if !ok {
		return 0
	}

	return int(idx) + 1
}

// withName sets the Name field in queryArgs.
func (q *queryArgs) withName(name string) *queryArgs {
	return q.withField("name", name)
}

// withAddress sets the Address field in queryArgs.
func (q *queryArgs) withAddress(address PublicKey) *queryArgs {
	return q.withField("address", address)
}

// withEventName sets the EventName field in queryArgs.
func (q *queryArgs) withEventName(eventName string) *queryArgs {
	return q.withField("event_name", eventName)
}

// withEventSig sets the EventSig field in queryArgs.
func (q *queryArgs) withEventSig(eventSig []byte) *queryArgs {
	return q.withField("event_sig", eventSig)
}

// withStartingBlock sets the StartingBlock field in queryArgs.
func (q *queryArgs) withStartingBlock(startingBlock int64) *queryArgs {
	return q.withField("starting_block", startingBlock)
}

// withEventIDL sets the EventIDL field in queryArgs.
func (q *queryArgs) withEventIDL(eventIDL string) *queryArgs {
	return q.withField("event_idl", eventIDL)
}

// withSubKeyPaths sets the SubKeyPaths field in queryArgs.
func (q *queryArgs) withSubKeyPaths(subKeyPaths [][]string) *queryArgs {
	return q.withField("sub_key_paths", subKeyPaths)
}

// withRetention sets the Retention field in queryArgs.
func (q *queryArgs) withRetention(retention time.Duration) *queryArgs {
	return q.withField("retention", retention)
}

// withMaxLogsKept sets the MaxLogsKept field in queryArgs.
func (q *queryArgs) withMaxLogsKept(maxLogsKept int64) *queryArgs {
	return q.withField("max_logs_kept", maxLogsKept)
}

// withID sets the ID field in Log.
func (q *queryArgs) withID(id int64) *queryArgs {
	return q.withField("id", id)
}

// withFilterId sets the FilterId field in Log.
func (q *queryArgs) withFilterId(filterId int64) *queryArgs {
	return q.withField("filter_id", filterId)
}

// withChainId sets the ChainId field in Log.
func (q *queryArgs) withChainId(chainId string) *queryArgs {
	return q.withField("chain_id", chainId)
}

// withLogIndex sets the LogIndex field in Log.
func (q *queryArgs) withLogIndex(logIndex int64) *queryArgs {
	return q.withField("log_index", logIndex)
}

// withBlockHash sets the BlockHash field in Log.
func (q *queryArgs) withBlockHash(blockHash solana.Hash) *queryArgs {
	return q.withField("block_hash", blockHash)
}

// withBlockNumber sets the BlockNumber field in Log.
func (q *queryArgs) withBlockNumber(blockNumber int64) *queryArgs {
	return q.withField("block_number", blockNumber)
}

// withBlockTimestamp sets the BlockTimestamp field in Log.
func (q *queryArgs) withBlockTimestamp(blockTimestamp time.Time) *queryArgs {
	return q.withField("block_timestamp", blockTimestamp)
}

// withSubKeyValues sets the SubKeyValues field in Log.
func (q *queryArgs) withSubKeyValues(subKeyValues pq.ByteaArray) *queryArgs {
	return q.withField("sub_key_values", subKeyValues)
}

// withTxHash sets the TxHash field in Log.
func (q *queryArgs) withTxHash(txHash solana.Signature) *queryArgs {
	return q.withField("tx_hash", txHash)
}

// withData sets the Data field in Log.
func (q *queryArgs) withData(data []byte) *queryArgs {
	return q.withField("data", data)
}

// withCreatedAt sets the CreatedAt field in Log.
func (q *queryArgs) withCreatedAt(createdAt time.Time) *queryArgs {
	return q.withField("created_at", createdAt)
}

// withExpiresAt sets the ExpiresAt field in Log.
func (q *queryArgs) withExpiresAt(expiresAt *time.Time) *queryArgs {
	return q.withField("expires_at", expiresAt)
}

// withSequenceNum sets the SequenceNum field in Log.
func (q *queryArgs) withSequenceNum(sequenceNum int64) *queryArgs {
	return q.withField("sequence_num", sequenceNum)
}

func newQueryArgsForEvent(chainId string, address PublicKey, eventSig []byte) *queryArgs {
	return newQueryArgs(chainId).
		withAddress(address).
		withEventSig(eventSig)
}

func (q *queryArgs) withStartBlock(startBlock int64) *queryArgs {
	return q.withField("start_block", startBlock)
}

func (q *queryArgs) withEndBlock(endBlock int64) *queryArgs {
	return q.withField("end_block", endBlock)
}

func logsQuery(clause string) string {
	return fmt.Sprintf(`SELECT %s FROM solana.logs %s`, strings.Join(logsFields[:], ", "), clause)
}

func (q *queryArgs) toArgs() (map[string]any, error) {
	if len(q.err) > 0 {
		return nil, errors.Join(q.err...)
	}

	return q.args, nil
}
