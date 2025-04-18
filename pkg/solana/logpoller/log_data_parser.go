package logpoller

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/gagliardetto/solana-go"
)

const PROGRAM_LOG = "Program log: "
const PROGRAM_DATA = "Program data: "

var (
	invokeMatcher   = regexp.MustCompile(`^Program (\w*) invoke \[(\d)\]`)
	consumedMatcher = regexp.MustCompile(`^Program \w* consumed (\d*) (.*)`)
)

type BlockData struct {
	SlotNumber          uint64
	BlockHeight         uint64
	BlockHash           solana.Hash
	BlockTime           solana.UnixTimeSeconds
	TransactionHash     solana.Signature
	TransactionIndex    int
	TransactionLogIndex uint
	Error               interface{}
}

type ProgramLog struct {
	BlockData
	Text   string
	Prefix string
}

type ProgramEvent struct {
	Program string
	BlockData
	Data string
}

type ProgramOutput struct {
	Program      string
	Logs         []ProgramLog
	Events       []ProgramEvent
	ComputeUnits uint
	Truncated    bool
	Failed       bool
	ErrorText    string
}

func prefixBuilder(depth int) string {
	return strings.Repeat(">", depth)
}

func ParseProgramLogs(logs []string) []ProgramOutput {
	var depth int
	programs := []string{}
	instLogs := []ProgramOutput{}

	output := &ProgramOutput{}

	for _, log := range logs {
		if strings.HasPrefix(log, PROGRAM_LOG) {
			if output == nil {
				continue
			}
			logData := log[len(PROGRAM_LOG):]

			// this is a general log
			output.Logs = append(output.Logs, ProgramLog{
				Prefix: prefixBuilder(depth),
				Text:   logData,
			})
		} else if strings.HasPrefix(log, PROGRAM_DATA) {
			if output == nil {
				continue
			}

			logData := log[len(PROGRAM_DATA):]

			txLogIdx := uint(len(output.Events))
			output.Events = append(output.Events, ProgramEvent{
				Program: programs[len(programs)-1],
				Data:    logData,
				BlockData: BlockData{
					TransactionLogIndex: txLogIdx,
				},
			})
		} else if strings.HasPrefix(log, "Log truncated") {
			if output == nil {
				continue
			}

			output.Truncated = true
		} else {
			matches := invokeMatcher.FindStringSubmatch(log)

			if len(matches) > 0 {
				if depth == 0 {
					instLogs = append(instLogs, ProgramOutput{
						Program: matches[1],
					})
					output = &instLogs[len(instLogs)-1]
				}

				depth++
				programs = append(programs, matches[1])
			} else if strings.Contains(log, "success") {
				depth--
				programs = programs[:len(programs)-1]
			} else if strings.Contains(log, "failed") {
				if output == nil {
					continue
				}

				output.Failed = true

				idx := strings.Index(log, ": ") + 2

				// failed to verify log of previous program so reset depth and print full log
				if strings.HasPrefix(log, "failed") {
					depth++
				}

				output.ErrorText = log[idx:]

				depth--
				programs = programs[:len(programs)-1]
			} else {
				if depth == 0 {
					instLogs = append(instLogs, ProgramOutput{})
					output = &instLogs[len(instLogs)-1]
				}

				if output == nil {
					continue
				}

				matches := consumedMatcher.FindStringSubmatch(log)
				// we only care about toplevel compute units cost
				if len(matches) == 3 && depth == 1 {
					if val, err := strconv.Atoi(matches[1]); err == nil {
						output.ComputeUnits = uint(val) //nolint:gosec
					}
				}
			}
		}
	}

	return instLogs
}
