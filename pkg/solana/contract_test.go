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
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
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

func TestFetchTransmissionsRaw(t *testing.T) {
	// each fetchTransmissionsState submits two API requests
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

	a, err := fetchTransmissionsState(context.TODO(), rpc.New(mockServer.URL), solana.PublicKey{})
	assert.NoError(t, err)
	assert.Equal(t, expectedTime, a.Timestamp)
	assert.Equal(t, expectedAns, a.Answer.String())

	// fail if returned cursor is too short
	_, err = fetchTransmissionsState(context.TODO(), rpc.New(mockServer.URL), solana.PublicKey{})
	assert.ErrorIs(t, err, errCursorLength)

	// fail if returned transmission is too short
	_, err = fetchTransmissionsState(context.TODO(), rpc.New(mockServer.URL), solana.PublicKey{})
	assert.ErrorIs(t, err, errTransmissionLength)
}

type mockFetch struct {
	lock *atomic.Bool
	done chan struct{}
}

func (m *mockFetch) fetch(ctx context.Context) error {
	// lock
	if !m.lock.CAS(false, true) {
		return errAlreadyTriggered
	}
	defer m.lock.Store(false)

	// create channel to announce done
	m.done = make(chan struct{})
	defer close(m.done)

	// start a timer for 5 seconds
	timer := make(chan struct{})
	go func() {
		time.Sleep(2 * time.Second)
		close(timer)
	}()

	fmt.Println("sleeping...")
	select {
	case <-timer:
		// continue
	case <-ctx.Done():
		return errFetchCtxCancelled
	}
	fmt.Println("...waking")
	return nil
}

func TestFetchWrap(t *testing.T) {
	m := mockFetch{lock: atomic.NewBool(false)}
	n := 5

	// calls to fetch
	res := make(chan error)
	for i := 0; i < n; i++ {
		go func() {
			res <- fetchWrap(context.TODO(), m.fetch, &m.done)
		}()
	}

	for i := 0; i < n; i++ {
		assert.NoError(t, <-res) // wait to check the error
	}
}

func TestFetchWrap_CancelContext(t *testing.T) {
	m := mockFetch{lock: atomic.NewBool(false)}
	n := 5

	ctx, cancel := context.WithTimeout(context.TODO(), 1*time.Second)
	defer cancel()

	// calls to fetch
	res := make(chan error)
	for i := 0; i < n; i++ {
		go func() {
			res <- fetchWrap(ctx, m.fetch, &m.done)
		}()
	}

	for i := 0; i < n; i++ {
		assert.ErrorIs(t, <-res, errFetchCtxCancelled) // wait to check the error
	}
}
