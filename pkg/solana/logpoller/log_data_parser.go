package logpoller

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/gagliardetto/solana-go"
)

type MessageStyle string

const (
	MessageStyleMuted   MessageStyle = "muted"
	MessageStyleInfo    MessageStyle = "info"
	MessageStyleSuccess MessageStyle = "success"
	MessageStyleWarning MessageStyle = "warning"
)

var (
	invokeMatcher      = regexp.MustCompile(`Program (\w*) invoke \[(\d)\]`)
	consumedMatcher    = regexp.MustCompile(`Program \w* consumed (\d*) (.*)`)
	logMatcher         = regexp.MustCompile(`Program log: (.*)`)
	dataMatcher        = regexp.MustCompile(`Program data: (.*)`)
	instructionMatcher = regexp.MustCompile(`Instruction: (.*)`)
)

type BlockData struct {
	BlockNumber         uint64
	BlockHash           solana.Hash
	TransactionHash     solana.Signature
	TransactionIndex    int
	TransactionLogIndex uint
}

type ProgramLog struct {
	BlockData
	Text   string
	Prefix string
	Style  MessageStyle
}

type ProgramEvent struct {
	BlockData
	Prefix       string
	FunctionName string
	Data         string
}

type ProgramOutput struct {
	Program      string
	Logs         []ProgramLog
	Events       []ProgramEvent
	ComputeUnits uint
	Truncated    bool
	Failed       bool
}

func prefixBuilder(depth int) string {
	return strings.Repeat(">", depth)
}

/*
Program J1zQwrBNBngz26jRPNWsUSZMHJwBwpkoDitXRV95LdK4 invoke [1]
Program log: Instruction: CreateLog
Program data: HDQnaQjSWwkNAAAASGVsbG8sIFdvcmxkISoAAAAAAAAA // base64 encoded; borsh encoded with identifier
Program J1zQwrBNBngz26jRPNWsUSZMHJwBwpkoDitXRV95LdK4 consumed 1477 of 200000 compute units
Program J1zQwrBNBngz26jRPNWsUSZMHJwBwpkoDitXRV95LdK4 success
*/
func parseProgramLogs(logs []string) []ProgramOutput {
	var depth int

	instLogs := []ProgramOutput{}
	lastEventIdx := -1
	lastLogIdx := -1

	for _, log := range logs {
		if strings.HasPrefix(log, "Program log:") {
			logDataMatches := logMatcher.FindStringSubmatch(log)

			if len(logDataMatches) <= 1 || lastLogIdx < 0 {
				continue
			}

			instructionMatches := instructionMatcher.FindStringSubmatch(logDataMatches[1])

			if len(instructionMatches) > 1 {
				// is an event which should be followed by: Program data: (.*)
				instLogs[lastLogIdx].Events = append(instLogs[lastLogIdx].Events, ProgramEvent{
					Prefix:       prefixBuilder(depth),
					FunctionName: instructionMatches[1],
				})

				lastEventIdx = len(instLogs[lastLogIdx].Events) - 1
			} else {
				// if contains: Instruction: (.*) this is an event and should be followed by: Program data:
				// else this is a log
				instLogs[lastLogIdx].Logs = append(instLogs[lastLogIdx].Logs, ProgramLog{
					Prefix: prefixBuilder(depth),
					Style:  MessageStyleMuted,
					Text:   log,
				})
			}
		} else if strings.HasPrefix(log, "Program data:") {
			if lastLogIdx < 0 {
				continue
			}

			dataMatches := dataMatcher.FindStringSubmatch(log)

			if len(dataMatches) > 1 {
				if lastEventIdx > -1 {
					instLogs[lastLogIdx].Events[lastEventIdx].Data = dataMatches[1]
				}
			}
		} else if strings.HasPrefix(log, "Log truncated") {
			if lastLogIdx < 0 {
				continue
			}

			instLogs[lastLogIdx].Truncated = true
		} else {
			matches := invokeMatcher.FindStringSubmatch(log)

			if len(matches) > 0 {
				if depth == 0 {
					instLogs = append(instLogs, ProgramOutput{
						ComputeUnits: 0,
						Failed:       false,
						Program:      matches[1],
						Logs:         []ProgramLog{},
						Truncated:    false,
					})

					lastLogIdx = len(instLogs) - 1
				} else {
					if lastLogIdx < 0 {
						continue
					}

					instLogs[lastLogIdx].Logs = append(instLogs[lastLogIdx].Logs, ProgramLog{
						Prefix: prefixBuilder(depth),
						Style:  MessageStyleInfo,
						Text:   fmt.Sprintf("Program invoked: %s", matches[1]),
					})
				}

				depth++
			} else if strings.Contains(log, "success") {
				if lastLogIdx < 0 {
					continue
				}

				instLogs[lastLogIdx].Logs = append(instLogs[lastLogIdx].Logs, ProgramLog{
					Prefix: prefixBuilder(depth),
					Style:  MessageStyleSuccess,
					Text:   "Program returned success",
				})

				depth--
			} else if strings.Contains(log, "failed") {
				if lastLogIdx < 0 {
					continue
				}

				instLogs[lastLogIdx].Failed = true

				idx := strings.Index(log, ": ") + 2
				currText := fmt.Sprintf(`Program returned error: "%s"`, log[idx:])

				// failed to verify log of previous program so reset depth and print full log
				if strings.HasPrefix(log, "failed") {
					depth++

					currText = strings.ToTitle(log)
				}

				instLogs[lastLogIdx].Logs = append(instLogs[lastLogIdx].Logs, ProgramLog{
					Prefix: prefixBuilder(depth),
					Style:  MessageStyleWarning,
					Text:   currText,
				})

				depth--
			} else {
				if depth == 0 {
					instLogs = append(instLogs, ProgramOutput{
						ComputeUnits: 0,
						Failed:       false,
						Program:      "",
						Logs:         []ProgramLog{},
						Truncated:    false,
					})

					lastLogIdx = len(instLogs) - 1
				}

				if lastLogIdx < 0 {
					continue
				}

				matches := consumedMatcher.FindStringSubmatch(log)
				if len(matches) == 3 {
					if depth == 1 {
						if val, err := strconv.Atoi(matches[1]); err == nil {
							instLogs[lastLogIdx].ComputeUnits = uint(val) //nolint:gosec
						}
					}

					log = fmt.Sprintf("Program consumed: %s %s", matches[1], matches[2])
				}

				// native program logs don't start with "Program log:"
				instLogs[lastLogIdx].Logs = append(instLogs[lastLogIdx].Logs, ProgramLog{
					Prefix: prefixBuilder(depth),
					Style:  MessageStyleMuted,
					Text:   log,
				})
			}
		}
	}

	return instLogs
}
