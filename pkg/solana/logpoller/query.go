package logpoller

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// queryArgs is a helper for building the arguments to a postgres query created by DSORM
// Besides the convenience methods, it also keeps track of arguments validation and sanitization.
type queryArgs struct {
	args      map[string]any
	idxLookup map[string]uint8
	err       []error
}

func newQueryArgs(chainID string) *queryArgs {
	return &queryArgs{
		args: map[string]any{
			"chain_id": chainID,
		},
		idxLookup: make(map[string]uint8),
		err:       []error{},
	}
}

func (q *queryArgs) withField(fieldName string, value any) *queryArgs {
	_, args := q.withIndexableField(fieldName, value, false)

	return args
}

func (q *queryArgs) withIndexableField(fieldName string, value any, addIndex bool) (string, *queryArgs) {
	if addIndex {
		idx := q.nextIdx(fieldName)
		idxName := fmt.Sprintf("%s_%d", fieldName, idx)

		q.idxLookup[fieldName] = idx
		fieldName = idxName
	}

	q.args[fieldName] = value

	return fieldName, q
}

func (q *queryArgs) nextIdx(baseFieldName string) uint8 {
	idx, ok := q.idxLookup[baseFieldName]
	if !ok {
		return 0
	}

	return idx + 1
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

// withSubkeyPaths sets the SubkeyPaths field in queryArgs.
func (q *queryArgs) withSubkeyPaths(subkeyPaths [][]string) *queryArgs {
	return q.withField("subkey_paths", subkeyPaths)
}

// withRetention sets the Retention field in queryArgs.
func (q *queryArgs) withRetention(retention time.Duration) *queryArgs {
	return q.withField("retention", retention)
}

// withMaxLogsKept sets the MaxLogsKept field in queryArgs.
func (q *queryArgs) withMaxLogsKept(maxLogsKept int64) *queryArgs {
	return q.withField("max_logs_kept", maxLogsKept)
}

func newQueryArgsForEvent(chainID string, address PublicKey, eventSig []byte) *queryArgs {
	return newQueryArgs(chainID).
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
