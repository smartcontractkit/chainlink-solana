package utils

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/stretchr/testify/require"
)

var ZeroAddress = [32]byte{}

func MakeRandom32ByteArray() [32]byte {
	a := make([]byte, 32)
	if _, err := rand.Read(a); err != nil {
		panic(err) // should never panic but check in case
	}
	return [32]byte(a)
}

func Uint64ToLE(chain uint64) []byte {
	chainLE := make([]byte, 8)
	binary.LittleEndian.PutUint64(chainLE, chain)
	return chainLE
}

func To28BytesLE(value uint64) [28]byte {
	le := make([]byte, 28)
	binary.LittleEndian.PutUint64(le, value)
	return [28]byte(le)
}

func Map[T, V any](ts []T, fn func(T) V) []V {
	result := make([]V, len(ts))
	for i, t := range ts {
		result[i] = fn(t)
	}
	return result
}

const DiscriminatorLength = 8

func Discriminator(namespace, name string) [DiscriminatorLength]byte {
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%s:%s", namespace, name)))
	return [DiscriminatorLength]byte(h.Sum(nil)[:DiscriminatorLength])
}

func FundAccounts(ctx context.Context, accounts []solana.PrivateKey, solanaGoClient *rpc.Client, t *testing.T) {
	sigs := []solana.Signature{}
	for _, v := range accounts {
		sig, err := solanaGoClient.RequestAirdrop(ctx, v.PublicKey(), 1000*solana.LAMPORTS_PER_SOL, rpc.CommitmentFinalized)
		require.NoError(t, err)
		sigs = append(sigs, sig)
	}

	// wait for confirmation so later transactions don't fail
	remaining := len(sigs)
	count := 0
	for remaining > 0 {
		count++
		statusRes, sigErr := solanaGoClient.GetSignatureStatuses(ctx, true, sigs...)
		require.NoError(t, sigErr)
		require.NotNil(t, statusRes)
		require.NotNil(t, statusRes.Value)

		unconfirmedTxCount := 0
		for _, res := range statusRes.Value {
			if res == nil || res.ConfirmationStatus == rpc.ConfirmationStatusProcessed || res.ConfirmationStatus == rpc.ConfirmationStatusConfirmed {
				unconfirmedTxCount++
			}
		}
		remaining = unconfirmedTxCount

		time.Sleep(500 * time.Millisecond)
		if count > 60 {
			require.NoError(t, fmt.Errorf("unable to find transaction within timeout"))
		}
	}
}

func IsEvent(event string, data []byte) bool {
	if len(data) < 8 {
		return false
	}
	d := Discriminator("event", event)
	return bytes.Equal(d[:], data[:8])
}

func ParseEvent(logs []string, event string, obj interface{}, print ...bool) error {
	for _, v := range logs {
		if strings.Contains(v, "Program data:") {
			encodedData := strings.TrimSpace(strings.TrimPrefix(v, "Program data:"))
			data, err := base64.StdEncoding.DecodeString(encodedData)
			if err != nil {
				return err
			}
			if IsEvent(event, data) {
				if err := bin.UnmarshalBorsh(obj, data); err != nil {
					return err
				}

				if len(print) > 0 && print[0] {
					fmt.Printf("%s: %+v\n", event, obj)
				}
				return nil
			}
		}
	}
	return fmt.Errorf("%s: event not found", event)
}

func ParseMultipleEvents[T any](logs []string, event string, print bool) ([]T, error) {
	var results []T
	for _, v := range logs {
		if strings.Contains(v, "Program data:") {
			encodedData := strings.TrimSpace(strings.TrimPrefix(v, "Program data:"))
			data, err := base64.StdEncoding.DecodeString(encodedData)
			if err != nil {
				return nil, err
			}
			if IsEvent(event, data) {
				var obj T
				if err := bin.UnmarshalBorsh(&obj, data); err != nil {
					return nil, err
				}

				if print {
					fmt.Printf("%s: %+v\n", event, obj)
				}

				results = append(results, obj)
			}
		}
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("%s: event not found", event)
	}

	return results, nil
}

type AnchorInstruction struct {
	Name         string
	ProgramID    string
	Logs         []string
	ComputeUnits int
	InnerCalls   []*AnchorInstruction
}

// Parses the log messages from an Anchor program and returns a list of AnchorInstructions.
func ParseLogMessages(logMessages []string) []*AnchorInstruction {
	var instructions []*AnchorInstruction
	var stack []*AnchorInstruction
	var currentInstruction *AnchorInstruction

	programInvokeRegex := regexp.MustCompile(`Program (\w+) invoke`)
	programSuccessRegex := regexp.MustCompile(`Program (\w+) success`)
	computeUnitsRegex := regexp.MustCompile(`Program (\w+) consumed (\d+) of \d+ compute units`)

	for _, line := range logMessages {
		line = strings.TrimSpace(line)

		// Program invocation - push to stack
		if match := programInvokeRegex.FindStringSubmatch(line); len(match) > 1 {
			newInstruction := &AnchorInstruction{
				ProgramID:    match[1],
				Name:         "",
				Logs:         []string{},
				ComputeUnits: 0,
				InnerCalls:   []*AnchorInstruction{},
			}

			if len(stack) == 0 {
				instructions = append(instructions, newInstruction)
			} else {
				stack[len(stack)-1].InnerCalls = append(stack[len(stack)-1].InnerCalls, newInstruction)
			}

			stack = append(stack, newInstruction)
			currentInstruction = newInstruction
			continue
		}

		// Program success - pop from stack
		if match := programSuccessRegex.FindStringSubmatch(line); len(match) > 1 {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1] // pop
				if len(stack) > 0 {
					currentInstruction = stack[len(stack)-1]
				} else {
					currentInstruction = nil
				}
			}
			continue
		}

		// Instruction name
		if strings.Contains(line, "Instruction:") {
			if currentInstruction != nil {
				currentInstruction.Name = strings.TrimSpace(strings.Split(line, "Instruction:")[1])
			}
			continue
		}

		// Program logs
		if strings.HasPrefix(line, "Program log:") {
			if currentInstruction != nil {
				logMessage := strings.TrimSpace(strings.TrimPrefix(line, "Program log:"))
				currentInstruction.Logs = append(currentInstruction.Logs, logMessage)
			}
			continue
		}

		// Compute units
		if match := computeUnitsRegex.FindStringSubmatch(line); len(match) > 1 {
			programID := match[1]
			computeUnits, _ := strconv.Atoi(match[2])

			// Find the instruction in the stack that matches this program ID
			for i := len(stack) - 1; i >= 0; i-- {
				if stack[i].ProgramID == programID {
					stack[i].ComputeUnits = computeUnits
					break
				}
			}
		}
	}

	return instructions
}

// Pretty prints the given Anchor instructions.
// Example usage:
// parsed := utils.ParseLogMessages(result.Meta.LogMessages)
// output := utils.PrintInstructions(parsed)
// t.Logf("Parsed Instructions: %s", output)
func PrintInstructions(instructions []*AnchorInstruction) string {
	var output strings.Builder

	var printInstruction func(*AnchorInstruction, int, string)
	printInstruction = func(instruction *AnchorInstruction, index int, indent string) {
		output.WriteString(fmt.Sprintf("%sInstruction %d: %s\n", indent, index, instruction.Name))
		output.WriteString(fmt.Sprintf("%s  Program ID: %s\n", indent, instruction.ProgramID))
		output.WriteString(fmt.Sprintf("%s  Compute Units: %d\n", indent, instruction.ComputeUnits))
		output.WriteString(fmt.Sprintf("%s  Logs:\n", indent))
		for _, log := range instruction.Logs {
			output.WriteString(fmt.Sprintf("%s    %s\n", indent, log))
		}
		if len(instruction.InnerCalls) > 0 {
			output.WriteString(fmt.Sprintf("%s  Inner Calls:\n", indent))
			for i, innerCall := range instruction.InnerCalls {
				printInstruction(innerCall, i+1, indent+"    ")
			}
		}
	}

	for i, instruction := range instructions {
		printInstruction(instruction, i+1, "")
	}

	return output.String()
}

func GetBlockTime(ctx context.Context, client *rpc.Client, commitment rpc.CommitmentType) (*solana.UnixTimeSeconds, error) {
	block, err := client.GetBlockHeight(ctx, commitment)
	if err != nil {
		return nil, fmt.Errorf("failed to get block height: %w", err)
	}

	blockTime, err := client.GetBlockTime(ctx, block)
	if err != nil {
		return nil, fmt.Errorf("failed to get block time: %w", err)
	}

	return blockTime, nil
}
