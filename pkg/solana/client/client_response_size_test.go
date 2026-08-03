package client

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonhttp "github.com/smartcontractkit/chainlink-common/pkg/http"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

// TestClient_ResponseSizeLimit_FromContext exercises the full path from NewClient through
// jsonrpc.NewClientWithOpts to the commonhttp.LimitedTransport wrapper. It proves three things:
//   - The transport is actually wired into the underlying solana-go *rpc.Client (otherwise the
//     context limit would have no effect).
//   - commonhttp.WithResponseSizeLimit propagates through Client.requestGroup.Do(...) into the
//     outbound *http.Request.Context() (singleflight callback closure preservation).
//   - An oversized RPC response surfaces to the caller as an error (not silent truncation).
func TestClient_ResponseSizeLimit_FromContext(t *testing.T) {
	t.Parallel()

	// Pad the "result" string so the JSON body is comfortably larger than any per-test limit
	// but still a syntactically valid JSON-RPC response envelope.
	bigResult := strings.Repeat("x", 64*1024) // 64 KiB
	body := fmt.Sprintf(`{"jsonrpc":"2.0","result":"%s","id":1}`, bigResult)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(body))
		assert.NoError(t, err)
	}))
	t.Cleanup(srv.Close)

	requestTimeout := 5 * time.Second
	cfg := config.NewDefault()

	t.Run("no limit set -> body fully read, no size error", func(t *testing.T) {
		c, err := NewClient(srv.URL, cfg, requestTimeout, logger.Test(t))
		require.NoError(t, err)

		// "result" is not a valid base58 genesis hash, so ChainID will return an error from
		// solana-go's Hash parsing; the point is that the error MUST NOT be a size-cap error
		// because no limit was set on the context.
		_, err = c.ChainID(t.Context())
		require.Error(t, err)
		require.NotContains(t, err.Error(), "response is too large",
			"unexpected size-cap error when no limit is set")
	})

	t.Run("limit larger than body -> body fully read, no size error", func(t *testing.T) {
		c, err := NewClient(srv.URL, cfg, requestTimeout, logger.Test(t))
		require.NoError(t, err)

		// Fixed headroom above body size; avoid uint32(len(body)) (G115 on int→uint32).
		ctx := commonhttp.WithResponseSizeLimit(t.Context(), 2*1024*1024)
		_, err = c.ChainID(ctx)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "response is too large",
			"unexpected size-cap error when limit exceeds body size")
	})

	t.Run("limit smaller than body -> size error surfaces", func(t *testing.T) {
		c, err := NewClient(srv.URL, cfg, requestTimeout, logger.Test(t))
		require.NoError(t, err)

		ctx := commonhttp.WithResponseSizeLimit(t.Context(), 1024)
		_, err = c.ChainID(ctx)
		require.Error(t, err)
		require.ErrorContains(t, err, "response is too large")
	})
}
