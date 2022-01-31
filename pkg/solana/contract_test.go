package solana

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/smartcontractkit/chainlink/core/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
)

var mockTransmission = []byte{
	96, 179, 69, 66, 128, 129, 73, 117, // account discriminator
	1, // version
	60, 231, 89, 57, 209, 16, 239, 36, 134, 61, 118, 182, 240, 207, 143,
	75, 4, 54, 145, 168, 142, 150, 94, 65, 111, 136, 110,
	107, 148, 97, 201, 107, // store, 32 bytes
	71, 192, 69, 231, 146, 55, 106,
	174, 33, 124, 218, 253, 229, 182, 236, 61, 80, 206, 74,
	121, 148, 151, 4, 63, 154, 142, 206, 234, 134, 108, 73, 141, // writer, 32 bytes
	18, // decimals, 1 byte
	69, 84, 72, 47, 66, 84, 67, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, // description, 32 bytes
	128, 56, 1, 0, // flagging_threshold, 4 bytes
	1, 0, 0, 0, // latest_round_id, 4 bytes
	30,         // granularity, 1 byte
	0, 4, 0, 0, // live_length, 4 bytes
	8, 0, 0, 0, // live_cursor, 4 bytes
	0, 0, 0, 0, // historical_cursor, 4 bytes

	0, 0, 0, 0, 0, 0, 0, 0,
	0,
	// -- end of header

	// 240, 181, 184, 97, 0, 0, 0, 0, // timestamp u64, 8 bytes
	// 255, 122, 108, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // answer i128, 16 byte

	255, 184, 184, 97, 0, 0, 0, 0,
	192, 197, 168, 108, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,

	18, 185, 184, 97, 0, 0, 0, 0,
	192, 197, 168, 108, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,

	67, 187, 184, 97, 0, 0, 0, 0,
	64, 92, 65, 109, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,

	19, 189, 184, 97, 0, 0, 0, 0,
	192, 206, 229, 108, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,

	193, 189, 184, 97, 0, 0, 0, 0,
	128, 149, 19, 109, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,

	30, 191, 184, 97, 0, 0, 0, 0,
	64, 56, 77, 108, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,

	35, 192, 184, 97, 0, 0, 0, 0,
	64, 20, 89, 107, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,

	83, 43, 91, 97, 0, 0, 0, 0,
	210, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,

	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,

	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
}

var (
	expectedTime = uint64(1633364819)
	expectedAns  = big.NewInt(0).SetBytes([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 210}).String()
)

type mockRequest struct {
	Method  string
	Params  []json.RawMessage
	ID      uint
	JSONRPC string
}

func testStateResponse() []byte {
	value := base64.StdEncoding.EncodeToString(mockState.Raw)
	res := fmt.Sprintf(`{"jsonrpc":"2.0","result":{"context": {"slot":1},"value": {"data":["%s","base64"],"executable": false,"lamports": 1000000000,"owner": "11111111111111111111111111111111","rentEpoch":2}},"id":1}`, value)
	return []byte(res)
}

func testTransmissionsResponse(t *testing.T, body []byte, sub uint64) []byte {
	// parse message
	var msg mockRequest
	err := json.Unmarshal(body, &msg)
	require.NoError(t, err)
	var opts rpc.GetAccountInfoOpts
	err = json.Unmarshal(msg.Params[1], &opts)
	require.NoError(t, err)

	// create response
	value := base64.StdEncoding.EncodeToString(mockTransmission[*opts.DataSlice.Offset : *opts.DataSlice.Offset+*opts.DataSlice.Length-sub])
	res := fmt.Sprintf(`{"jsonrpc":"2.0","result":{"context": {"slot":1},"value": {"data":["%s","base64"],"executable": false,"lamports": 1000000000,"owner": "11111111111111111111111111111111","rentEpoch":2}},"id":1}`, value)
	return []byte(res)
}

func TestGetState(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write(testStateResponse())
		require.NoError(t, err)
	}))
	defer mockServer.Close()

	// happy path does not error (actual state decoding handled in types_test)
	_, _, err := GetState(context.TODO(), rpc.New(mockServer.URL), solana.PublicKey{}, rpc.CommitmentConfirmed)
	require.NoError(t, err)
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

		_, err = w.Write(testTransmissionsResponse(t, body, sub))
		require.NoError(t, err)
	}))
	defer mockServer.Close()

	a, _, err := GetLatestTransmission(context.TODO(), rpc.New(mockServer.URL), solana.PublicKey{}, rpc.CommitmentConfirmed)
	assert.NoError(t, err)
	assert.Equal(t, expectedTime, a.Timestamp)
	assert.Equal(t, expectedAns, a.Data.String())

	// fail if returned cursor is too short
	_, _, err = GetLatestTransmission(context.TODO(), rpc.New(mockServer.URL), solana.PublicKey{}, rpc.CommitmentConfirmed)
	assert.ErrorIs(t, err, errCursorLength)

	// fail if returned transmission is too short
	_, _, err = GetLatestTransmission(context.TODO(), rpc.New(mockServer.URL), solana.PublicKey{}, rpc.CommitmentConfirmed)
	assert.ErrorIs(t, err, errTransmissionLength)
}

func TestStatePolling(t *testing.T) {
	i := atomic.NewInt32(0)
	wait := 5 * time.Second
	callsPerSecond := 4 // total number of rpc calls between getState and GetLatestTransmission

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// create response
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		i.Inc() // count calls

		// state query
		if bytes.Contains(body, []byte("11111111111111111111111111111111")) {
			_, err := w.Write(testStateResponse())
			require.NoError(t, err)
			return
		}

		// transmissions query
		_, err = w.Write(testTransmissionsResponse(t, body, 0))
		require.NoError(t, err)
	}))

	tracker := ContractTracker{
		StateID:         solana.MustPublicKeyFromBase58("11111111111111111111111111111111"),
		TransmissionsID: solana.MustPublicKeyFromBase58("11111111111111111111111111111112"),
		client:          NewClient(OCR2Spec{NodeEndpointHTTP: mockServer.URL}, logger.TestLogger(t)),
		lggr:            logger.TestLogger(t),
		stateLock:       &sync.RWMutex{},
		ansLock:         &sync.RWMutex{},
		staleTimeout:    defaultStaleTimeout,
	}
	require.NoError(t, tracker.Start())
	require.Error(t, tracker.Start()) // test startOnce
	time.Sleep(wait)
	require.NoError(t, tracker.Close())
	require.Error(t, tracker.Close())                                           // test StopOnce
	mockServer.Close()                                                          // close server once tracker is stopped
	assert.GreaterOrEqual(t, callsPerSecond*int(wait.Seconds()), int(i.Load())) // expect minimum number of calls

	answer, err := tracker.ReadAnswer()
	assert.NoError(t, err)
	assert.Equal(t, expectedTime, answer.Timestamp)
	assert.Equal(t, expectedAns, answer.Data.String())
}
