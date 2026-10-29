package client

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

// newMockRPC serves the same canned JSON-RPC body for every request.
func newMockRPC(t *testing.T, body string) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte(body))
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	c, err := NewClient(server.URL, config.NewDefault(), 5*time.Second, logger.Test(t))
	require.NoError(t, err)
	return c
}

// solana-go leaves its `out` pointer nil on every error path, and singleflight boxes that
// typed nil into a non-nil interface{}. The nil-result checks must therefore come after the
// error check, or every RPC failure gets reported as "<method> returned nil result" and the
// real cause is lost (see the block history estimator's "failed to get blocks with limit").
func TestClient_RPCErrorNotMaskedByNilResultCheck(t *testing.T) {
	t.Parallel()

	const rpcErrBody = `{"jsonrpc":"2.0","error":{"code":-32429,"message":"Too many requests"},"id":1}`

	for _, tt := range []struct {
		name string
		call func(*Client) error
	}{
		{"GetBlocksWithLimit", func(c *Client) error {
			_, err := c.GetBlocksWithLimit(t.Context(), 100, 20)
			return err
		}},
		{"GetBlock", func(c *Client) error {
			_, err := c.GetBlock(t.Context(), 100)
			return err
		}},
		{"GetTransaction", func(c *Client) error {
			_, err := c.GetTransaction(t.Context(), solana.Signature{})
			return err
		}},
		{"LatestBlockhash", func(c *Client) error {
			_, err := c.LatestBlockhash(t.Context())
			return err
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.call(newMockRPC(t, rpcErrBody))
			require.Error(t, err)
			require.ErrorContains(t, err, "Too many requests", "the underlying RPC error must reach the caller")
			require.NotContains(t, err.Error(), "returned nil result", "nil-result check must not shadow the RPC error")
		})
	}
}

// A literal JSON `null` result is not a transport-level error. solana-go special-cases null
// for getTransaction and getBlock and turns it into a sentinel; for the others the nil-result
// check is what reports it. Either way the caller gets something specific, not a masked error.
func TestClient_NullResultStillReported(t *testing.T) {
	t.Parallel()

	const nullBody = `{"jsonrpc":"2.0","result":null,"id":1}`

	for _, tt := range []struct {
		name string
		want string
		call func(*Client) error
	}{
		{"GetBlocksWithLimit", "GetBlocksWithLimit returned nil result", func(c *Client) error {
			_, err := c.GetBlocksWithLimit(t.Context(), 100, 20)
			return err
		}},
		{"LatestBlockhash", "LatestBlockhash returned nil result", func(c *Client) error {
			_, err := c.LatestBlockhash(t.Context())
			return err
		}},
		// solana-go maps a null getBlock/getTransaction result to a sentinel error before we see it.
		{"GetBlock", rpc.ErrNotConfirmed.Error(), func(c *Client) error {
			_, err := c.GetBlock(t.Context(), 100)
			return err
		}},
		{"GetTransaction", rpc.ErrNotFound.Error(), func(c *Client) error {
			_, err := c.GetTransaction(t.Context(), solana.Signature{})
			return err
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.call(newMockRPC(t, nullBody))
			require.ErrorContains(t, err, tt.want)
		})
	}
}
