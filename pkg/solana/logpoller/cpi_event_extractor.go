package logpoller

import (
	"encoding/base64"
	bin "encoding/binary"
	"fmt"
	"sync"

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
	e.lggr.Infow("[DEBUG] CPI filter added",
		"filterName", filter.Name,
		"sourceProgram", filter.Address.ToSolana().String(),
		"destProgram", filter.ExtraFilterConfig.DestProgram.ToSolana().String(),
		"methodSig", fmt.Sprintf("%x", filter.ExtraFilterConfig.MethodSignature),
		"totalRegistered", len(e.registered),
	)
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
		e.lggr.Debugw("[DEBUG] ExtractCPIEvents: no inner instructions",
			"slot", detail.slotNumber, "metaNil", meta == nil)
		return nil
	}

	allAccountKeys := getAllAccountKeys(tx, meta)
	if len(allAccountKeys) == 0 {
		e.lggr.Debugw("[DEBUG] ExtractCPIEvents: no account keys", "slot", detail.slotNumber)
		return nil
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	e.lggr.Infow("[DEBUG] ExtractCPIEvents: processing transaction",
		"slot", detail.slotNumber,
		"txSig", detail.trxSig.String(),
		"innerInstructionSets", len(meta.InnerInstructions),
		"registeredFilters", len(e.registered),
		"accountKeys", len(allAccountKeys),
	)

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

		e.lggr.Infow("[DEBUG] ExtractCPIEvents: processing inner instruction set",
			"slot", detail.slotNumber,
			"outerIndex", inner.Index,
			"outerProgram", outerProgram.ToSolana().String(),
			"innerIxCount", len(inner.Instructions),
		)

		for ixIdx, ix := range inner.Instructions {
			if int(ix.ProgramIDIndex) >= len(allAccountKeys) {
				e.lggr.Warnf("program ID index out of range: %d, len(allAccountKeys): %d", ix.ProgramIDIndex, len(allAccountKeys))
				continue
			}

			destProgram := types.PublicKey(allAccountKeys[ix.ProgramIDIndex])
			if ix.StackHeight > 0 {
				programAtStackHeight[ix.StackHeight] = destProgram
			}

			e.lggr.Infow("[DEBUG] ExtractCPIEvents: inner ix",
				"slot", detail.slotNumber,
				"ixIdx", ixIdx,
				"destProgram", destProgram.ToSolana().String(),
				"stackHeight", ix.StackHeight,
				"dataLen", len(ix.Data),
			)

			if len(ix.Data) <= MethodDiscriminatorLen {
				e.lggr.Debugw("[DEBUG] ExtractCPIEvents: data too short for method disc",
					"slot", detail.slotNumber, "dataLen", len(ix.Data))
				continue
			}

			methodSig := types.EventSignature(ix.Data[:MethodDiscriminatorLen])
			anchorDisc := types.AnchorCPIEventDiscriminator()

			e.lggr.Infow("[DEBUG] ExtractCPIEvents: method discriminator check",
				"slot", detail.slotNumber,
				"methodSigHex", fmt.Sprintf("%x", methodSig),
				"anchorDiscHex", fmt.Sprintf("%x", anchorDisc),
				"isAnchor", methodSig == anchorDisc,
			)

			var eventData []byte
			var ok bool
			if methodSig == types.AnchorCPIEventDiscriminator() {
				eventData, ok = extractAnchorCPIEventData(ix.Data)
			} else {
				eventData, ok = extractVecCPIEventData(ix.Data)
			}
			if !ok || len(eventData) == 0 {
				e.lggr.Debugw("[DEBUG] ExtractCPIEvents: event data extraction failed or empty",
					"slot", detail.slotNumber, "ok", ok, "eventDataLen", len(eventData))
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
			} else {
				sourceProgram = outerProgram
			}

			key := cpiFilterKey{
				sourceProgram: sourceProgram,
				destProgram:   destProgram,
				methodSig:     methodSig,
			}

			e.lggr.Infow("[DEBUG] ExtractCPIEvents: filter key lookup",
				"slot", detail.slotNumber,
				"sourceProgram", sourceProgram.ToSolana().String(),
				"destProgram", destProgram.ToSolana().String(),
				"methodSigHex", fmt.Sprintf("%x", methodSig),
				"eventDataLen", len(eventData),
			)

			if _, ok := e.registered[key]; !ok {
				e.lggr.Warnw("[DEBUG] ExtractCPIEvents: NO matching registered filter",
					"slot", detail.slotNumber,
					"sourceProgram", sourceProgram.ToSolana().String(),
					"destProgram", destProgram.ToSolana().String(),
					"methodSigHex", fmt.Sprintf("%x", methodSig),
				)
				for rk := range e.registered {
					e.lggr.Infow("[DEBUG] ExtractCPIEvents: registered key dump",
						"regSource", rk.sourceProgram.ToSolana().String(),
						"regDest", rk.destProgram.ToSolana().String(),
						"regMethodHex", fmt.Sprintf("%x", rk.methodSig),
					)
				}
				continue
			}

			encodedData := base64.StdEncoding.EncodeToString(eventData)

			e.lggr.Infow("Found CPI event",
				"sourceProgram", sourceProgram.ToSolana().String(),
				"destProgram", allAccountKeys[ix.ProgramIDIndex].String(),
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
func extractAnchorCPIEventData(data []byte) ([]byte, bool) {
	if len(data) <= CPIEventDataOffsetCurrent {
		return nil, false
	}
	return data[CPIEventDataOffsetCurrent:], true
}

// extractVecCPIEventData handles the Borsh Vec<u8> format used by CCIP's cpi_event and
// Anchor <=0.29: [method_disc(8)][vec_len(4)][event_data(N)].
// Validation is strict: declaredLen must be >0 and must exactly equal the remaining bytes.
// Returns (nil, false) on any mismatch -- no fallback.
func extractVecCPIEventData(data []byte) ([]byte, bool) {
	if len(data) < CPIEventDataOffsetLegacy {
		return nil, false
	}
	declaredLen := bin.LittleEndian.Uint32(data[MethodDiscriminatorLen:CPIEventDataOffsetLegacy])
	remaining := len(data) - CPIEventDataOffsetLegacy
	if declaredLen == 0 || int(declaredLen) != remaining {
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
