package logpoller

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/gagliardetto/solana-go"
)

var (
	invokeMatcher   = regexp.MustCompile(`Program (\w*) invoke \[(\d)\]`)
	consumedMatcher = regexp.MustCompile(`Program \w* consumed (\d*) (.*)`)
	logMatcher      = regexp.MustCompile(`Program log: (.*)`)
	dataMatcher     = regexp.MustCompile(`Program data: (.*)`)
)

type BlockData struct {
	SlotNumber          uint64
	BlockHeight         uint64
	BlockHash           solana.Hash
	TransactionHash     solana.Signature
	TransactionIndex    int
	TransactionLogIndex uint
}

type ProgramLog struct {
	BlockData
	Text   string
	Prefix string
}

type ProgramEvent struct {
	BlockData
	Prefix string
	Data   string
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

func parseProgramLogs(logs []string) []ProgramOutput {
	var depth int

	instLogs := []ProgramOutput{}
	lastLogIdx := -1

	for _, log := range logs {
		if strings.HasPrefix(log, "Program log:") {
			logDataMatches := logMatcher.FindStringSubmatch(log)

			if len(logDataMatches) <= 1 || lastLogIdx < 0 {
				continue
			}

			// this is a general log
			instLogs[lastLogIdx].Logs = append(instLogs[lastLogIdx].Logs, ProgramLog{
				Prefix: prefixBuilder(depth),
				Text:   logDataMatches[1],
			})
		} else if strings.HasPrefix(log, "Program data:") {
			if lastLogIdx < 0 {
				continue
			}

			dataMatches := dataMatcher.FindStringSubmatch(log)

			if len(dataMatches) > 1 {
				instLogs[lastLogIdx].Events = append(instLogs[lastLogIdx].Events, ProgramEvent{
					Prefix: prefixBuilder(depth),
					Data:   dataMatches[1],
				})
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
						Program: matches[1],
					})

					lastLogIdx = len(instLogs) - 1
				}

				depth++
			} else if strings.Contains(log, "success") {
				depth--
			} else if strings.Contains(log, "failed") {
				if lastLogIdx < 0 {
					continue
				}

				instLogs[lastLogIdx].Failed = true

				idx := strings.Index(log, ": ") + 2

				// failed to verify log of previous program so reset depth and print full log
				if strings.HasPrefix(log, "failed") {
					depth++
				}

				instLogs[lastLogIdx].ErrorText = log[idx:]

				depth--
			} else {
				if depth == 0 {
					instLogs = append(instLogs, ProgramOutput{})
					lastLogIdx = len(instLogs) - 1
				}

				if lastLogIdx < 0 {
					continue
				}

				matches := consumedMatcher.FindStringSubmatch(log)
				if len(matches) == 3 && depth == 1 {
					if val, err := strconv.Atoi(matches[1]); err == nil {
						instLogs[lastLogIdx].ComputeUnits = uint(val) //nolint:gosec
					}
				}
			}
		}
	}

	return instLogs
}
