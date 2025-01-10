package logpoller

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-common/pkg/types/query"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query/primitives"
)

const (
	blockFieldName     = "block_number"
	chainIDFieldName   = "chain_id"
	timestampFieldName = "block_timestamp"
	txHashFieldName    = "tx_hash"
	addressFieldName   = "address"
	eventSigFieldName  = "event_sig"
	defaultSort        = "block_number ASC, log_index ASC"
)

var (
	ErrInvalidComparator   = errors.New("invalid comparison operator")
	ErrInvalidConfidence   = errors.New("invalid confidence level; solana only supports finalized")
	ErrInvalidCursorDir    = errors.New("invalid cursor direction")
	ErrInvalidCursorFormat = errors.New("invalid cursor format")
	ErrInvalidSortDir      = errors.New("invalid sort direction")
	ErrInvalidSortType     = errors.New("invalid sort by type")

	logsFields = [...]string{"id", "filter_id", "chain_id", "log_index", "block_hash", "block_number",
		"block_timestamp", "address", "event_sig", "subkey_values", "tx_hash", "data", "created_at",
		"expires_at", "sequence_num"}

	filterFields = [...]string{"id", "name", "address", "event_name", "event_sig", "starting_block",
		"event_idl", "subkey_paths", "retention", "max_logs_kept", "is_deleted", "is_backfilled"}
)

// The parser builds SQL expressions piece by piece for each Accept function call and resets the error and expression
// values after every call.
type pgDSLParser struct {
	args *queryArgs

	// transient properties expected to be set and reset with every expression
	expression string
	err        error
}

var _ primitives.Visitor = (*pgDSLParser)(nil)

func (v *pgDSLParser) Comparator(_ primitives.Comparator) {}

func (v *pgDSLParser) Block(prim primitives.Block) {
	cmp, err := cmpOpToString(prim.Operator)
	if err != nil {
		v.err = err

		return
	}

	v.expression = fmt.Sprintf(
		"%s %s :%s",
		blockFieldName,
		cmp,
		v.args.withIndexedField(blockFieldName, prim.Block),
	)
}

func (v *pgDSLParser) Confidence(prim primitives.Confidence) {
	switch prim.ConfidenceLevel {
	case primitives.Finalized, primitives.Unconfirmed:
		// solana LogPoller will only use and store finalized logs
		// to ensure x-chain compatibility, do nothing and return no error
		// confidence in solana is effectively a noop
		return
	default:
		// still return an error for invalid confidence levels
		v.err = fmt.Errorf("%w: %s", ErrInvalidConfidence, prim.ConfidenceLevel)

		return
	}
}

func (v *pgDSLParser) Timestamp(prim primitives.Timestamp) {
	cmp, err := cmpOpToString(prim.Operator)
	if err != nil {
		v.err = err

		return
	}

	tm := int64(prim.Timestamp) //nolint:gosec // disable G115
	if prim.Timestamp > math.MaxInt64 {
		tm = 0
	}

	v.expression = fmt.Sprintf(
		"%s %s :%s",
		timestampFieldName,
		cmp,
		v.args.withIndexedField(timestampFieldName, time.Unix(tm, 0)),
	)
}

func (v *pgDSLParser) TxHash(prim primitives.TxHash) {
	txHash, err := solana.PublicKeyFromBase58(prim.TxHash)
	if err != nil {
		v.err = err

		return
	}

	v.expression = fmt.Sprintf(
		"%s = :%s",
		txHashFieldName,
		v.args.withIndexedField(txHashFieldName, PublicKey(txHash)),
	)
}

func (v *pgDSLParser) VisitAddressFilter(p *addressFilter) {
	v.expression = fmt.Sprintf(
		"%s = :%s",
		addressFieldName,
		v.args.withIndexedField(addressFieldName, p.address),
	)
}

func (v *pgDSLParser) VisitEventSigFilter(p *eventSigFilter) {
	v.expression = fmt.Sprintf(
		"%s = :%s",
		eventSigFieldName,
		v.args.withIndexedField(eventSigFieldName, p.eventSig),
	)
}

func (v *pgDSLParser) buildQuery(
	chainID string,
	expressions []query.Expression,
	limiter query.LimitAndSort,
) (string, *queryArgs, error) {
	// reset transient properties
	v.args = newQueryArgs(chainID)
	v.expression = ""
	v.err = nil

	// build the query string
	clauses := []string{logsQuery("")}

	where, err := v.whereClause(expressions, limiter)
	if err != nil {
		return "", nil, err
	}

	clauses = append(clauses, where)

	order, err := v.orderClause(limiter)
	if err != nil {
		return "", nil, err
	}

	if len(order) > 0 {
		clauses = append(clauses, order)
	}

	limit := v.limitClause(limiter)
	if len(limit) > 0 {
		clauses = append(clauses, limit)
	}

	return strings.Join(clauses, " "), v.args, nil
}

func (v *pgDSLParser) whereClause(expressions []query.Expression, limiter query.LimitAndSort) (string, error) {
	segment := fmt.Sprintf("WHERE %s = :chain_id", chainIDFieldName)

	if len(expressions) > 0 {
		exp, err := v.combineExpressions(expressions, query.AND)
		if err != nil {
			return "", err
		}

		if exp != "" {
			segment = fmt.Sprintf("%s AND %s", segment, exp)
		}
	}

	if limiter.HasCursorLimit() {
		var op string
		switch limiter.Limit.CursorDirection {
		case query.CursorFollowing:
			op = ">"
		case query.CursorPrevious:
			op = "<"
		default:
			return "", ErrInvalidCursorDir
		}

		block, logIdx, _, err := valuesFromCursor(limiter.Limit.Cursor)
		if err != nil {
			return "", err
		}

		segment = fmt.Sprintf("%s AND (block_number %s :cursor_block_number OR (block_number = :cursor_block_number AND log_index %s :cursor_log_index))", segment, op, op)

		v.args.withField("cursor_block_number", block).
			withField("cursor_log_index", logIdx)
	}

	return segment, nil
}

func (v *pgDSLParser) orderClause(limiter query.LimitAndSort) (string, error) {
	sorting := limiter.SortBy

	if limiter.HasCursorLimit() && !limiter.HasSequenceSort() {
		var dir query.SortDirection

		switch limiter.Limit.CursorDirection {
		case query.CursorFollowing:
			dir = query.Asc
		case query.CursorPrevious:
			dir = query.Desc
		default:
			return "", ErrInvalidCursorDir
		}

		sorting = append(sorting, query.NewSortBySequence(dir))
	}

	if len(sorting) == 0 {
		return fmt.Sprintf("ORDER BY %s", defaultSort), nil
	}

	sort := make([]string, len(sorting))

	for idx, sorted := range sorting {
		var name string

		order, err := orderToString(sorted.GetDirection())
		if err != nil {
			return "", err
		}

		switch sorted.(type) {
		case query.SortByBlock:
			name = blockFieldName
		case query.SortBySequence:
			sort[idx] = fmt.Sprintf("block_number %s, log_index %s, tx_hash %s", order, order, order)

			continue
		case query.SortByTimestamp:
			name = timestampFieldName
		default:
			return "", fmt.Errorf("%w: %T", ErrInvalidSortType, sorted)
		}

		sort[idx] = fmt.Sprintf("%s %s", name, order)
	}

	return fmt.Sprintf("ORDER BY %s", strings.Join(sort, ", ")), nil
}

func (v *pgDSLParser) limitClause(limiter query.LimitAndSort) string {
	if !limiter.HasCursorLimit() && limiter.Limit.Count == 0 {
		return ""
	}

	return fmt.Sprintf("LIMIT %d", limiter.Limit.Count)
}

func (v *pgDSLParser) combineExpressions(expressions []query.Expression, op query.BoolOperator) (string, error) {
	clauses := make([]string, 0, len(expressions))

	for _, exp := range expressions {
		if exp.IsPrimitive() {
			exp.Primitive.Accept(v)

			clause, err := v.getLastExpression()
			if err != nil {
				return "", err
			}

			if clause != "" {
				clauses = append(clauses, clause)
			}
		} else {
			clause, err := v.combineExpressions(exp.BoolExpression.Expressions, exp.BoolExpression.BoolOperator)
			if err != nil {
				return "", err
			}

			if clause != "" {
				clauses = append(clauses, clause)
			}
		}
	}

	if len(clauses) == 0 {
		return "", nil
	}

	output := strings.Join(clauses, fmt.Sprintf(" %s ", op.String()))

	if len(clauses) > 1 {
		output = fmt.Sprintf("(%s)", output)
	}

	return output, nil
}

func (v *pgDSLParser) getLastExpression() (string, error) {
	exp := v.expression
	err := v.err

	v.expression = ""
	v.err = nil

	return exp, err
}

func cmpOpToString(op primitives.ComparisonOperator) (string, error) {
	switch op {
	case primitives.Eq:
		return "=", nil
	case primitives.Neq:
		return "!=", nil
	case primitives.Gt:
		return ">", nil
	case primitives.Gte:
		return ">=", nil
	case primitives.Lt:
		return "<", nil
	case primitives.Lte:
		return "<=", nil
	default:
		return "", ErrInvalidComparator
	}
}

// ensure valuesFromCursor remains consistent with the function above that creates a cursor
func valuesFromCursor(cursor string) (int64, int, []byte, error) {
	partCount := 3

	parts := strings.Split(cursor, "-")
	if len(parts) != partCount {
		return 0, 0, nil, fmt.Errorf("%w: must be composed as block-logindex-txHash", ErrInvalidCursorFormat)
	}

	block, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("%w: block number not parsable as int64", ErrInvalidCursorFormat)
	}

	logIdx, err := strconv.ParseInt(parts[1], 10, 32)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("%w: log index not parsable as int", ErrInvalidCursorFormat)
	}

	txHash, err := solana.PublicKeyFromBase58(parts[2])
	if err != nil {
		return 0, 0, nil, fmt.Errorf("%w: invalid transaction hash: %s", ErrInvalidCursorFormat, err.Error())
	}

	return block, int(logIdx), txHash.Bytes(), nil
}

func orderToString(dir query.SortDirection) (string, error) {
	switch dir {
	case query.Asc:
		return "ASC", nil
	case query.Desc:
		return "DESC", nil
	default:
		return "", ErrInvalidSortDir
	}
}

type addressFilter struct {
	address solana.PublicKey
}

func NewAddressFilter(address solana.PublicKey) query.Expression {
	return query.Expression{
		Primitive: &addressFilter{address: address},
	}
}

func (f *addressFilter) Accept(visitor primitives.Visitor) {
	switch v := visitor.(type) {
	case *pgDSLParser:
		v.VisitAddressFilter(f)
	}
}

type eventSigFilter struct {
	eventSig []byte
}

func NewEventSigFilter(sig []byte) query.Expression {
	return query.Expression{
		Primitive: &eventSigFilter{eventSig: sig},
	}
}

func (f *eventSigFilter) Accept(visitor primitives.Visitor) {
	switch v := visitor.(type) {
	case *pgDSLParser:
		v.VisitEventSigFilter(f)
	}
}
