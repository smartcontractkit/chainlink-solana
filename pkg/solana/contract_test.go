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
	96, 179, 69, 66, 128, 129, 73, 117, 2, 0, 42, 195,
	51, 245, 109, 152, 157, 191, 52, 252, 122, 195, 60, 136,
	46, 95, 164, 123, 7, 132, 62, 133, 183, 255, 55, 14,
	134, 167, 4, 188, 130, 218, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 153, 56, 154, 99, 168, 217, 60, 195, 166, 70,
	52, 237, 80, 50, 218, 93, 164, 123, 170, 66, 255, 168,
	40, 27, 40, 194, 147, 199, 20, 178, 51, 196, 69, 84,
	72, 47, 66, 84, 67, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 18, 128, 56, 1, 0, 13,
	0, 0, 0, 30, 3, 0, 0, 0, 1, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 64, 0, 0, 0,
	0, 0, 0, 0, 83, 43, 91, 97, 0, 0, 0, 0,
	14, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 58, 0, 0, 0,
	0, 0, 0, 0, 83, 43, 91, 97, 0, 0, 0, 0,
	12, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 61, 0, 0, 0,
	0, 0, 0, 0, 83, 43, 91, 97, 0, 0, 0, 0,
	13, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
}

var (
	expectedTime = uint32(1633364819)
	expectedAns  = big.NewInt(14).String()
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
	mock := mockTransmission
	value := base64.StdEncoding.EncodeToString(mock[*opts.DataSlice.Offset : *opts.DataSlice.Offset+*opts.DataSlice.Length-sub])
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
	// 1 (+ 0): return too short cursor (fail on first API request)
	// 0 + 1: return too short transmission
	// 0 + 0: everything passes (v1 config)
	offsets := []uint64{0, 0, 1, 0, 1, 0, 0}
	i := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sub = offsets[i]   // change offset depending on when called
		defer func() { i++ }() // increment

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

	// fail if returned transmission header is too short
	_, _, err = GetLatestTransmission(context.TODO(), rpc.New(mockServer.URL), solana.PublicKey{}, rpc.CommitmentConfirmed)
	assert.Error(t, err)

	// fail if returned transmission is too short
	_, _, err = GetLatestTransmission(context.TODO(), rpc.New(mockServer.URL), solana.PublicKey{}, rpc.CommitmentConfirmed)
	assert.Error(t, err)
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
			_, err = w.Write(testStateResponse())
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

func TestNilPointerHandling(t *testing.T) {
	passFirst := false
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var data []byte
		if passFirst {
			// successful transmissions query
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			data = testTransmissionsResponse(t, body, 0)
			passFirst = false
		} else {
			// bad payload missing data
			data = []byte(`{"jsonrpc":"2.0","result":{"context": {"slot":1},"value": {"executable": false,"lamports": 1000000000,"owner": "11111111111111111111111111111111","rentEpoch":2}},"id":1}`)
		}
		_, err := w.Write(data)
		require.NoError(t, err)
	}))
	defer mockServer.Close()

	errString := "nil pointer returned in "

	// fail on get state query
	_, _, err := GetState(context.TODO(), rpc.New(mockServer.URL), solana.PublicKey{}, rpc.CommitmentConfirmed)
	assert.EqualError(t, err, errString+"GetState.GetAccountInfoWithOpts")

	// fail on transmissions header query
	_, _, err = GetLatestTransmission(context.TODO(), rpc.New(mockServer.URL), solana.PublicKey{}, rpc.CommitmentConfirmed)
	assert.EqualError(t, err, errString+"GetLatestTransmission.GetAccountInfoWithOpts.Header")

	passFirst = true // allow proper response for header query, fail on transmission
	_, _, err = GetLatestTransmission(context.TODO(), rpc.New(mockServer.URL), solana.PublicKey{}, rpc.CommitmentConfirmed)
	assert.EqualError(t, err, errString+"GetLatestTransmission.GetAccountInfoWithOpts.Transmission")

}
