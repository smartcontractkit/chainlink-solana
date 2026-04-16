package logpoller

import (
	"encoding/base64"
	"sync"

	bin "encoding/binary"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
)

const (
	MethodDiscriminatorLen    = 8
	VecLengthPrefixLen        = 4
	CPIEventDataOffsetLegacy  = MethodDiscriminatorLen + VecLengthPrefixLen
	CPIEventDataOffsetCurrent = MethodDiscriminatorLen
)

type cpiFilterKey struct {
	sourceProgram types.PublicKey
	destProgram   types.PublicKey
	methodSig     types.EventSignature
}

type CPIEventExtractor struct {
	mu         sync.RWMutex
	registered map[cpiFilterKey]struct{}
	lggr       logger.SugaredLogger
}

func NewCPIEventExtractor(lggr logger.SugaredLogger) *CPIEventExtractor {
	return &CPIEventExtractor{
		registered: make(map[cpiFilterKey]struct{}),
		lggr:       lggr,
	}
}

func (e *CPIEventExtractor) AddFilter(filter types.Filter) {
	if !filter.IsCPIFilter() {
		return
	}

	key := cpiFilterKey{
		sourceProgram: filter.Address,
		destProgram:   filter.ExtraFilterConfig.DestProgram,
		methodSig:     filter.ExtraFilterConfig.MethodSignature,
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	e.registered[key] = struct{}{}
}

func (e *CPIEventExtractor) RemoveFilter(filter types.Filter) {
	if !filter.IsCPIFilter() {
		return
	}

	key := cpiFilterKey{
		sourceProgram: filter.Address,
		destProgram:   filter.ExtraFilterConfig.DestProgram,
		methodSig:     filter.ExtraFilterConfig.MethodSignature,
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	delete(e.registered, key)
}

func (e *CPIEventExtractor) HasCPIFilters() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.registered) > 0
}

func (e *CPIEventExtractor) ExtractCPIEvents(
	tx *solana.Transaction,
	meta *rpc.TransactionMeta,
	detail eventDetail,
	logIdxOffset uint,
) []types.ProgramEvent {
	if meta == nil || len(meta.InnerInstructions) == 0 {
		return nil
	}

	allAccountKeys := getAllAccountKeys(tx, meta)
	if len(allAccountKeys) == 0 {
		return nil
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	var events []types.ProgramEvent
	logIdx := logIdxOffset

	for _, inner := range meta.InnerInstructions {
		if int(inner.Index) >= len(tx.Message.Instructions) {
			e.lggr.Warnw("inner instruction index out of range", "index", inner.Index, "numInstructions", len(tx.Message.Instructions))
			continue
		}

		outerInstruction := tx.Message.Instructions[inner.Index]
		if int(outerInstruction.ProgramIDIndex) >= len(allAccountKeys) {
			e.lggr.Warnw("outer instruction program ID index out of range", "index", outerInstruction.ProgramIDIndex, "numKeys", len(allAccountKeys))
			continue
		}

		outerProgram := types.PublicKey(allAccountKeys[outerInstruction.ProgramIDIndex])
		programAtStackHeight := map[uint16]types.PublicKey{
			1: outerProgram,
		}

		for _, ix := range inner.Instructions {
			if int(ix.ProgramIDIndex) >= len(allAccountKeys) {
				e.lggr.Warnf("program ID index out of range: %d, len(allAccountKeys): %d", ix.ProgramIDIndex, len(allAccountKeys))
				continue
			}

			destProgram := types.PublicKey(allAccountKeys[ix.ProgramIDIndex])
			if ix.StackHeight > 0 {
				programAtStackHeight[ix.StackHeight] = destProgram
			}

			if len(ix.Data) <= MethodDiscriminatorLen {
				continue
			}

			methodSig := types.EventSignature(ix.Data[:MethodDiscriminatorLen])

			var eventData []byte
			var ok bool
			if methodSig == types.AnchorCPIEventDiscriminator() {
				eventData, ok = extractAnchorCPIEventData(e.lggr, ix.Data)
			} else {
				eventData, ok = extractVecCPIEventData(e.lggr, ix.Data, allAccountKeys, ix, programAtStackHeight)
			}
			if !ok || len(eventData) == 0 {
				continue
			}

			// Determine the source program: use StackHeight tracking when available,
			// fall back to the outer instruction's program when StackHeight is missing (0).
			var sourceProgram types.PublicKey
			if ix.StackHeight > 1 {
				sp, ok := programAtStackHeight[ix.StackHeight-1]
				if !ok {
					e.lggr.Warnw("could not find caller for instruction", "stackHeight", ix.StackHeight)
					continue
				}
				sourceProgram = sp
			} else if ix.StackHeight == 1 {
				e.lggr.Warnw("unexpected stack height for inner instruction",
					"ix", ix,
					"destProgram", destProgram.ToSolana(),
					"methodSig", methodSig,
					"innerIndex", inner.Index,
				)
				continue
			} else {
				sourceProgram = outerProgram
			}

			key := cpiFilterKey{
				sourceProgram: sourceProgram,
				destProgram:   destProgram,
				methodSig:     methodSig,
			}

			if _, ok := e.registered[key]; !ok {
				continue
			}

			encodedData := base64.StdEncoding.EncodeToString(eventData)

			e.lggr.Infow("Found CPI event",
				"sourceProgram", sourceProgram.ToSolana().String(),
				"destProgram", allAccountKeys[ix.ProgramIDIndex].String(),
				"loadedWritableAddresses", meta.LoadedAddresses.Writable,
				"loadedReadOnlyAddresses", meta.LoadedAddresses.ReadOnly,
			)

			event := types.ProgramEvent{
				Program: sourceProgram.ToSolana().String(),
				BlockData: types.BlockData{
					SlotNumber:          detail.slotNumber,
					BlockHeight:         detail.blockHeight,
					BlockHash:           detail.blockHash,
					BlockTime:           detail.blockTime,
					TransactionHash:     detail.trxSig,
					TransactionIndex:    detail.trxIdx,
					TransactionLogIndex: logIdx,
					Error:               detail.err,
				},
				Data:  encodedData,
				IsCPI: true,
			}

			events = append(events, event)
			logIdx++
		}
	}

	return events
}

// extractAnchorCPIEventData handles Anchor 0.31+ emit_cpi! format: [method_disc(8)][event_data(N)].
// Event data directly follows the 8-byte method discriminator with no vec prefix.
func extractAnchorCPIEventData(lggr logger.SugaredLogger, data []byte) ([]byte, bool) {
	if len(data) <= CPIEventDataOffsetCurrent {
		lggr.Warnw("anchor CPI event data shorter than method discriminator", "dataLen", len(data), "required", CPIEventDataOffsetCurrent+1)
		return nil, false
	}
	return data[CPIEventDataOffsetCurrent:], true
}

// extractVecCPIEventData handles the Borsh Vec<u8> format used by CCIP's cpi_event and
// Anchor <=0.29: [method_disc(8)][vec_len(4)][event_data(N)].
// Validation is strict: declaredLen must be >0 and must exactly equal the remaining bytes.
// Returns (nil, false) on any mismatch -- no fallback.
func extractVecCPIEventData(
	lggr logger.SugaredLogger,
	data []byte,
	allAccountKeys []solana.PublicKey,
	ix rpc.CompiledInstruction,
	programAtStackHeight map[uint16]types.PublicKey,
) ([]byte, bool) {
	if len(data) < CPIEventDataOffsetLegacy {
		lggr.Warnw("data shorter than cpiEventDataOffset", "dataLen", len(data), "required", CPIEventDataOffsetLegacy)
		return nil, false
	}

	declaredLen := bin.LittleEndian.Uint32(data[MethodDiscriminatorLen:CPIEventDataOffsetLegacy])
	if declaredLen == 0 {
		lggr.Warnw("cpi event vec length is zero",
			"sourceProgram", programAtStackHeight[ix.StackHeight-1].ToSolana().String(),
			"destProgram", allAccountKeys[ix.ProgramIDIndex].String(),
		)
		return nil, false
	}

	remaining := len(data) - CPIEventDataOffsetLegacy
	if int(declaredLen) > remaining {
		lggr.Warnw("cpi event vec length exceeds remaining bytes",
			"declaredLen", declaredLen, "remaining", remaining,
			"sourceProgram", programAtStackHeight[ix.StackHeight-1].ToSolana().String(),
			"destProgram", allAccountKeys[ix.ProgramIDIndex].String(),
		)
		return nil, false
	}

	if int(declaredLen) != remaining {
		lggr.Warnw("cpi event vec length does not match remaining bytes",
			"declaredLen", declaredLen, "remaining", remaining,
			"sourceProgram", programAtStackHeight[ix.StackHeight-1].ToSolana().String(),
			"destProgram", allAccountKeys[ix.ProgramIDIndex].String(),
		)
		return nil, false
	}

	return data[CPIEventDataOffsetLegacy:], true
}

func getAllAccountKeys(tx *solana.Transaction, meta *rpc.TransactionMeta) []solana.PublicKey {
	if tx == nil {
		return nil
	}

	allKeys := make([]solana.PublicKey, 0, len(tx.Message.AccountKeys))
	allKeys = append(allKeys, tx.Message.AccountKeys...)

	if meta != nil && meta.LoadedAddresses.Writable != nil {
		allKeys = append(allKeys, meta.LoadedAddresses.Writable...)
	}
	if meta != nil && meta.LoadedAddresses.ReadOnly != nil {
		allKeys = append(allKeys, meta.LoadedAddresses.ReadOnly...)
	}

	return allKeys
}
