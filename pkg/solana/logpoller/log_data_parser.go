package logpoller

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
)

/*
Loosely based on Anchor's event parsing
(https://github.com/solana-foundation/anchor/blob/48aba30646fb7ac4400a33a7ae679fc790d796ea/ts/packages/anchor/src/program/event.ts#L190),
with the main difference being that the parsing in our code is not subscribed to a particular program ID nor just
looking for events.
Also inspired in part by a community rust event parser for solana
(https://github.com/cyphersnake/solana-events-parser/tree/f6e6b274f1607eb46ccb6390938204141ffb97c9)

Prefixes and regexes are based on Agave:
- Most expressions: https://github.com/anza-xyz/agave/blob/b6d599d82eb9faf3bd14574a70f5137cdd9953f8/program-runtime/src/stable_log.rs#L13-L110
- Units consumed: https://github.com/anza-xyz/agave/blob/9763a422bff65294a7a391f34e8a9b473b01b109/programs/bpf_loader/src/lib.rs#L1556
- Truncated log: https://github.com/anza-xyz/agave/blob/9763a422bff65294a7a391f34e8a9b473b01b109/svm-log-collector/src/lib.rs#L44

Firedancer maintains compatibility with agave for these log formats too: https://github.com/firedancer-io/firedancer/blob/3be85093b5f03e3d9fa22554c3616c3c19665d4e/src/flamenco/log_collector/fd_log_collector.h#L107-L110

Jito too:
- Most expressions: https://github.com/jito-foundation/jito-solana/blob/f976835e322dedc52f143349fa97828bc05d18a3/program-runtime/src/stable_log.rs
- Units consumed: https://github.com/jito-foundation/jito-solana/blob/f976835e322dedc52f143349fa97828bc05d18a3/programs/bpf_loader/src/lib.rs#L1548
- Truncated log: https://github.com/jito-foundation/jito-solana/blob/f976835e322dedc52f143349fa97828bc05d18a3/svm-log-collector/src/lib.rs#L44
*/

const programLog = "Program log: "
const programData = "Program data: "
const programReturn = "Program return: "
const logTruncated = "Log truncated"

// Pattern based on https://github.com/solana-foundation/anchor/blob/c1c9261ca4e2f9ce3232e4fd41e4f6ad8742fe3e/ts/packages/anchor/src/program/event.ts#L282
// and taking into account that addresses are 32 bytes, so the base58 encoding is between 32 & 44 characters long.
// Same pattern found on https://coin.space/solana-address-example/
const addressPattern = `[1-9A-HJ-NP-Za-km-z]{32,44}`

var (
	// CPI depth today is capped at 4, but using \d+ in invokeMatcher for future-proofing
	invokeMatcher   = regexp.MustCompile(`^Program (` + addressPattern + `) invoke \[(\d+)\]$`)
	consumedMatcher = regexp.MustCompile(`^Program (` + addressPattern + `) consumed (\d+) of \d+ compute units$`)
	successMatcher  = regexp.MustCompile(`^Program (` + addressPattern + `) success$`)
	failureMatcher  = regexp.MustCompile(`^Program (` + addressPattern + `) failed: (.+)$`)
)

func ParseProgramLogs(logs []string) ([]types.ProgramOutput, error) {
	programs := newStack[string]()
	instLogs := newAppendOnly[types.ProgramOutput]()

	for _, log := range logs {
		line := toLogLine(log)
		err := line.Process(&instLogs, &programs)
		if err != nil {
			return nil, fmt.Errorf("error processing log line of type %s: %w. Full line was %s", line.Type(), err, log)
		}
		if line.Type() == Truncated {
			break // return early if truncated logs are encountered, there are no more logs to process
		}
	}

	// Completeness heuristic: if stack isn't fully unwound, treat as truncated/incomplete
	if instLogs.Len() > 0 && programs.Depth() != 0 {
		instLogs.PeekUnchecked().Truncated = true
	}

	return instLogs.items, nil
}

type logType string

const (
	Unknown   logType = "Unknown"   // catch-all for unrecognized log lines, such as BPF loader logs
	General   logType = "General"   // Program log: <text>
	Event     logType = "Event"     // Program data: <base64>
	Return    logType = "Return"    // Program return: <program_id> <base64>
	Invoke    logType = "Invoke"    // Program <program_id> invoke [<depth>]
	Consume   logType = "Consume"   // Program <program_id> consumed <units> of <total> compute units
	Success   logType = "Success"   // Program <program_id> success
	Failed    logType = "Failed"    // Program <program_id> failed: <reason>
	Truncated logType = "Truncated" // Log truncated
)

func toLogLine(log string) lineProcessor {
	// Prefix matches first (more performant)
	if strings.HasPrefix(log, programLog) {
		return &logLine{LogText: log[len(programLog):]}
	}
	if strings.HasPrefix(log, programData) {
		return &eventLine{EventData: log[len(programData):]}
	}
	if strings.HasPrefix(log, programReturn) {
		logData := log[len(programReturn):]
		matches := strings.SplitN(logData, " ", 2) // any line that has that prefix must have both program ID and data
		// If SplitN returns fewer than 2 matches, the return line is malformed and should be marked as unknown to avoid processing
		if len(matches) != 2 {
			return &UnknownLine{}
		}
		return &returnLine{ProgramID: matches[0], Data: matches[1]}
	}
	if strings.HasPrefix(log, logTruncated) {
		return &truncatedLine{}
	}

	// Early best-effort return before less-performant regex matching
	// Must be done after the log truncated check to not miss those logs
	if !strings.HasPrefix(log, "Program ") {
		// Unknown line type, to be dismissed. Can happen for example on BPF loader logs, e.g.
		// https://explorer.solana.com/tx/3psYALQ9s7SjdezXw2kxKkVuQLtSAQxPAjETvy765EVxJE7cYqfc4oGbpNYEWsAiuXuTnqcsSUHLQ3iZUenTHTsA?cluster=devnet
		return &UnknownLine{}
	}

	// Regex matches next (less performant)
	if matches := invokeMatcher.FindStringSubmatch(log); len(matches) > 2 {
		return &invokeLine{ProgramID: matches[1], Depth: matches[2]}
	}
	if matches := consumedMatcher.FindStringSubmatch(log); len(matches) > 2 {
		return &consumeLine{ProgramID: matches[1], Units: matches[2]}
	}
	if matches := successMatcher.FindStringSubmatch(log); len(matches) > 1 {
		return &successLine{ProgramID: matches[1]}
	}
	if matches := failureMatcher.FindStringSubmatch(log); len(matches) > 2 {
		return &failedLine{ProgramID: matches[1], Reason: matches[2]}
	}

	// Unknown line type, to be dismissed. Can happen for example on BPF loader logs (see comment above).
	// The BPF loader may log many different types of lines, some with the "Program " prefix and some without.
	// See https://github.com/anza-xyz/agave/blob/64512ef/programs/bpf_loader/src/lib.rs
	// and look for `ic_logger_msg!` invocations.
	return &UnknownLine{}
}

// =================================================
// LINE PROCESSORS for each type of matched log line
// =================================================
type lineProcessor interface {
	Process(instLogs *appendOnly[types.ProgramOutput], programs *stack[string]) error
	Type() logType
}

// ---
// Program log: <text>
type logLine struct {
	LogText string
}

func (l *logLine) Process(instLogs *appendOnly[types.ProgramOutput], programs *stack[string]) error {
	if instLogs == nil || instLogs.Len() == 0 {
		return errNilOrEmptyArg
	}
	output := instLogs.PeekUnchecked()
	output.Logs = append(output.Logs, types.ProgramLog{
		Prefix: strings.Repeat(">", programs.Depth()),
		Text:   l.LogText,
	})
	return nil
}

func (l *logLine) Type() logType {
	return General
}

// ---
// Program data: <base64>
type eventLine struct {
	EventData string
}

func (l *eventLine) Process(instLogs *appendOnly[types.ProgramOutput], programs *stack[string]) error {
	if instLogs == nil || instLogs.Len() == 0 || programs == nil || programs.Depth() == 0 {
		return errNilOrEmptyArg
	}
	output := instLogs.PeekUnchecked()
	txLogIdx := uint(len(output.Events))
	output.Events = append(output.Events, types.ProgramEvent{
		Program:   programs.PeekUnchecked(),
		Data:      l.EventData,
		BlockData: types.BlockData{TransactionLogIndex: txLogIdx},
	})
	return nil
}

func (l *eventLine) Type() logType {
	return Event
}

// ---
// Program <program_id> invoke [<depth>]
type invokeLine struct {
	ProgramID string
	Depth     string
}

func (l *invokeLine) Process(instLogs *appendOnly[types.ProgramOutput], programs *stack[string]) error {
	if instLogs == nil || programs == nil {
		return errNilOrEmptyArg
	}
	if programs.Depth() == 0 {
		newOutput := types.ProgramOutput{Program: l.ProgramID}
		instLogs.Append(newOutput)
	}
	programs.Push(l.ProgramID)
	invokeDepth, err := strconv.Atoi(l.Depth)
	if err != nil {
		return err
	}
	// condition _after_ pushing the new program ID, as solana "invoke [\d]" logs counts depth starting at 1
	if programs.Depth() != invokeDepth {
		return &invokeDepthError{parsedDepth: invokeDepth, expectedDepth: programs.Depth()}
	}
	return nil
}

func (l *invokeLine) Type() logType {
	return Invoke
}

// ---
// Program return: <program_id> <base64>
type returnLine struct {
	ProgramID string
	Data      string
}

func (l *returnLine) Process(instLogs *appendOnly[types.ProgramOutput], programs *stack[string]) error {
	err := checkProgramIDMatch(programs, l.ProgramID)
	if err != nil {
		return err
	}
	// currently no processing on return logs, as there has to be a success or failure log afterwards where the
	// program stack is actually popped
	return nil
}

func (l *returnLine) Type() logType {
	return Return
}

// ---
// Program <program_id> consumed <units> of <total> compute units
type consumeLine struct {
	ProgramID string
	Units     string
}

func (l *consumeLine) Process(instLogs *appendOnly[types.ProgramOutput], programs *stack[string]) error {
	if instLogs == nil || instLogs.Len() == 0 || programs == nil {
		return errNilOrEmptyArg
	}
	if programs.Depth() != 1 {
		return nil // we only track compute units for top-level programs, not nested CPIs
	}
	err := checkProgramIDMatch(programs, l.ProgramID)
	if err != nil {
		return err
	}
	val, err := strconv.Atoi(l.Units)
	if err != nil {
		return err
	}
	output := instLogs.PeekUnchecked() // the initial check already covers the empty case
	output.ComputeUnits = uint(val)    //nolint:gosec // compute units are always positive so it is safe to cast to uint
	return nil
}

func (l *consumeLine) Type() logType {
	return Consume
}

// ---
// Program <program_id> success
type successLine struct {
	ProgramID string
}

func (l *successLine) Process(instLogs *appendOnly[types.ProgramOutput], programs *stack[string]) error {
	err := checkProgramIDMatch(programs, l.ProgramID)
	if err != nil {
		return err
	}
	programs.Pop() // the previous check for programIDs ensures the stack is not empty
	return nil
}

func (l *successLine) Type() logType {
	return Success
}

// ---
// Program <program_id> failed: <reason>
type failedLine struct {
	ProgramID string
	Reason    string
}

func (l *failedLine) Process(instLogs *appendOnly[types.ProgramOutput], programs *stack[string]) error {
	if instLogs == nil || instLogs.Len() == 0 || programs.Depth() == 0 {
		return errNilOrEmptyArg
	}
	err := checkProgramIDMatch(programs, l.ProgramID)
	if err != nil {
		return err
	}

	output := instLogs.PeekUnchecked() // the initial check already covers the empty case
	output.Failed = true
	output.ErrorText = l.Reason
	programs.Pop() // the previous check for programIDs ensures the stack is not empty
	return nil
}

func (l *failedLine) Type() logType {
	return Failed
}

// ---
// Log truncated
type truncatedLine struct{}

func (l *truncatedLine) Process(instLogs *appendOnly[types.ProgramOutput], programs *stack[string]) error {
	if instLogs == nil || instLogs.Len() == 0 {
		return errNilOrEmptyArg
	}
	instLogs.PeekUnchecked().Truncated = true
	return nil
}

func (l *truncatedLine) Type() logType {
	return Truncated
}

// ---
// catch-all for unrecognized log lines, such as BPF loader logs
type UnknownLine struct{}

func (l *UnknownLine) Process(instLogs *appendOnly[types.ProgramOutput], programs *stack[string]) error {
	// nothing to do, we just ignore these lines
	return nil
}

func (l *UnknownLine) Type() logType {
	return Unknown
}

// ===================================================
// ERROR TYPES for line processors and reusable checks
// ===================================================

var errNilOrEmptyArg = errors.New("unexpected nil or empty argument")

type invokeDepthError struct {
	parsedDepth, expectedDepth int
}

func (e *invokeDepthError) Error() string {
	return fmt.Sprintf("invoke depth %d does not match program stack depth %d", e.parsedDepth, e.expectedDepth)
}

type programIDMismatchError struct {
	expected, actual string
}

func (e *programIDMismatchError) Error() string {
	return fmt.Sprintf("program ID %s does not match expected program ID %s from stack", e.actual, e.expected)
}

// helper function to check for consistency between program ID from log line and program ID at the top of stack
func checkProgramIDMatch(programs *stack[string], actual string) error {
	expected, ok := programs.Peek()
	if !ok {
		return &programIDMismatchError{expected: "<empty stack>", actual: actual}
	}
	if expected != actual {
		return &programIDMismatchError{expected: expected, actual: actual}
	}
	return nil
}
