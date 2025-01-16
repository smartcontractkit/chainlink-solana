package logpoller

import (
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/types/query"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query/primitives"
)

var (
	chainID = "chain"
	txHash  = "J1zQwrBNBngz26jRPNWsUSZMHJwBwpkoDitXRV95LdK4"
)

func assertArgs(t *testing.T, args *queryArgs, numVals int) {
	values, err := args.toArgs()

	assert.Len(t, values, numVals)
	assert.NoError(t, err)
}

func TestDSLParser(t *testing.T) {
	t.Parallel()

	t.Run("query with no filters no order and no limit", func(t *testing.T) {
		t.Parallel()

		parser := &pgDSLParser{}
		expressions := []query.Expression{}
		limiter := query.LimitAndSort{}

		result, args, err := parser.buildQuery(chainID, expressions, limiter)

		require.NoError(t, err)
		assert.Equal(t, logsQuery(" WHERE chain_id = :chain_id ORDER BY "+defaultSort), result)

		assertArgs(t, args, 1)
	})

	t.Run("query with cursor and no order by", func(t *testing.T) {
		t.Parallel()

		var pk solana.PublicKey

		_, _ = rand.Read(pk[:])

		parser := &pgDSLParser{}
		expressions := []query.Expression{
			NewAddressFilter(pk),
			NewEventSigFilter([]byte("test")),
			query.Confidence(primitives.Unconfirmed),
		}
		limiter := query.NewLimitAndSort(query.CursorLimit(fmt.Sprintf("10-5-%s", txHash), query.CursorFollowing, 20))

		result, args, err := parser.buildQuery(chainID, expressions, limiter)
		expected := logsQuery(
			" WHERE chain_id = :chain_id " +
				"AND (address = :address_0 AND event_sig = :event_sig_0) " +
				"AND (block_number > :cursor_block_number OR (block_number = :cursor_block_number " +
				"AND log_index > :cursor_log_index)) " +
				"ORDER BY block_number ASC, log_index ASC, tx_hash ASC LIMIT 20")

		require.NoError(t, err)
		assert.Equal(t, expected, result)

		assertArgs(t, args, 5)
	})

	t.Run("query with limit and no order by", func(t *testing.T) {
		t.Parallel()

		var pk solana.PublicKey

		_, _ = rand.Read(pk[:])

		parser := &pgDSLParser{}
		expressions := []query.Expression{
			NewAddressFilter(pk),
			NewEventSigFilter([]byte("test")),
		}
		limiter := query.NewLimitAndSort(query.CountLimit(20))

		result, args, err := parser.buildQuery(chainID, expressions, limiter)
		expected := logsQuery(
			" WHERE chain_id = :chain_id " +
				"AND (address = :address_0 AND event_sig = :event_sig_0) " +
				"ORDER BY " + defaultSort + " " +
				"LIMIT 20")

		require.NoError(t, err)
		assert.Equal(t, expected, result)

		assertArgs(t, args, 3)
	})

	t.Run("query with order by sequence no cursor no limit", func(t *testing.T) {
		t.Parallel()

		parser := &pgDSLParser{}
		expressions := []query.Expression{}
		limiter := query.NewLimitAndSort(query.Limit{}, query.NewSortBySequence(query.Desc))

		result, args, err := parser.buildQuery(chainID, expressions, limiter)
		expected := logsQuery(
			" WHERE chain_id = :chain_id " +
				"ORDER BY block_number DESC, log_index DESC, tx_hash DESC")

		require.NoError(t, err)
		assert.Equal(t, expected, result)

		assertArgs(t, args, 1)
	})

	t.Run("query with multiple order by no limit", func(t *testing.T) {
		t.Parallel()

		parser := &pgDSLParser{}
		expressions := []query.Expression{}
		limiter := query.NewLimitAndSort(query.Limit{}, query.NewSortByBlock(query.Asc), query.NewSortByTimestamp(query.Desc))

		result, args, err := parser.buildQuery(chainID, expressions, limiter)
		expected := logsQuery(
			" WHERE chain_id = :chain_id " +
				"ORDER BY block_number ASC, block_timestamp DESC")

		require.NoError(t, err)
		assert.Equal(t, expected, result)

		assertArgs(t, args, 1)
	})

	t.Run("basic query with default primitives no order by and cursor", func(t *testing.T) {
		t.Parallel()

		parser := &pgDSLParser{}
		expressions := []query.Expression{
			query.Timestamp(10, primitives.Eq),
			query.TxHash(txHash),
			query.Block("99", primitives.Neq),
			query.Confidence(primitives.Finalized),
		}
		limiter := query.NewLimitAndSort(query.CursorLimit(fmt.Sprintf("10-20-%s", txHash), query.CursorPrevious, 20))

		result, args, err := parser.buildQuery(chainID, expressions, limiter)
		expected := logsQuery(
			" WHERE chain_id = :chain_id " +
				"AND (block_timestamp = :block_timestamp_0 AND tx_hash = :tx_hash_0 " +
				"AND block_number != :block_number_0) " +
				"AND (block_number < :cursor_block_number OR (block_number = :cursor_block_number " +
				"AND log_index < :cursor_log_index)) " +
				"ORDER BY block_number DESC, log_index DESC, tx_hash DESC LIMIT 20")

		require.NoError(t, err)
		assert.Equal(t, expected, result)

		assertArgs(t, args, 6)
	})

	t.Run("query for finality", func(t *testing.T) {
		t.Parallel()

		t.Run("finalized", func(t *testing.T) {
			parser := &pgDSLParser{}
			expressions := []query.Expression{query.Confidence(primitives.Finalized)}
			limiter := query.LimitAndSort{}

			result, args, err := parser.buildQuery(chainID, expressions, limiter)
			expected := logsQuery(
				" WHERE chain_id = :chain_id " +
					"ORDER BY " + defaultSort)

			require.NoError(t, err)
			assert.Equal(t, expected, result)

			assertArgs(t, args, 1)
		})

		t.Run("unconfirmed", func(t *testing.T) {
			parser := &pgDSLParser{}
			expressions := []query.Expression{query.Confidence(primitives.Unconfirmed)}
			limiter := query.LimitAndSort{}

			result, args, err := parser.buildQuery(chainID, expressions, limiter)
			expected := logsQuery(
				" WHERE chain_id = :chain_id " +
					"ORDER BY " + defaultSort)

			require.NoError(t, err)
			assert.Equal(t, expected, result)

			assertArgs(t, args, 1)
		})
	})

	// nested query -> a & (b || c)
	t.Run("nested query", func(t *testing.T) {
		t.Parallel()

		parser := &pgDSLParser{}

		expressions := []query.Expression{
			{BoolExpression: query.BoolExpression{
				Expressions: []query.Expression{
					query.Timestamp(10, primitives.Gte),
					{BoolExpression: query.BoolExpression{
						Expressions: []query.Expression{
							query.TxHash(txHash),
							query.Confidence(primitives.Unconfirmed),
						},
						BoolOperator: query.OR,
					}},
				},
				BoolOperator: query.AND,
			}},
		}
		limiter := query.LimitAndSort{}

		result, args, err := parser.buildQuery(chainID, expressions, limiter)
		expected := logsQuery(
			" WHERE chain_id = :chain_id " +
				"AND (block_timestamp >= :block_timestamp_0 AND tx_hash = :tx_hash_0) " +
				"ORDER BY " + defaultSort)

		require.NoError(t, err)
		assert.Equal(t, expected, result)

		assertArgs(t, args, 3)
	})

	// deep nested query -> a & (b || (c & d))
	t.Run("nested query deep", func(t *testing.T) {
		t.Parallel()

		sigFilter := NewEventSigFilter([]byte("test"))
		parser := &pgDSLParser{}

		expressions := []query.Expression{
			{BoolExpression: query.BoolExpression{
				Expressions: []query.Expression{
					query.Timestamp(10, primitives.Eq),
					{BoolExpression: query.BoolExpression{
						Expressions: []query.Expression{
							query.TxHash(txHash),
							{BoolExpression: query.BoolExpression{
								Expressions: []query.Expression{
									query.Confidence(primitives.Unconfirmed),
									sigFilter,
								},
								BoolOperator: query.AND,
							}},
						},
						BoolOperator: query.OR,
					}},
				},
				BoolOperator: query.AND,
			}},
		}
		limiter := query.LimitAndSort{}

		result, args, err := parser.buildQuery(chainID, expressions, limiter)
		expected := logsQuery(
			" WHERE chain_id = :chain_id " +
				"AND (block_timestamp = :block_timestamp_0 " +
				"AND (tx_hash = :tx_hash_0 OR event_sig = :event_sig_0)) " +
				"ORDER BY " + defaultSort)

		require.NoError(t, err)
		assert.Equal(t, expected, result)

		assertArgs(t, args, 4)
	})
}
