package fees

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"golang.org/x/exp/constraints"
)

// https://github.com/solana-labs/solana/blob/60858d043ca612334de300805d93ea3014e8ab37/sdk/src/compute_budget.rs#L25
const (
	// deprecated: will not support for building instruction
	InstructionRequestUnitsDeprecated computeBudgetInstruction = iota

	// Request a specific transaction-wide program heap region size in bytes.
	// The value requested must be a multiple of 1024. This new heap region
	// size applies to each program executed in the transaction, including all
	// calls to CPIs.
	// note: uses ag_binary.Varuint32
	InstructionRequestHeapFrame

	// Set a specific compute unit limit that the transaction is allowed to consume.
	// note: uses ag_binary.Varuint32
	InstructionSetComputeUnitLimit

	// Set a compute unit price in "micro-lamports" to pay a higher transaction
	// fee for higher transaction prioritization.
	// note: uses ag_binary.Uint64
	InstructionSetComputeUnitPrice
)

var (
	ComputeBudgetProgram = solana.MustPublicKeyFromBase58("ComputeBudget111111111111111111111111111111")
)

type computeBudgetInstruction uint8

func (ins computeBudgetInstruction) String() (out string) {
	out = "INVALID"
	switch ins {
	case InstructionRequestUnitsDeprecated:
		out = "RequestUnitsDeprecated"
	case InstructionRequestHeapFrame:
		out = "RequestHeapFrame"
	case InstructionSetComputeUnitLimit:
		out = "SetComputeUnitLimit"
	case InstructionSetComputeUnitPrice:
		out = "SetComputeUnitPrice"
	}
	return out
}

// instruction is an internal interface for encoding instruction data
type instruction interface {
	Data() ([]byte, error)
	Selector() computeBudgetInstruction
}

// https://docs.solana.com/developing/programming-model/runtime
type ComputeUnitPrice uint64

// simple encoding into program expected format
func (val ComputeUnitPrice) Data() ([]byte, error) {
	return encode(InstructionSetComputeUnitPrice, val)
}

func (val ComputeUnitPrice) Selector() computeBudgetInstruction {
	return InstructionSetComputeUnitPrice
}

type ComputeUnitLimit uint32

func (val ComputeUnitLimit) Data() ([]byte, error) {
	return encode(InstructionSetComputeUnitLimit, val)
}

func (val ComputeUnitLimit) Selector() computeBudgetInstruction {
	return InstructionSetComputeUnitLimit
}

// encode combines the identifier and little encoded value into a byte array
func encode[V constraints.Unsigned](identifier computeBudgetInstruction, val V) ([]byte, error) {
	buf := new(bytes.Buffer)

	// encode method identifier
	if err := buf.WriteByte(uint8(identifier)); err != nil {
		return []byte{}, err
	}

	// encode value
	if err := binary.Write(buf, binary.LittleEndian, val); err != nil {
		return []byte{}, err
	}

	return buf.Bytes(), nil
}

func ParseComputeUnitPrice(data []byte) (ComputeUnitPrice, error) {
	v, err := parse(InstructionSetComputeUnitPrice, data, binary.LittleEndian.Uint64)
	return ComputeUnitPrice(v), err
}

func ParseComputeUnitLimit(data []byte) (ComputeUnitLimit, error) {
	v, err := parse(InstructionSetComputeUnitLimit, data, binary.LittleEndian.Uint32)
	return ComputeUnitLimit(v), err
}

// parse implements tx data parsing for the provided instruction type and specified decoder
func parse[V constraints.Unsigned](ins computeBudgetInstruction, data []byte, decoder func([]byte) V) (V, error) {
	if len(data) != (1 + binary.Size(V(0))) { // instruction byte + uintXXX length
		return 0, fmt.Errorf("invalid length: %d", len(data))
	}

	// validate instruction identifier
	if data[0] != uint8(ins) {
		return 0, fmt.Errorf("not %s identifier: %d", ins, data[0])
	}

	// guarantees length to fit the binary decoder
	return decoder(data[1:]), nil
}

// modifies passed in tx to set compute unit price
func SetComputeUnitPrice(tx *solana.Transaction, value ComputeUnitPrice) error {
	return set(tx, value, true) // data feeds expects SetComputeUnitPrice instruction to be right before report instruction
}

func SetComputeUnitLimit(tx *solana.Transaction, value ComputeUnitLimit) error {
	return set(tx, value, false) // appends instruction to the end
}

// set adds or modifies instructions for the compute budget program
func set(tx *solana.Transaction, baseData instruction, appendToFront bool) error {
	// find ComputeBudget program to accounts if it exists
	// reimplements HasAccount to retrieve index: https://github.com/gagliardetto/solana-go/blob/618f56666078f8131a384ab27afd918d248c08b7/message.go#L233
	var exists bool
	var programIdx int
	for i, a := range tx.Message.AccountKeys {
		if a.Equals(ComputeBudgetProgram) {
			exists = true
			programIdx = i
			break
		}
	}
	// if it doesn't exist, add to account keys
	if !exists {
		tx.Message.AccountKeys = append(tx.Message.AccountKeys, ComputeBudgetProgram)
		programIdx = len(tx.Message.AccountKeys) - 1 // last index of account keys

		// https://github.com/gagliardetto/solana-go/blob/618f56666078f8131a384ab27afd918d248c08b7/transaction.go#L293
		tx.Message.Header.NumReadonlyUnsignedAccounts++

		// lookup table addresses are indexed after the tx.Message.AccountKeys
		// higher indices must be increased
		// https://github.com/gagliardetto/solana-go/blob/da2193071f56059aa35010a239cece016c4e827f/transaction.go#L440
		for i, ix := range tx.Message.Instructions {
			for j, v := range ix.Accounts {
				if int(v) >= programIdx {
					tx.Message.Instructions[i].Accounts[j]++
				}
			}
		}
	}

	// get instruction data
	data, err := baseData.Data()
	if err != nil {
		return err
	}

	// compiled instruction
	instruction := solana.CompiledInstruction{
		ProgramIDIndex: uint16(programIdx), //nolint:gosec // max value would exceed tx size
		Data:           data,
	}

	// check if there is an instruction for setcomputeunitprice
	var found bool
	var instructionIdx int
	for i := range tx.Message.Instructions {
		if int(tx.Message.Instructions[i].ProgramIDIndex) == programIdx &&
			len(tx.Message.Instructions[i].Data) > 0 &&
			tx.Message.Instructions[i].Data[0] == uint8(baseData.Selector()) {
			found = true
			instructionIdx = i
			break
		}
	}

	if found {
		tx.Message.Instructions[instructionIdx] = instruction
	} else {
		if appendToFront {
			tx.Message.Instructions = append([]solana.CompiledInstruction{instruction}, tx.Message.Instructions...)
		} else {
			tx.Message.Instructions = append(tx.Message.Instructions, instruction)
		}
	}

	return nil
}
