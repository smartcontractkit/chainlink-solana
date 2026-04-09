package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	solanatesting "github.com/smartcontractkit/chainlink-solana/pkg/solana/testing"
)

// jsonRPCErrorBody is the JSON-RPC 2.0 error object.
type jsonRPCErrorBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// jsonRPCOutcome is either a successful result or a JSON-RPC error response.
type jsonRPCOutcome struct {
	Result   json.RawMessage
	RPCError *jsonRPCErrorBody
}

func jsonRPCSuccess(result json.RawMessage) jsonRPCOutcome {
	return jsonRPCOutcome{Result: result}
}

func jsonRPCErrorOutcome(code int, msg string) jsonRPCOutcome {
	return jsonRPCOutcome{RPCError: &jsonRPCErrorBody{Code: code, Message: msg}}
}

// jsonRPCHandler handles a single JSON-RPC request. Echo id in responses so the RPC client accepts them.
type jsonRPCHandler func(method string, params json.RawMessage, id json.RawMessage) jsonRPCOutcome

type jsonRPCRequest struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	ID     json.RawMessage `json:"id"`
}

type jsonRPCResponse struct {
	Error  *jsonRPCErrorBody `json:"error"`
	Result json.RawMessage   `json:"result"`
	ID     json.RawMessage   `json:"id"`
}

func newMockJSONRPCServer(t *testing.T, handle jsonRPCHandler) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !assert.Equal(t, http.MethodPost, r.Method) {
			http.Error(w, "only POST supported", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(r.Body)
		if !assert.NoError(t, err) {
			http.Error(w, "failed to read body", http.StatusInternalServerError)
			return
		}
		var req jsonRPCRequest
		if !assert.NoError(t, json.Unmarshal(body, &req)) {
			http.Error(w, "invalid JSON-RPC request", http.StatusBadRequest)
			return
		}
		outcome := handle(req.Method, req.Params, req.ID)
		w.Header().Set("Content-Type", "application/json")
		resp := jsonRPCResponse{Result: outcome.Result, Error: outcome.RPCError, ID: req.ID}
		out, err := json.Marshal(resp)
		if !assert.NoError(t, err) {
			http.Error(w, "failed to marshal response", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(out)
	}))
	t.Cleanup(server.Close)
	return server
}

// testBlockhashB58 is a valid blockhash string for mock getBlock responses.
const testBlockhashB58 = "5M77sHdwzH6rckuQwF8HL1w52n7hjrh4GVTFiF6T8QyB"

func requireCommitment(t *testing.T, raw json.RawMessage, expectedCommitment rpc.CommitmentType) {
	t.Helper()
	var cfg struct {
		Commitment string `json:"commitment"`
	}
	require.NoError(t, json.Unmarshal(raw, &cfg))
	require.Equal(t, string(expectedCommitment), cfg.Commitment)
}

func requireValidGetSlotParams(t *testing.T, params json.RawMessage, expectedCommitment rpc.CommitmentType) {
	t.Helper()
	var arr []json.RawMessage
	require.NoError(t, json.Unmarshal(params, &arr))
	require.Len(t, arr, 1)
	requireCommitment(t, arr[0], expectedCommitment)
}

func requireValidGetBlocksParams(t *testing.T, params json.RawMessage, wantMin, wantMax uint64, expectedCommitment rpc.CommitmentType) {
	t.Helper()
	var arr []json.RawMessage
	require.NoError(t, json.Unmarshal(params, &arr))
	require.Len(t, arr, 3)
	var minS, maxS uint64
	require.NoError(t, json.Unmarshal(arr[0], &minS))
	require.NoError(t, json.Unmarshal(arr[1], &maxS))
	require.Equal(t, wantMin, minS)
	require.Equal(t, wantMax, maxS)
	requireCommitment(t, arr[2], expectedCommitment)
}

func requireValidGetBlockParams(t *testing.T, params json.RawMessage, wantSlot uint64, expectedCommitment rpc.CommitmentType) {
	t.Helper()
	var arr []json.RawMessage
	require.NoError(t, json.Unmarshal(params, &arr))
	require.Len(t, arr, 2)
	var slot uint64
	require.NoError(t, json.Unmarshal(arr[0], &slot))
	require.Equal(t, wantSlot, slot)

	var cfg struct {
		TransactionDetails string `json:"transactionDetails"`
		Rewards            bool   `json:"rewards"`
	}
	require.NoError(t, json.Unmarshal(arr[1], &cfg))
	require.Equal(t, string(rpc.TransactionDetailsNone), cfg.TransactionDetails)
	require.False(t, cfg.Rewards)
	requireCommitment(t, arr[1], expectedCommitment)
}

func initializeMultiNodeClient(t *testing.T) *MultiNodeClient {
	url := solanatesting.SetupLocalSolNode(t)

	requestTimeout := 5 * time.Second
	lggr := logger.Test(t)
	cfg := config.NewDefault()
	enabled := true
	cfg.MultiNode.MultiNode.Enabled = &enabled

	c, err := NewMultiNodeClient(url, cfg, requestTimeout, lggr)
	require.NoError(t, err)
	return c
}

func TestMultiNodeClient_ClientVersion(t *testing.T) {
	c := initializeMultiNodeClient(t)
	_, err := c.ClientVersion(t.Context())
	require.NoError(t, err)
}

func TestMultiNodeClient_LatestBlock_MockRPC(t *testing.T) {
	runGetBlockTest(t, rpc.CommitmentConfirmed, (*MultiNodeClient).LatestBlock)
}

func TestMultiNodeClient_FinalizedBlock_MockRPC(t *testing.T) {
	runGetBlockTest(t, rpc.CommitmentConfirmed, (*MultiNodeClient).LatestFinalizedBlock)
}

func runGetBlockTest(t *testing.T, expectedCommitment rpc.CommitmentType, getBlock func(m *MultiNodeClient, ctx context.Context) (*Head, error)) {
	expectedHash := solana.MustHashFromBase58(testBlockhashB58)
	minimalGetBlockResult := fmt.Sprintf(
		`{"blockhash":%q,"previousBlockhash":"11111111111111111111111111111111","parentSlot":0}`,
		testBlockhashB58,
	)

	tests := []struct {
		name          string
		handler       jsonRPCHandler
		expectedError string
		expectedHead  Head
	}{
		{
			name: "Failure on getSlot",
			handler: func(method string, params json.RawMessage, _ json.RawMessage) jsonRPCOutcome {
				switch method {
				case "getSlot":
					requireValidGetSlotParams(t, params, expectedCommitment)
					return jsonRPCErrorOutcome(-32000, "slot unavailable")
				default:
					t.Fatalf("unexpected method %s", method)
					return jsonRPCOutcome{}
				}
			},
			expectedError: "slot unavailable",
		},
		{
			name: "Failure on GetBlocks",
			handler: func(method string, params json.RawMessage, _ json.RawMessage) jsonRPCOutcome {
				switch method {
				case "getSlot":
					requireValidGetSlotParams(t, params, expectedCommitment)
					return jsonRPCSuccess(json.RawMessage(`100`))
				case "getBlocks":
					requireValidGetBlocksParams(t, params, 90, 100, expectedCommitment)
					return jsonRPCErrorOutcome(-32001, "getBlocks failed")
				default:
					t.Fatalf("unexpected method %s", method)
				}
				return jsonRPCOutcome{}
			},
			expectedError: "failed to get blocks for latest head",
		},
		{
			name: "No non-empty blocks in range",
			handler: func(method string, params json.RawMessage, _ json.RawMessage) jsonRPCOutcome {
				switch method {
				case "getSlot":
					requireValidGetSlotParams(t, params, expectedCommitment)
					return jsonRPCSuccess(json.RawMessage(`100`))
				case "getBlocks":
					requireValidGetBlocksParams(t, params, 90, 100, expectedCommitment)
					return jsonRPCSuccess(json.RawMessage(`[]`))
				default:
					t.Fatalf("unexpected method %s", method)
				}
				return jsonRPCOutcome{}
			},
			expectedError: "failed to find non-empty block in last 10 slots",
		},
		{
			name: "Happy path: latest slot < maxLookBack",
			handler: func(method string, params json.RawMessage, _ json.RawMessage) jsonRPCOutcome {
				switch method {
				case "getSlot":
					requireValidGetSlotParams(t, params, expectedCommitment)
					return jsonRPCSuccess(json.RawMessage(`5`))
				case "getBlocks":
					requireValidGetBlocksParams(t, params, 0, 5, expectedCommitment)
					return jsonRPCSuccess(json.RawMessage(`[1,2,3]`))
				case "getBlock":
					requireValidGetBlockParams(t, params, 3, expectedCommitment)
					return jsonRPCSuccess(json.RawMessage(minimalGetBlockResult))
				default:
					t.Fatalf("unexpected method %s", method)
				}
				return jsonRPCOutcome{}
			},
			expectedHead: Head{SlotNumber: ptr[uint64](3), BlockHash: &expectedHash},
		},
		{
			name: "Happy path",
			handler: func(method string, params json.RawMessage, _ json.RawMessage) jsonRPCOutcome {
				switch method {
				case "getSlot":
					requireValidGetSlotParams(t, params, expectedCommitment)
					return jsonRPCSuccess(json.RawMessage(`100`))
				case "getBlocks":
					requireValidGetBlocksParams(t, params, 90, 100, expectedCommitment)
					return jsonRPCSuccess(json.RawMessage(`[95,97]`))
				case "getBlock":
					requireValidGetBlockParams(t, params, 97, expectedCommitment)
					return jsonRPCSuccess(json.RawMessage(minimalGetBlockResult))
				default:
					t.Fatalf("unexpected method %s", method)
				}
				return jsonRPCOutcome{}
			},
			expectedHead: Head{SlotNumber: ptr[uint64](97), BlockHash: &expectedHash},
		},
		{
			name: "Get block fails",
			handler: func(method string, params json.RawMessage, _ json.RawMessage) jsonRPCOutcome {
				switch method {
				case "getSlot":
					requireValidGetSlotParams(t, params, expectedCommitment)
					return jsonRPCSuccess(json.RawMessage(`50`))
				case "getBlocks":
					requireValidGetBlocksParams(t, params, 40, 50, expectedCommitment)
					return jsonRPCSuccess(json.RawMessage(`[48,50]`))
				case "getBlock":
					requireValidGetBlockParams(t, params, 50, expectedCommitment)
					return jsonRPCErrorOutcome(-32002, "block not found")
				default:
					t.Fatalf("unexpected method %s", method)
				}
				return jsonRPCOutcome{}
			},
			expectedError: "failed to get block for latest head",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := newMockJSONRPCServer(t, tc.handler)
			c, err := NewMultiNodeClient(server.URL, config.NewDefault(), 5*time.Second, logger.Test(t))
			require.NoError(t, err)
			head, err := getBlock(c, t.Context())
			if tc.expectedError != "" {
				require.ErrorContains(t, err, tc.expectedError)
				require.Nil(t, head)
			} else {
				require.NoError(t, err)
				require.NotNil(t, head)
				require.Equal(t, tc.expectedHead, *head)
			}
		})
	}
}

func ptr[T any](v T) *T {
	return &v
}

func TestMultiNodeClient_LatestBlock(t *testing.T) {
	c := initializeMultiNodeClient(t)

	t.Run("LatestBlock", func(t *testing.T) {
		head, err := c.LatestBlock(t.Context())
		require.NoError(t, err)
		require.True(t, head.IsValid())
		require.NotEqual(t, solana.Hash{}, head.BlockHash)
	})

	t.Run("LatestFinalizedBlock", func(t *testing.T) {
		finalizedHead, err := c.LatestFinalizedBlock(t.Context())
		require.NoError(t, err)
		require.True(t, finalizedHead.IsValid())
		require.NotEqual(t, solana.Hash{}, finalizedHead.BlockHash)
	})
}

func TestMultiNodeClient_HeadSubscriptions(t *testing.T) {
	c := initializeMultiNodeClient(t)

	t.Run("SubscribeToHeads", func(t *testing.T) {
		ch, sub, err := c.SubscribeToHeads(t.Context())
		require.NoError(t, err)
		defer sub.Unsubscribe()

		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		select {
		case head := <-ch:
			require.NotEqual(t, solana.Hash{}, head.BlockHash)
			latest, _ := c.GetInterceptedChainInfo()
			require.Equal(t, head.BlockNumber(), latest.BlockNumber)
		case <-ctx.Done():
			t.Fatal("failed to receive head: ", ctx.Err())
		}
	})

	t.Run("SubscribeToFinalizedHeads", func(t *testing.T) {
		finalizedCh, finalizedSub, err := c.SubscribeToFinalizedHeads(t.Context())
		require.NoError(t, err)
		defer finalizedSub.Unsubscribe()

		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		select {
		case finalizedHead := <-finalizedCh:
			require.NotEqual(t, solana.Hash{}, finalizedHead.BlockHash)
			latest, _ := c.GetInterceptedChainInfo()
			require.Equal(t, finalizedHead.BlockNumber(), latest.FinalizedBlockNumber)
		case <-ctx.Done():
			t.Fatal("failed to receive finalized head: ", ctx.Err())
		}
	})
}

type mockSub struct {
	unsubscribed bool
}

func newMockSub() *mockSub {
	return &mockSub{unsubscribed: false}
}

func (s *mockSub) Unsubscribe() {
	s.unsubscribed = true
}
func (s *mockSub) Err() <-chan error {
	return nil
}

func TestMultiNodeClient_RegisterSubs(t *testing.T) {
	c := initializeMultiNodeClient(t)

	t.Run("registerSub", func(t *testing.T) {
		sub := newMockSub()
		err := c.registerSub(sub, make(chan struct{}))
		require.NoError(t, err)
		require.Len(t, c.subs, 1)
		c.UnsubscribeAllExcept()
	})

	t.Run("chStopInFlight returns error and unsubscribes", func(t *testing.T) {
		chStopInFlight := make(chan struct{})
		close(chStopInFlight)
		sub := newMockSub()
		err := c.registerSub(sub, chStopInFlight)
		require.Error(t, err)
		require.Equal(t, true, sub.unsubscribed)
	})

	t.Run("UnsubscribeAllExcept", func(t *testing.T) {
		chStopInFlight := make(chan struct{})
		sub1 := newMockSub()
		sub2 := newMockSub()
		err := c.registerSub(sub1, chStopInFlight)
		require.NoError(t, err)
		err = c.registerSub(sub2, chStopInFlight)
		require.NoError(t, err)
		require.Len(t, c.subs, 2)

		c.UnsubscribeAllExcept(sub1)
		require.Len(t, c.subs, 1)
		require.Equal(t, true, sub2.unsubscribed)

		c.UnsubscribeAllExcept()
		require.Len(t, c.subs, 0)
		require.Equal(t, true, sub1.unsubscribed)
	})
}
