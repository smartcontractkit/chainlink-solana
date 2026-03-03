package logpoller

import (
	"encoding/base64"
	"encoding/binary"
	"testing"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
)

func TestCPIEventExtractor_AddRemoveFilter(t *testing.T) {
	t.Run("adds and removes CPI filter", func(t *testing.T) {
		extractor := NewCPIEventExtractor(logger.Sugared(logger.Test(t)))

		sourceProgram := newRandomPublicKey(t)
		destProgram := newRandomPublicKey(t)
		methodSig := newRandomEventSignature(t)

		filter := types.Filter{
			ID:       1,
			Name:     "cpi-filter",
			Address:  sourceProgram,
			EventSig: newRandomEventSignature(t),
			ExtraFilterConfig: types.ExtraFilterConfig{
				DestProgram:     destProgram,
				MethodSignature: methodSig,
			},
		}

		extractor.AddFilter(filter)
		require.True(t, extractor.HasCPIFilters())
		require.Len(t, extractor.registered, 1)

		extractor.RemoveFilter(filter)
		require.False(t, extractor.HasCPIFilters())
		require.Empty(t, extractor.registered)
	})

	t.Run("ignores non-CPI filter", func(t *testing.T) {
		extractor := NewCPIEventExtractor(logger.Sugared(logger.Test(t)))

		filter := types.Filter{
			ID:       1,
			Name:     "regular-filter",
			Address:  newRandomPublicKey(t),
			EventSig: newRandomEventSignature(t),
		}

		extractor.AddFilter(filter)
		require.False(t, extractor.HasCPIFilters())
	})
}

func TestCPIEventExtractor_ExtractCPIEvents(t *testing.T) {
	t.Run("extracts matching CPI event with encoded struct data", func(t *testing.T) {
		extractor := NewCPIEventExtractor(logger.Sugared(logger.Test(t)))

		sourceProgram := newRandomPublicKey(t)
		destProgram := newRandomPublicKey(t)
		methodSig := newRandomEventSignature(t)
		eventSig := newRandomEventSignature(t)

		filter := types.Filter{
			ID:       1,
			Name:     "cpi-filter",
			Address:  sourceProgram,
			EventSig: eventSig,
			ExtraFilterConfig: types.ExtraFilterConfig{
				DestProgram:     destProgram,
				MethodSignature: methodSig,
			},
		}
		extractor.AddFilter(filter)

		type TestEvent struct {
			Value  int64
			Sender string
		}
		testEvent := TestEvent{Value: 12345, Sender: "test_sender"}

		eventPayload, err := bin.MarshalBorsh(&testEvent)
		require.NoError(t, err)
		eventData := append(eventSig[:], eventPayload...)

		vecLengthPrefix := make([]byte, 4)
		binary.LittleEndian.PutUint32(vecLengthPrefix, uint32(len(eventData))) //nolint:gosec
		innerInstData := append(methodSig[:], append(vecLengthPrefix, eventData...)...)

		tx := &solana.Transaction{
			Message: solana.Message{
				AccountKeys: []solana.PublicKey{
					solana.PublicKey(sourceProgram),
					solana.PublicKey(destProgram),
				},
				Instructions: []solana.CompiledInstruction{
					{
						ProgramIDIndex: 0,
					},
				},
			},
		}

		meta := &rpc.TransactionMeta{
			InnerInstructions: []rpc.InnerInstruction{
				{
					Index: 0,
					Instructions: []rpc.CompiledInstruction{
						{
							ProgramIDIndex: 1,
							Data:           innerInstData,
							StackHeight:    2,
						},
					},
				},
			},
		}

		detail := eventDetail{
			slotNumber:  100,
			blockHeight: 99,
			blockHash:   solana.Hash{1, 2, 3},
			blockTime:   solana.UnixTimeSeconds(12345),
			trxIdx:      0,
			trxSig:      solana.Signature{4, 5, 6},
		}

		events := extractor.ExtractCPIEvents(tx, meta, detail, 0)

		require.Len(t, events, 1)
		event := events[0]
		require.True(t, event.IsCPI)
		require.Equal(t, sourceProgram.ToSolana().String(), event.Program)
		require.Equal(t, uint64(100), event.SlotNumber)
		require.Equal(t, uint64(99), event.BlockHeight)
		require.Equal(t, detail.trxSig, event.TransactionHash)

		decodedData, err := base64.StdEncoding.DecodeString(event.Data)
		require.NoError(t, err)
		require.Equal(t, eventData, decodedData)

		require.Equal(t, eventSig[:], decodedData[:8])
		var decodedEvent TestEvent
		err = bin.UnmarshalBorsh(&decodedEvent, decodedData[8:])
		require.NoError(t, err)
		require.Equal(t, testEvent, decodedEvent)
	})

	t.Run("returns empty when method signature does not match", func(t *testing.T) {
		extractor := NewCPIEventExtractor(logger.Sugared(logger.Test(t)))

		sourceProgram := newRandomPublicKey(t)
		destProgram := newRandomPublicKey(t)
		methodSig := newRandomEventSignature(t)
		wrongMethodSig := newRandomEventSignature(t)

		filter := types.Filter{
			ID:       1,
			Name:     "cpi-filter",
			Address:  sourceProgram,
			EventSig: newRandomEventSignature(t),
			ExtraFilterConfig: types.ExtraFilterConfig{
				DestProgram:     destProgram,
				MethodSignature: methodSig,
			},
		}
		extractor.AddFilter(filter)

		vecLengthPrefix := []byte{0x04, 0x00, 0x00, 0x00}
		eventData := []byte{0xAA, 0xBB, 0xCC, 0xDD}
		innerInstData := append(wrongMethodSig[:], append(vecLengthPrefix, eventData...)...)

		tx := &solana.Transaction{
			Message: solana.Message{
				AccountKeys: []solana.PublicKey{
					solana.PublicKey(sourceProgram),
					solana.PublicKey(destProgram),
				},
				Instructions: []solana.CompiledInstruction{
					{
						ProgramIDIndex: 0,
					},
				},
			},
		}

		meta := &rpc.TransactionMeta{
			InnerInstructions: []rpc.InnerInstruction{
				{
					Index: 0,
					Instructions: []rpc.CompiledInstruction{
						{
							ProgramIDIndex: 1,
							Data:           innerInstData,
							StackHeight:    2,
						},
					},
				},
			},
		}

		detail := eventDetail{slotNumber: 100}
		events := extractor.ExtractCPIEvents(tx, meta, detail, 0)
		require.Empty(t, events)
	})

	t.Run("returns empty when instruction data is too short", func(t *testing.T) {
		extractor := NewCPIEventExtractor(logger.Sugared(logger.Test(t)))

		sourceProgram := newRandomPublicKey(t)
		destProgram := newRandomPublicKey(t)
		methodSig := newRandomEventSignature(t)

		filter := types.Filter{
			ID:       1,
			Name:     "cpi-filter",
			Address:  sourceProgram,
			EventSig: newRandomEventSignature(t),
			ExtraFilterConfig: types.ExtraFilterConfig{
				DestProgram:     destProgram,
				MethodSignature: methodSig,
			},
		}
		extractor.AddFilter(filter)

		tx := &solana.Transaction{
			Message: solana.Message{
				AccountKeys: []solana.PublicKey{
					solana.PublicKey(sourceProgram),
					solana.PublicKey(destProgram),
				},
				Instructions: []solana.CompiledInstruction{
					{
						ProgramIDIndex: 0,
					},
				},
			},
		}

		meta := &rpc.TransactionMeta{
			InnerInstructions: []rpc.InnerInstruction{
				{
					Index: 0,
					Instructions: []rpc.CompiledInstruction{
						{
							ProgramIDIndex: 1,
							Data:           []byte{0x01, 0x02, 0x03},
							StackHeight:    2,
						},
					},
				},
			},
		}

		detail := eventDetail{slotNumber: 100}
		events := extractor.ExtractCPIEvents(tx, meta, detail, 0)
		require.Empty(t, events)
	})

	t.Run("returns empty when actual source program does not match registered source", func(t *testing.T) {
		extractor := NewCPIEventExtractor(logger.Sugared(logger.Test(t)))

		sourceProgram := newRandomPublicKey(t)
		wrongSourceProgram := newRandomPublicKey(t)
		destProgram := newRandomPublicKey(t)
		methodSig := newRandomEventSignature(t)

		filter := types.Filter{
			ID:       1,
			Name:     "cpi-filter",
			Address:  sourceProgram,
			EventSig: newRandomEventSignature(t),
			ExtraFilterConfig: types.ExtraFilterConfig{
				DestProgram:     destProgram,
				MethodSignature: methodSig,
			},
		}
		extractor.AddFilter(filter)

		vecLengthPrefix := []byte{0x04, 0x00, 0x00, 0x00}
		eventData := []byte{0xAA, 0xBB, 0xCC, 0xDD}
		innerInstData := append(methodSig[:], append(vecLengthPrefix, eventData...)...)

		tx := &solana.Transaction{
			Message: solana.Message{
				AccountKeys: []solana.PublicKey{
					solana.PublicKey(sourceProgram),
					solana.PublicKey(destProgram),
					solana.PublicKey(wrongSourceProgram),
				},
				Instructions: []solana.CompiledInstruction{
					{
						ProgramIDIndex: 2,
					},
				},
			},
		}

		meta := &rpc.TransactionMeta{
			InnerInstructions: []rpc.InnerInstruction{
				{
					Index: 0,
					Instructions: []rpc.CompiledInstruction{
						{
							ProgramIDIndex: 1,
							Data:           innerInstData,
							StackHeight:    2,
						},
					},
				},
			},
		}

		detail := eventDetail{slotNumber: 100}
		events := extractor.ExtractCPIEvents(tx, meta, detail, 0)
		require.Empty(t, events)
	})

	t.Run("extracts nested CPI event using correct source from stack", func(t *testing.T) {
		extractor := NewCPIEventExtractor(logger.Sugared(logger.Test(t)))

		outerProgram := newRandomPublicKey(t)
		routerProgram := newRandomPublicKey(t)
		destProgram := newRandomPublicKey(t)
		methodSig := newRandomEventSignature(t)

		filter := types.Filter{
			ID:       1,
			Name:     "cpi-filter",
			Address:  routerProgram,
			EventSig: newRandomEventSignature(t),
			ExtraFilterConfig: types.ExtraFilterConfig{
				DestProgram:     destProgram,
				MethodSignature: methodSig,
			},
		}
		extractor.AddFilter(filter)

		vecLengthPrefix := []byte{0x04, 0x00, 0x00, 0x00}
		eventData := []byte{0xAA, 0xBB, 0xCC, 0xDD}
		innerInstData := append(methodSig[:], append(vecLengthPrefix, eventData...)...)

		tx := &solana.Transaction{
			Message: solana.Message{
				AccountKeys: []solana.PublicKey{
					solana.PublicKey(outerProgram),
					solana.PublicKey(routerProgram),
					solana.PublicKey(destProgram),
				},
				Instructions: []solana.CompiledInstruction{
					{
						ProgramIDIndex: 0,
					},
				},
			},
		}

		meta := &rpc.TransactionMeta{
			InnerInstructions: []rpc.InnerInstruction{
				{
					Index: 0,
					Instructions: []rpc.CompiledInstruction{
						{
							ProgramIDIndex: 1,
							Data:           []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C},
							StackHeight:    2,
						},
						{
							ProgramIDIndex: 2,
							Data:           innerInstData,
							StackHeight:    3,
						},
					},
				},
			},
		}

		detail := eventDetail{slotNumber: 100}
		events := extractor.ExtractCPIEvents(tx, meta, detail, 0)

		require.Len(t, events, 1)
		event := events[0]
		require.True(t, event.IsCPI)
		require.Equal(t, routerProgram.ToSolana().String(), event.Program)
	})

	t.Run("handles nil inputs gracefully", func(t *testing.T) {
		extractor := NewCPIEventExtractor(logger.Sugared(logger.Test(t)))

		sourceProgram := newRandomPublicKey(t)
		destProgram := newRandomPublicKey(t)
		methodSig := newRandomEventSignature(t)

		filter := types.Filter{
			ID:       1,
			Name:     "cpi-filter",
			Address:  sourceProgram,
			EventSig: newRandomEventSignature(t),
			ExtraFilterConfig: types.ExtraFilterConfig{
				DestProgram:     destProgram,
				MethodSignature: methodSig,
			},
		}
		extractor.AddFilter(filter)

		detail := eventDetail{slotNumber: 100}

		events := extractor.ExtractCPIEvents(nil, nil, detail, 0)
		require.Empty(t, events)

		tx := &solana.Transaction{
			Message: solana.Message{
				AccountKeys: []solana.PublicKey{solana.PublicKey(sourceProgram)},
			},
		}
		events = extractor.ExtractCPIEvents(tx, nil, detail, 0)
		require.Empty(t, events)

		meta := &rpc.TransactionMeta{
			InnerInstructions: []rpc.InnerInstruction{},
		}
		events = extractor.ExtractCPIEvents(tx, meta, detail, 0)
		require.Empty(t, events)
	})

	t.Run("extracts CPI event when dest program is in LoadedAddresses", func(t *testing.T) {
		extractor := NewCPIEventExtractor(logger.Sugared(logger.Test(t)))

		sourceProgram := newRandomPublicKey(t)
		destProgram := newRandomPublicKey(t)
		methodSig := newRandomEventSignature(t)
		eventSig := newRandomEventSignature(t)

		filter := types.Filter{
			ID:       1,
			Name:     "cpi-filter",
			Address:  sourceProgram,
			EventSig: eventSig,
			ExtraFilterConfig: types.ExtraFilterConfig{
				DestProgram:     destProgram,
				MethodSignature: methodSig,
			},
		}
		extractor.AddFilter(filter)

		eventData := append(eventSig[:], []byte{0xCA, 0xFE, 0xBA, 0xBE}...)
		vecLengthPrefix := make([]byte, 4)
		binary.LittleEndian.PutUint32(vecLengthPrefix, uint32(len(eventData))) //nolint:gosec
		innerInstData := append(methodSig[:], append(vecLengthPrefix, eventData...)...)

		tx := &solana.Transaction{
			Message: solana.Message{
				AccountKeys: []solana.PublicKey{
					solana.PublicKey(sourceProgram),
				},
				Instructions: []solana.CompiledInstruction{
					{
						ProgramIDIndex: 0,
					},
				},
			},
		}

		meta := &rpc.TransactionMeta{
			LoadedAddresses: rpc.LoadedAddresses{
				Writable: []solana.PublicKey{
					solana.PublicKey(destProgram),
				},
			},
			InnerInstructions: []rpc.InnerInstruction{
				{
					Index: 0,
					Instructions: []rpc.CompiledInstruction{
						{
							ProgramIDIndex: 1,
							Data:           innerInstData,
							StackHeight:    2,
						},
					},
				},
			},
		}

		detail := eventDetail{slotNumber: 100}
		events := extractor.ExtractCPIEvents(tx, meta, detail, 0)

		require.Len(t, events, 1)
		require.Equal(t, sourceProgram.ToSolana().String(), events[0].Program)
	})
}
