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

	t.Run("matches by source dest and method across distinct filter names", func(t *testing.T) {
		extractor := NewCPIEventExtractor(logger.Sugared(logger.Test(t)))

		sourceProgram := newRandomPublicKey(t)
		destProgram := newRandomPublicKey(t)
		methodSig := newRandomEventSignature(t)

		filterA := types.Filter{
			ID:       1,
			Name:     "cpi-filter-a",
			Address:  sourceProgram,
			EventSig: newRandomEventSignature(t),
			ExtraFilterConfig: types.ExtraFilterConfig{
				DestProgram:     destProgram,
				MethodSignature: methodSig,
			},
		}
		filterB := filterA
		filterB.ID = 2
		filterB.Name = "cpi-filter-b"

		extractor.AddFilter(filterA)
		extractor.AddFilter(filterB)

		require.True(t, extractor.HasCPIFilters())
		require.Len(t, extractor.registered, 2)
		require.Len(t, extractor.matchKeyRefs, 1)

		extractor.RemoveFilter(filterA)
		require.True(t, extractor.HasCPIFilters())
		require.Len(t, extractor.registered, 1)
		require.Len(t, extractor.matchKeyRefs, 1)

		extractor.RemoveFilter(filterB)
		require.False(t, extractor.HasCPIFilters())
		require.Empty(t, extractor.registered)
		require.Empty(t, extractor.matchKeyRefs)
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

		blockData := types.BlockData{
			SlotNumber:       100,
			BlockHeight:      99,
			BlockHash:        solana.Hash{1, 2, 3},
			BlockTime:        solana.UnixTimeSeconds(12345),
			TransactionIndex: 0,
			TransactionHash:  solana.Signature{4, 5, 6},
		}

		events := extractor.ExtractCPIEvents(tx, meta, blockData)

		require.Len(t, events, 1)
		event := events[0]
		require.True(t, event.IsCPI)
		require.Equal(t, sourceProgram.ToSolana().String(), event.Program)
		require.Equal(t, uint64(100), event.SlotNumber)
		require.Equal(t, uint64(99), event.BlockHeight)
		require.Equal(t, blockData.TransactionHash, event.TransactionHash)

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

		blockData := types.BlockData{SlotNumber: 100}
		events := extractor.ExtractCPIEvents(tx, meta, blockData)
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

		blockData := types.BlockData{SlotNumber: 100}
		events := extractor.ExtractCPIEvents(tx, meta, blockData)
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

		blockData := types.BlockData{SlotNumber: 100}
		events := extractor.ExtractCPIEvents(tx, meta, blockData)
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

		blockData := types.BlockData{SlotNumber: 100}
		events := extractor.ExtractCPIEvents(tx, meta, blockData)

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

		blockData := types.BlockData{SlotNumber: 100}

		events := extractor.ExtractCPIEvents(nil, nil, blockData)
		require.Empty(t, events)

		tx := &solana.Transaction{
			Message: solana.Message{
				AccountKeys: []solana.PublicKey{solana.PublicKey(sourceProgram)},
			},
		}
		events = extractor.ExtractCPIEvents(tx, nil, blockData)
		require.Empty(t, events)

		meta := &rpc.TransactionMeta{
			InnerInstructions: []rpc.InnerInstruction{},
		}
		events = extractor.ExtractCPIEvents(tx, meta, blockData)
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

		blockData := types.BlockData{SlotNumber: 100}
		events := extractor.ExtractCPIEvents(tx, meta, blockData)

		require.Len(t, events, 1)
		require.Equal(t, sourceProgram.ToSolana().String(), events[0].Program)
	})

	t.Run("extracts Anchor emit_cpi event using direct format", func(t *testing.T) {
		extractor := NewCPIEventExtractor(logger.Sugared(logger.Test(t)))

		sourceProgram := newRandomPublicKey(t)
		destProgram := newRandomPublicKey(t)
		anchorMethodSig := types.AnchorCPIEventDiscriminator()
		eventSig := newRandomEventSignature(t)

		filter := types.Filter{
			ID:       1,
			Name:     "anchor-cpi-filter",
			Address:  sourceProgram,
			EventSig: eventSig,
			ExtraFilterConfig: types.ExtraFilterConfig{
				DestProgram:     destProgram,
				MethodSignature: anchorMethodSig,
			},
		}
		extractor.AddFilter(filter)

		eventPayload := []byte{0xDE, 0xAD, 0xBE, 0xEF}
		eventData := append(eventSig[:], eventPayload...)
		innerInstData := append(anchorMethodSig[:], eventData...)

		tx := &solana.Transaction{
			Message: solana.Message{
				AccountKeys: []solana.PublicKey{
					solana.PublicKey(sourceProgram),
					solana.PublicKey(destProgram),
				},
				Instructions: []solana.CompiledInstruction{
					{ProgramIDIndex: 0},
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

		blockData := types.BlockData{SlotNumber: 200}
		events := extractor.ExtractCPIEvents(tx, meta, blockData)

		require.Len(t, events, 1)
		require.True(t, events[0].IsCPI)
		require.Equal(t, sourceProgram.ToSolana().String(), events[0].Program)

		decodedData, err := base64.StdEncoding.DecodeString(events[0].Data)
		require.NoError(t, err)
		require.Equal(t, eventData, decodedData)
	})

	t.Run("rejects vec-format CPI event with mismatched declared length", func(t *testing.T) {
		extractor := NewCPIEventExtractor(logger.Sugared(logger.Test(t)))

		sourceProgram := newRandomPublicKey(t)
		destProgram := newRandomPublicKey(t)
		methodSig := newRandomEventSignature(t)

		filter := types.Filter{
			ID:       1,
			Name:     "ccip-cpi-filter",
			Address:  sourceProgram,
			EventSig: newRandomEventSignature(t),
			ExtraFilterConfig: types.ExtraFilterConfig{
				DestProgram:     destProgram,
				MethodSignature: methodSig,
			},
		}
		extractor.AddFilter(filter)

		eventData := []byte{0xAA, 0xBB, 0xCC, 0xDD}
		wrongLen := make([]byte, 4)
		binary.LittleEndian.PutUint32(wrongLen, uint32(len(eventData)+99)) //nolint:gosec
		innerInstData := append(methodSig[:], append(wrongLen, eventData...)...)

		tx := &solana.Transaction{
			Message: solana.Message{
				AccountKeys: []solana.PublicKey{
					solana.PublicKey(sourceProgram),
					solana.PublicKey(destProgram),
				},
				Instructions: []solana.CompiledInstruction{
					{ProgramIDIndex: 0},
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

		blockData := types.BlockData{SlotNumber: 100}
		events := extractor.ExtractCPIEvents(tx, meta, blockData)
		require.Empty(t, events)
	})

	t.Run("rejects vec-format CPI event with zero declared length", func(t *testing.T) {
		extractor := NewCPIEventExtractor(logger.Sugared(logger.Test(t)))

		sourceProgram := newRandomPublicKey(t)
		destProgram := newRandomPublicKey(t)
		methodSig := newRandomEventSignature(t)

		filter := types.Filter{
			ID:       1,
			Name:     "ccip-cpi-filter",
			Address:  sourceProgram,
			EventSig: newRandomEventSignature(t),
			ExtraFilterConfig: types.ExtraFilterConfig{
				DestProgram:     destProgram,
				MethodSignature: methodSig,
			},
		}
		extractor.AddFilter(filter)

		eventData := []byte{0xAA, 0xBB, 0xCC, 0xDD}
		zeroLen := []byte{0x00, 0x00, 0x00, 0x00}
		innerInstData := append(methodSig[:], append(zeroLen, eventData...)...)

		tx := &solana.Transaction{
			Message: solana.Message{
				AccountKeys: []solana.PublicKey{
					solana.PublicKey(sourceProgram),
					solana.PublicKey(destProgram),
				},
				Instructions: []solana.CompiledInstruction{
					{ProgramIDIndex: 0},
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

		blockData := types.BlockData{SlotNumber: 100}
		events := extractor.ExtractCPIEvents(tx, meta, blockData)
		require.Empty(t, events)
	})

	t.Run("falls back to outer program when StackHeight is zero", func(t *testing.T) {
		extractor := NewCPIEventExtractor(logger.Sugared(logger.Test(t)))

		outerProgram := newRandomPublicKey(t)
		destProgram := newRandomPublicKey(t)
		methodSig := newRandomEventSignature(t)

		filter := types.Filter{
			ID:       1,
			Name:     "cpi-filter-stackheight-zero",
			Address:  outerProgram,
			EventSig: newRandomEventSignature(t),
			ExtraFilterConfig: types.ExtraFilterConfig{
				DestProgram:     destProgram,
				MethodSignature: methodSig,
			},
		}
		extractor.AddFilter(filter)

		eventData := []byte{0xCA, 0xFE, 0xBA, 0xBE}
		vecLengthPrefix := make([]byte, 4)
		binary.LittleEndian.PutUint32(vecLengthPrefix, uint32(len(eventData))) //nolint:gosec
		innerInstData := append(methodSig[:], append(vecLengthPrefix, eventData...)...)

		tx := &solana.Transaction{
			Message: solana.Message{
				AccountKeys: []solana.PublicKey{
					solana.PublicKey(outerProgram),
					solana.PublicKey(destProgram),
				},
				Instructions: []solana.CompiledInstruction{
					{ProgramIDIndex: 0},
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
							StackHeight:    0,
						},
					},
				},
			},
		}

		blockData := types.BlockData{SlotNumber: 100}
		events := extractor.ExtractCPIEvents(tx, meta, blockData)

		require.Len(t, events, 1)
		require.True(t, events[0].IsCPI)
		require.Equal(t, outerProgram.ToSolana().String(), events[0].Program)
	})
}

func TestExtractAnchorCPIEventData(t *testing.T) {
	lggr := logger.Sugared(logger.Test(t))

	t.Run("returns event data after discriminator", func(t *testing.T) {
		disc := types.AnchorCPIEventDiscriminator()
		payload := []byte{0x01, 0x02, 0x03, 0x04}
		data := append(disc[:], payload...)

		result, ok := ExtractAnchorCPIEventData(lggr, data)
		require.True(t, ok)
		require.Equal(t, payload, result)
	})

	t.Run("rejects data too short", func(t *testing.T) {
		data := make([]byte, MethodDiscriminatorLen)
		result, ok := ExtractAnchorCPIEventData(lggr, data)
		require.False(t, ok)
		require.Nil(t, result)
	})
}

func TestExtractVecCPIEventData(t *testing.T) {
	lggr := logger.Sugared(logger.Test(t))

	sourceProgram := newRandomPublicKey(t)
	destProgram := newRandomPublicKey(t)
	allAccountKeys := []solana.PublicKey{
		solana.PublicKey(sourceProgram),
		solana.PublicKey(destProgram),
	}
	programAtStackHeight := map[uint16]types.PublicKey{
		1: sourceProgram,
	}
	ix := rpc.CompiledInstruction{
		ProgramIDIndex: 1,
		StackHeight:    2,
	}

	t.Run("returns event data with valid vec prefix", func(t *testing.T) {
		disc := make([]byte, MethodDiscriminatorLen)
		payload := []byte{0xAA, 0xBB, 0xCC, 0xDD}
		vecLen := make([]byte, 4)
		binary.LittleEndian.PutUint32(vecLen, uint32(len(payload))) //nolint:gosec
		data := append(disc, append(vecLen, payload...)...)

		result, ok := ExtractVecCPIEventData(lggr, data, allAccountKeys, ix, programAtStackHeight, sourceProgram)
		require.True(t, ok)
		require.Equal(t, payload, result)
	})

	t.Run("rejects mismatched declared length", func(t *testing.T) {
		disc := make([]byte, MethodDiscriminatorLen)
		payload := []byte{0xAA, 0xBB, 0xCC, 0xDD}
		vecLen := make([]byte, 4)
		binary.LittleEndian.PutUint32(vecLen, uint32(len(payload)+10)) //nolint:gosec
		data := append(disc, append(vecLen, payload...)...)

		result, ok := ExtractVecCPIEventData(lggr, data, allAccountKeys, ix, programAtStackHeight, sourceProgram)
		require.False(t, ok)
		require.Nil(t, result)
	})

	t.Run("rejects zero declared length", func(t *testing.T) {
		disc := make([]byte, MethodDiscriminatorLen)
		payload := []byte{0xAA, 0xBB, 0xCC, 0xDD}
		vecLen := []byte{0x00, 0x00, 0x00, 0x00}
		data := append(disc, append(vecLen, payload...)...)

		result, ok := ExtractVecCPIEventData(lggr, data, allAccountKeys, ix, programAtStackHeight, sourceProgram)
		require.False(t, ok)
		require.Nil(t, result)
	})

	t.Run("rejects data shorter than legacy offset", func(t *testing.T) {
		data := make([]byte, CPIEventDataOffsetLegacy-1)
		result, ok := ExtractVecCPIEventData(lggr, data, allAccountKeys, ix, programAtStackHeight, sourceProgram)
		require.False(t, ok)
		require.Nil(t, result)
	})
}
