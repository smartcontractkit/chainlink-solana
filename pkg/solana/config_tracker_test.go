package solana

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
)

func TestLatestBlockHeight(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte(`{"jsonrpc":"2.0","result":1,"id":1}`))
		require.NoError(t, err)
	}))
	defer mockServer.Close()

	ctx := context.Background()
	c := &ConfigTracker{
		getReader: func() (client.Reader, error) { return testSetupReader(t, mockServer.URL), nil },
	}

	h, err := c.LatestBlockHeight(ctx)
	assert.NoError(t, err)
	assert.True(t, h > 0)
}
