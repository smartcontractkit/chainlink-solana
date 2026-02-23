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
	MethodDiscriminatorLen = 8
	VecLengthPrefixLen     = 4
	CPIEventDataOffset     = MethodDiscriminatorLen + VecLengthPrefixLen
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

		programAtStackHeight := map[uint16]types.PublicKey{
			1: types.PublicKey(allAccountKeys[outerInstruction.ProgramIDIndex]),
		}

		for _, ix := range inner.Instructions {
			if int(ix.ProgramIDIndex) >= len(allAccountKeys) {
				e.lggr.Warnf("program ID index out of range: %d, len(allAccountKeys): %d", ix.ProgramIDIndex, len(allAccountKeys))
				continue
			}

			destProgram := types.PublicKey(allAccountKeys[ix.ProgramIDIndex])
			programAtStackHeight[ix.StackHeight] = destProgram
			if len(ix.Data) < CPIEventDataOffset {
				e.lggr.Warnw("data shorter than cpiEventDataOffset", "dataLen", len(ix.Data), "required", CPIEventDataOffset)
				continue
			}

			declaredLen := bin.LittleEndian.Uint32(ix.Data[MethodDiscriminatorLen:CPIEventDataOffset])
			if declaredLen == 0 {
				e.lggr.Warnw("cpi event vec length is zero",
					"sourceProgram", programAtStackHeight[ix.StackHeight-1].ToSolana().String(),
					"destProgram", allAccountKeys[ix.ProgramIDIndex].String(),
				)
				continue
			}

			remaining := len(ix.Data) - CPIEventDataOffset
			if int(declaredLen) > remaining {
				e.lggr.Warnw("cpi event vec length exceeds remaining bytes",
					"declaredLen", declaredLen, "remaining", remaining,
					"sourceProgram", programAtStackHeight[ix.StackHeight-1].ToSolana().String(),
					"destProgram", allAccountKeys[ix.ProgramIDIndex].String(),
				)
				continue
			}

			if int(declaredLen) != remaining {
				e.lggr.Warnw("cpi event vec length does not match remaining bytes",
					"declaredLen", declaredLen, "remaining", remaining,
					"sourceProgram", programAtStackHeight[ix.StackHeight-1].ToSolana().String(),
					"destProgram", allAccountKeys[ix.ProgramIDIndex].String(),
				)
				continue
			}

			methodSig := types.EventSignature(ix.Data[:MethodDiscriminatorLen])

			if ix.StackHeight <= 1 {
				e.lggr.Warnw("unexpected stack height for inner instruction",
					"ix", ix,
					"destProgram", destProgram.ToSolana(),
					"methodSig", methodSig,
					"innerIndex", inner.Index,
				)
				continue
			}

			sourceProgram, ok := programAtStackHeight[ix.StackHeight-1]
			if !ok {
				e.lggr.Warnw("could not find caller for instruction", "stackHeight", ix.StackHeight)
				continue
			}

			key := cpiFilterKey{
				sourceProgram: sourceProgram,
				destProgram:   destProgram,
				methodSig:     methodSig,
			}

			if _, ok := e.registered[key]; !ok {
				continue
			}

			eventData := ix.Data[CPIEventDataOffset : CPIEventDataOffset+int(declaredLen)]
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
