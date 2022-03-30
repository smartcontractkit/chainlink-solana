package solana

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLatestBlockHeight(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte(`{"jsonrpc":"2.0","result":1,"id":1}`))
		require.NoError(t, err)
	}))
	defer mockServer.Close()

	ctx := context.Background()
	c := &ContractTracker{
		reader: testSetupReader(t, mockServer.URL),
	}

	h, err := c.LatestBlockHeight(ctx)
	assert.NoError(t, err)
	assert.True(t, h > 0)
}
