package solana

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var mockTransmission = []byte{
	96, 179, 69, 66, 128, 129, 73, 117, 1, 0, 0, 0,
	1, 0, 0, 0, 210, 2, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 83, 43, 91, 97,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0,
}

type mockRequest struct {
	Method  string
	Params  []json.RawMessage
	ID      uint
	JSONRPC string
}

func TestGetLatestTransmission(t *testing.T) {
	// each GetLatestTransmission submits two API requests
	// 0 + 0: everything passes
	// 1 + 0: return too short cursor (fail on first API request)
	// 0 + 1: return too short transmission
	offsets := []uint64{0, 0, 1, 0, 1}
	i := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sub uint64 = offsets[i] // change offset depending on when called
		defer func() { i++ }()      // increment

		// read message
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		// parse message
		var msg mockRequest
		err = json.Unmarshal(body, &msg)
		require.NoError(t, err)
		var opts rpc.GetAccountInfoOpts
		err = json.Unmarshal(msg.Params[1], &opts)
		require.NoError(t, err)

		// create response
		value := base64.StdEncoding.EncodeToString(mockTransmission[*opts.DataSlice.Offset : *opts.DataSlice.Offset+*opts.DataSlice.Length-sub])
		res := fmt.Sprintf(`{"jsonrpc":"2.0","result":{"context": {"slot":1},"value": {"data":["%s","base64"],"executable": false,"lamports": 1000000000,"owner": "11111111111111111111111111111111","rentEpoch":2}},"id":1}`, value)

		_, err = w.Write([]byte(res))
		require.NoError(t, err)
	}))
	defer mockServer.Close()

	expectedTime := binary.BigEndian.Uint32([]byte{97, 91, 43, 83})
	expectedAns := big.NewInt(0).SetBytes([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 210}).String()

	a, _, err := GetLatestTransmission(context.TODO(), rpc.New(mockServer.URL), solana.PublicKey{})
	assert.NoError(t, err)
	assert.Equal(t, expectedTime, a.Timestamp)
	assert.Equal(t, expectedAns, a.Data.String())

	// fail if returned cursor is too short
	_, _, err = GetLatestTransmission(context.TODO(), rpc.New(mockServer.URL), solana.PublicKey{})
	assert.ErrorIs(t, err, errCursorLength)

	// fail if returned transmission is too short
	_, _, err = GetLatestTransmission(context.TODO(), rpc.New(mockServer.URL), solana.PublicKey{})
	assert.ErrorIs(t, err, errTransmissionLength)
}
