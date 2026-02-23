package logpoller

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
)

func TestLogDataParse_Error(t *testing.T) {
	t.Parallel()

	// logs include 2 program invocations
	logs := []string{
		"Program ComputeBudget111111111111111111111111111111 invoke [1]",
		"Program ComputeBudget111111111111111111111111111111 success",
		"Program cjg3oHmg9uuPsP8D6g29NWvhySJkdYdAo9D25PRbKXJ invoke [1]",
		"Program log: AnchorError thrown in programs/ocr2/src/lib.rs:639. Error Code: StaleReport. Error Number: 6003. Error Message: Stale report.",
		"Program cjg3oHmg9uuPsP8D6g29NWvhySJkdYdAo9D25PRbKXJ consumed 6504 of 199850 compute units",
		"Program cjg3oHmg9uuPsP8D6g29NWvhySJkdYdAo9D25PRbKXJ failed: custom program error: 0x1773",
	}

	output, err := ParseProgramLogs(logs)
	require.NoError(t, err)

	require.Len(t, output, 2)

	// first program has no logs, no events, no compute units and succeeded
	assert.Equal(t, types.ProgramOutput{
		Program: "ComputeBudget111111111111111111111111111111",
	}, output[0])

	// second program should have one log, no events, 6504 compute units and failed with error message
	expected := types.ProgramOutput{
		Program: "cjg3oHmg9uuPsP8D6g29NWvhySJkdYdAo9D25PRbKXJ",
		Logs: []types.ProgramLog{
			{
				Prefix: ">",
				Text:   "AnchorError thrown in programs/ocr2/src/lib.rs:639. Error Code: StaleReport. Error Number: 6003. Error Message: Stale report.",
			},
		},
		ComputeUnits: 6504,
		Failed:       true,
		ErrorText:    "custom program error: 0x1773",
	}

	assert.Equal(t, expected, output[1])
}

func TestLogDataParse_IncompleteWithoutLogTruncatedMarker(t *testing.T) {
	t.Parallel()

	// No "Log truncated" line, but the invoke stack is not fully unwound:
	// - We see a top-level invoke [1]
	// - We never see "success" or "failed" for that program
	logs := []string{
		"Program ComputeBudget111111111111111111111111111111 invoke [1]",
		"Program ComputeBudget111111111111111111111111111111 success",

		"Program J1zQwrBNBngz26jRPNWsUSZMHJwBwpkoDitXRV95LdK4 invoke [1]",
		"Program log: Instruction: CreateLog",
		"Program data: HDQnaQjSWwkNAAAASGVsbG8sIFdvcmxkISoAAAAAAAAA",
		"Program J1zQwrBNBngz26jRPNWsUSZMHJwBwpkoDitXRV95LdK4 consumed 1477 of 200000 compute units",
		// missing: "Program ... success" (or failed)
	}

	output, err := ParseProgramLogs(logs)
	require.NoError(t, err)

	// We should have 2 ProgramOutputs (ComputeBudget + J1zQ...)
	require.Len(t, output, 2)

	// First one is complete
	require.False(t, output[0].Truncated)
	require.Equal(t, "ComputeBudget111111111111111111111111111111", output[0].Program)

	// Second one should be treated as truncated/incomplete due to non-zero stack depth at end
	require.True(t, output[1].Truncated)
	require.Equal(t, "J1zQwrBNBngz26jRPNWsUSZMHJwBwpkoDitXRV95LdK4", output[1].Program)
}

func TestLogDataParse_SuccessBasic(t *testing.T) {
	t.Parallel()

	// logs include 2 program invocations
	logs := []string{
		"Program ComputeBudget111111111111111111111111111111 invoke [1]",
		"Program ComputeBudget111111111111111111111111111111 success",
		"Program SAGE2HAwep459SNq61LHvjxPk4pLPEJLoMETef7f7EE invoke [1]",
		"Program log: Instruction: IdleToLoadingBay",
		"Program log: Current state: Idle(Idle { sector: [13, 37] })",
		"Program SAGE2HAwep459SNq61LHvjxPk4pLPEJLoMETef7f7EE consumed 16850 of 199850 compute units",
		"Program SAGE2HAwep459SNq61LHvjxPk4pLPEJLoMETef7f7EE success",
	}

	output, err := ParseProgramLogs(logs)
	require.NoError(t, err)

	require.Len(t, output, 2)

	// first program has no logs, no events, no compute units and succeeded
	assert.Equal(t, types.ProgramOutput{
		Program: "ComputeBudget111111111111111111111111111111",
	}, output[0])

	// second program should have one log, no events, 6504 compute units and failed with error message
	expected := types.ProgramOutput{
		Program: "SAGE2HAwep459SNq61LHvjxPk4pLPEJLoMETef7f7EE",
		Logs: []types.ProgramLog{
			{Prefix: ">", Text: "Instruction: IdleToLoadingBay"},
			{Prefix: ">", Text: "Current state: Idle(Idle { sector: [13, 37] })"},
		},
		ComputeUnits: 16850,
	}

	assert.Equal(t, expected, output[1])
}

func TestLogDataParse_SuccessComplex(t *testing.T) {
	t.Parallel()

	// example program log output from solana explorer
	// tx_sig: 54tfPQgreeturXgQovpB6dBmprhEqaK6JoVCEsVRSBCG9wJrqAnezUWPwEN11PpEE2mAW5dD9xHpSdZD7krafHia
	// slot: 302_573_728
	logs := []string{
		// [0]
		"Program ComputeBudget111111111111111111111111111111 invoke [1]",
		"Program ComputeBudget111111111111111111111111111111 success",
		// [1]
		"Program ComputeBudget111111111111111111111111111111 invoke [1]",
		"Program ComputeBudget111111111111111111111111111111 success",
		// [2] System program
		"Program 11111111111111111111111111111111 invoke [1]",
		"Program 11111111111111111111111111111111 success",
		// [3] Token program
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [1]",
		"Program log: Instruction: InitializeAccount",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 3443 of 99550 compute units",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
		// [4] Associated token program
		"Program ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL invoke [1]",
		"Program log: Create",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [2]",
		"Program log: Instruction: GetAccountDataSize",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 1569 of 89240 compute units",
		"Program return: TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA pQAAAAAAAAA=",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
		"Program 11111111111111111111111111111111 invoke [2]",
		"Program 11111111111111111111111111111111 success",
		"Program log: Initialize the associated token account",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [2]",
		"Program log: Instruction: InitializeImmutableOwner",
		"Program log: Please upgrade to SPL Token 2022 for immutable owner support",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 1405 of 82653 compute units",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [2]",
		"Program log: Instruction: InitializeAccount3",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 4188 of 78771 compute units",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
		"Program ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL consumed 21807 of 96107 compute units",
		"Program ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL success",
		// [5]
		"Program 675kPX9MHTjS2zt1qfr1NYHuzeLXfQM9H24wFSUt1Mp8 invoke [1]",
		"Program log: ray_log: AwDC6wsAAAAAHxsZjgkAAAACAAAAAAAAAADC6wsAAAAAMW3pEz4AAAD7j2wjcDsAAAXbgGALAAAA",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [2]",
		"Program log: Instruction: Transfer",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 4736 of 56164 compute units",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [2]",
		"Program log: Instruction: Transfer",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 4645 of 48447 compute units",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
		"Program 675kPX9MHTjS2zt1qfr1NYHuzeLXfQM9H24wFSUt1Mp8 consumed 31576 of 74300 compute units",
		"Program 675kPX9MHTjS2zt1qfr1NYHuzeLXfQM9H24wFSUt1Mp8 success",
		// [6]
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [1]",
		"Program log: Instruction: CloseAccount",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 2915 of 42724 compute units",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
		// [7] System program
		"Program 11111111111111111111111111111111 invoke [1]",
		"Program 11111111111111111111111111111111 success",
		// [8]
		"Program 4pP8eDKACuV7T2rbFPE8CHxGKDYAzSdRsdMsGvz2k4oc invoke [1]",
		"Program log: Received timestamp: 1732124122",
		"Program log: Current timestamp: 1732124102",
		"Program log: The provided timestamp is valid.",
		"Program 4pP8eDKACuV7T2rbFPE8CHxGKDYAzSdRsdMsGvz2k4oc consumed 1661 of 39659 compute units",
		"Program 4pP8eDKACuV7T2rbFPE8CHxGKDYAzSdRsdMsGvz2k4oc success",
		// [9] System program
		"Program 11111111111111111111111111111111 invoke [1]",
		"Program 11111111111111111111111111111111 success",
		// [10]
		"Program HQ2UUt18uJqKaQFJhgV9zaTdQxUZjNrsKFgoEDquBkcx invoke [1]",
		"Program log: Powered by bloXroute Trader Api",
		"Program HQ2UUt18uJqKaQFJhgV9zaTdQxUZjNrsKFgoEDquBkcx consumed 803 of 37848 compute units",
		"Program HQ2UUt18uJqKaQFJhgV9zaTdQxUZjNrsKFgoEDquBkcx success",
	}

	output, err := ParseProgramLogs(logs)
	require.NoError(t, err)

	require.Len(t, output, 11)

	// first two programs have no logs, no events, no compute units and succeeded
	for idx := range 1 {
		assert.Equal(t, types.ProgramOutput{
			Program: "ComputeBudget111111111111111111111111111111",
		}, output[idx])
	}

	expectedSystemProgramIdxs := []int{2, 7, 9}
	for _, idx := range expectedSystemProgramIdxs {
		assert.Equal(t, types.ProgramOutput{
			Program: "11111111111111111111111111111111",
		}, output[idx])
	}

	require.Len(t, output[4].Logs, 6)
}

func TestLogDataParse_InvokeTooShallow(t *testing.T) {
	t.Parallel()

	logs := []string{
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [1]",
		// impossible case, just for testing: invoking with depth 1 (unnested) without original ix succeeding first
		"Program ComputeBudget111111111111111111111111111111 invoke [1]",
		"Program ComputeBudget111111111111111111111111111111 success",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
	}

	output, err := ParseProgramLogs(logs)

	var expectedErr *invokeDepthError
	require.ErrorAs(t, err, &expectedErr)
	require.Nil(t, output)
}

func TestLogDataParse_InvokeTooDeep(t *testing.T) {
	t.Parallel()

	logs := []string{
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [1]",
		// impossible case, just for testing: invoking with depth 3 without any invoke with depth 2
		"Program ComputeBudget111111111111111111111111111111 invoke [1]",
		"Program ComputeBudget111111111111111111111111111111 success",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
	}

	output, err := ParseProgramLogs(logs)

	var expectedErr *invokeDepthError
	require.ErrorAs(t, err, &expectedErr)
	require.Nil(t, output)
}

func TestLogDataParse_SuccessProgramIDs(t *testing.T) {
	t.Parallel()

	type testcase struct {
		name string
		logs []string
	}

	testcases := []testcase{
		{
			name: "MismatchedSuccess",
			logs: []string{
				"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [1]",
				"Program ComputeBudget111111111111111111111111111111 invoke [2]",
				// impossible case, just for testing: success program does not match invoked one
				"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
			},
		},
		{
			name: "UnmatchedSuccess",
			logs: []string{
				// impossible case, a program succeeds without being invoked
				"Program ComputeBudget111111111111111111111111111111 success",
			},
		},
		{
			name: "OutOfOrderSuccess",
			logs: []string{
				"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [1]",
				// impossible case, a program succeeds without being invoked
				"Program ComputeBudget111111111111111111111111111111 success",
				"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			output, err := ParseProgramLogs(tc.logs)

			var expectedErr *programIDMismatchError
			require.ErrorAs(t, err, &expectedErr)
			require.Nil(t, output)
		})
	}
}

func TestLogDataParse_SuccessReturn(t *testing.T) {
	t.Parallel()

	aLog := "F01Jt3u5czkXAAAAAAAAAOcDAAAAAAAAktzUYhnCGzTkA3rgGjl/oA32R1w3keBaTpfMYn9NvVYPAAAAAAAAABcAAAAAAAAA5wMAAAAAAAB4AwAAAAAAAF9NPMHmx7GsuvJ4huHC9+ne5I/7uwHAU/6nHVD6quLEEAAAAEFBQUFCQkJCQ0NDQ0REREQgAAAAAAAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAAAAAAVAAAAGB3PEEANAwAAAAAAAAAAAAAAAAAABpuIV/6rgYT7aH9jRhjANdrEOdwa6ztVmKDwAAAAAAEBAAAAfVthzhsr3oAUS2cJwgUfm23XD5bNQ/bRY2CMJccwNO8gAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABAAgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAl/f39/f39/f39/f39/f39/f39/f39/f39/f39/f39/fwQAAAAAAABk5GUqAgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAXBILeOgMAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="

	anotherLog := "F01Jt3u5czkVAAAAAAAAAAEAAAAAAAAAYLMTvjqK9A1r+qhK8K5a/oLtYiVDjzEofKppWShLUREPAAAAAAAAABUAAAAAAAAAAQAAAAAAAAABAAAAAAAAAF9NPMHmx7GsuvJ4huHC9+ne5I/7uwHAU/6nHVD6quLEAwAAAAQFBiAAAAAAAAAAAAAAAAAAAAABAAAAAAAAAAAAAAAAAAAAAAAAABUAAAAYHc8QQA0DAAAAAAAAAAAAAAAAAAAGm4hX/quBhPtof2NGGMA12sQ53BrrO1WYoPAAAAAAAQEAAACtYGyUcVUwXssP2A8KanRuQp8PMcAbZ4A0v/xPDoFGeSAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEACAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAACQEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABAAAAAAAAGTkZSoCAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABcEgt46AwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

	logsString := fmt.Sprintf(`Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [1]
Program log: Instruction: ApproveChecked
Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 4457 of 600000 compute units
Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success
Program Ccip842gzYHhvdDkSyi2YVCoAWPbYJoApMFzSxQroE9C invoke [1]
Program log: Instruction: CcipSend
Program RmnXLft1mSEwDgMKu2okYuHkiazxntFFcZFrrcXxYg7 invoke [2]
Program log: Instruction: VerifyNotCursed
Program RmnXLft1mSEwDgMKu2okYuHkiazxntFFcZFrrcXxYg7 consumed 6997 of 524999 compute units
Program RmnXLft1mSEwDgMKu2okYuHkiazxntFFcZFrrcXxYg7 success
Program FeeQPGkKDeRV1MgoYfMH6L8o3KeuYjwUZrgn4LRKfjHi invoke [2]
Program log: Instruction: GetFee
Program FeeQPGkKDeRV1MgoYfMH6L8o3KeuYjwUZrgn4LRKfjHi consumed 46019 of 481343 compute units
Program return: FeeQPGkKDeRV1MgoYfMH6L8o3KeuYjwUZrgn4LRKfjHi BpuIV/6rgYT7aH9jRhjANdrEOdwa6ztVmKDwAAAAAAHkZSoCAAAAAABcEgt46AwAAAAAAAAAAAABAAAAZAAAAGQAAAAVAAAAGB3PEEANAwAAAAAAAAAAAAAAAAAAQA0DAAAAAAAAAAAAAAAAAAAA
Program FeeQPGkKDeRV1MgoYfMH6L8o3KeuYjwUZrgn4LRKfjHi success
Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [2]
Program log: Instruction: TransferChecked
Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 6346 of 431550 compute units
Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success
Program 11111111111111111111111111111111 invoke [2]
Program 11111111111111111111111111111111 success
Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [2]
Program log: Instruction: TransferChecked
Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 6291 of 418794 compute units
Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success
Program G5TnFDEUNtXnPpj73PkphKJ7Uc2QR9WVUEZG5KHdEN8V invoke [2]
Program log: Instruction: LockOrBurnTokens
Program RmnXLft1mSEwDgMKu2okYuHkiazxntFFcZFrrcXxYg7 invoke [3]
Program log: Instruction: VerifyNotCursed
Program RmnXLft1mSEwDgMKu2okYuHkiazxntFFcZFrrcXxYg7 consumed 6997 of 366682 compute units
Program RmnXLft1mSEwDgMKu2okYuHkiazxntFFcZFrrcXxYg7 success
Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [3]
Program log: Instruction: Burn
Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 4753 of 357207 compute units
Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success
Program data: zyX7mu/lDkMyZB6FXWyVFQIAeT7zUhwyy3BjUp8rmSZ0UUd+I8Z6cwEAAAAAAAAA7aXVf7lOOB4ObyqQDzIKJGnrRzl1fsbEObXd22CeZ40=
Program G5TnFDEUNtXnPpj73PkphKJ7Uc2QR9WVUEZG5KHdEN8V invoke [3]
Program log: Instruction: ReturnData
Program G5TnFDEUNtXnPpj73PkphKJ7Uc2QR9WVUEZG5KHdEN8V consumed 1397 of 348485 compute units
Program return: G5TnFDEUNtXnPpj73PkphKJ7Uc2QR9WVUEZG5KHdEN8V successAAA==
Program G5TnFDEUNtXnPpj73PkphKJ7Uc2QR9WVUEZG5KHdEN8V success
Program data: %s
Program G5TnFDEUNtXnPpj73PkphKJ7Uc2QR9WVUEZG5KHdEN8V consumed 64646 of 400837 compute units
Program return: G5TnFDEUNtXnPpj73PkphKJ7Uc2QR9WVUEZG5KHdEN8V IAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAQAIAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAJ
Program G5TnFDEUNtXnPpj73PkphKJ7Uc2QR9WVUEZG5KHdEN8V success
Program data: %s
Program Ccip842gzYHhvdDkSyi2YVCoAWPbYJoApMFzSxQroE9C consumed 265023 of 595543 compute units
Program return: Ccip842gzYHhvdDkSyi2YVCoAWPbYJoApMFzSxQroE9C YLMTvjqK9A1r+qhK8K5a/oLtYiVDjzEofKppWShLURE=
Program Ccip842gzYHhvdDkSyi2YVCoAWPbYJoApMFzSxQroE9C success
Program ComputeBudget111111111111111111111111111111 invoke [1]
Program ComputeBudget111111111111111111111111111111 success`, aLog, anotherLog)

	logs := strings.Split(logsString, "\n")
	outputs, err := ParseProgramLogs(logs)
	require.NoError(t, err)

	expected := []types.ProgramOutput{
		{
			Program: "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA",
			Logs: []types.ProgramLog{
				{Prefix: ">", Text: "Instruction: ApproveChecked"},
			},
			ComputeUnits: 4457,
		},
		{
			Program: "Ccip842gzYHhvdDkSyi2YVCoAWPbYJoApMFzSxQroE9C",
			Logs: []types.ProgramLog{
				{Prefix: ">", Text: "Instruction: CcipSend"},
				{Prefix: ">>", Text: "Instruction: VerifyNotCursed"},
				{Prefix: ">>", Text: "Instruction: GetFee"},
				{Prefix: ">>", Text: "Instruction: TransferChecked"},
				{Prefix: ">>", Text: "Instruction: TransferChecked"},
				{Prefix: ">>", Text: "Instruction: LockOrBurnTokens"},
				{Prefix: ">>>", Text: "Instruction: VerifyNotCursed"},
				{Prefix: ">>>", Text: "Instruction: Burn"},
				{Prefix: ">>>", Text: "Instruction: ReturnData"},
			},
			Events: []types.ProgramEvent{
				{
					Program: "G5TnFDEUNtXnPpj73PkphKJ7Uc2QR9WVUEZG5KHdEN8V",
					Data:    "zyX7mu/lDkMyZB6FXWyVFQIAeT7zUhwyy3BjUp8rmSZ0UUd+I8Z6cwEAAAAAAAAA7aXVf7lOOB4ObyqQDzIKJGnrRzl1fsbEObXd22CeZ40=",
					BlockData: types.BlockData{
						TransactionLogIndex: 0,
					},
				},
				{
					Program: "G5TnFDEUNtXnPpj73PkphKJ7Uc2QR9WVUEZG5KHdEN8V",
					Data:    aLog,
					BlockData: types.BlockData{
						TransactionLogIndex: 1,
					},
				},
				{
					Program: "Ccip842gzYHhvdDkSyi2YVCoAWPbYJoApMFzSxQroE9C",
					Data:    anotherLog,
					BlockData: types.BlockData{
						TransactionLogIndex: 2,
					},
				},
			},
			ComputeUnits: 265023,
		},
		{
			Program: "ComputeBudget111111111111111111111111111111",
		},
	}

	requireExpected(t, expected, outputs)
}

func TestLogDataParse_Events(t *testing.T) {
	t.Parallel()

	// example program event output from test contract
	logs := []string{
		"Program J1zQwrBNBngz26jRPNWsUSZMHJwBwpkoDitXRV95LdK4 invoke [1]",
		"Program log: Instruction: CreateLog",
		"Program data: HDQnaQjSWwkNAAAASGVsbG8sIFdvcmxkISoAAAAAAAAA", // base64 encoded; borsh encoded with identifier
		"Program J1zQwrBNBngz26jRPNWsUSZMHJwBwpkoDitXRV95LdK4 consumed 1477 of 200000 compute units",
		"Program J1zQwrBNBngz26jRPNWsUSZMHJwBwpkoDitXRV95LdK4 success",
	}

	output, err := ParseProgramLogs(logs)
	require.NoError(t, err)

	require.Len(t, output, 1)
	assert.Len(t, output[0].Events, 1)
}

func TestLogDataParse_NestedCCIPSend(t *testing.T) {
	t.Parallel()

	// example program log output from solana explorer
	logs := []string{
		"Program 6LLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLL invoke [1]",
		"Program log: Instruction: StartPingPong",
		"Program Ccip8888888888888888888888888888888888888888 invoke [2]",
		"Program log: Instruction: CcipSend",
		"Program 11111111111111111111111111111111 invoke [3]",
		"Program 11111111111111111111111111111111 success",
		"Program RmnAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA invoke [3]",
		"Program log: Instruction: VerifyNotCursed",
		"Program RmnAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA consumed 5353 of 117093 compute units",
		"Program RmnAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA success",
		"Program FeeQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQ invoke [3]",
		"Program log: Instruction: GetFee",
		"Program FeeQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQ consumed 26059 of 106400 compute units",
		"Program return: FeeQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQ suG6JlKUMbSLTXQmSXm+3eln5seBIbgd1wizVTDAbEcAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAFQAAABgdzxBADQMAAAAAAAAAAAAAAAAAAEANAwAAAAAAAAAAAAAAAAAAAA==",
		"Program FeeQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQ success",
		"Program TokenzQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQ invoke [3]",
		"Program log: Instruction: TransferChecked",
		"Program TokenzQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQ consumed 1900 of 75968 compute units",
		"Program TokenzQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQ success",
		"Program data: F01Jt3u5cznZGtnJT7pB3lEGAAAAAAAAS9pilw25WRal2CYAvmIXJuQCq4gQGLxq+xIbdF3AUPXfN+OU4sfs49ka2clPukHeUQYAAAAAAAABAAAAAAAAAO+w8bNlBeDYJ6mAasw3PzgJHYDRC6PYjnR63SdS9S7sIAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABIAAAAAAAAAAAAAAAAAAAANZ0+dAL9sca9f0xVxj5Lj6B9ubNFQAAABgdzxBADQMAAAAAAAAAAAAAAAAAALLhuiZSlDG0i010Jkl5vt3pZ+bHgSG4HdcIs1UwwGxHAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		"Program Ccip8888888888888888888888888888888888888888 consumed 95683 of 165338 compute units",
		"Program return: Ccip8888888888888888888888888888888888888888 S9pilw25WRal2CYAvmIXJuQCq4gQGLxq+xIbdF3AUPU=",
		"Program Ccip8888888888888888888888888888888888888888 success",
		"Program 6LLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLL consumed 131843 of 200000 compute units",
		"Program return: 6LLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLL S9pilw25WRal2CYAvmIXJuQCq4gQGLxq+xIbdF3AUPU=",
		"Program 6LLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLL success",
	}

	output, err := ParseProgramLogs(logs)
	require.NoError(t, err)

	require.Len(t, output, 1)
	assert.Len(t, output[0].Events, 1)
	event := output[0].Events[0]
	require.Equal(t, event.Program, "Ccip8888888888888888888888888888888888888888")
}

func TestLogDataParse_LogTruncated(t *testing.T) {
	logs := []string{
		"Program ComputeBudget111111111111111111111111111111 invoke [1]",
		"Program ComputeBudget111111111111111111111111111111 success",
		"Program ComputeBudget111111111111111111111111111111 invoke [1]",
		"Program ComputeBudget111111111111111111111111111111 success",
		"Program DF1ow4tspfHX9JwWJsAb9epbkA8hmpSEAtxXy1V27QBH invoke [1]",
		"Program log: Instruction: WrapSol",
		"Program ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL invoke [2]",
		"Program log: Create",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [3]",
		"Program log: Instruction: GetAccountDataSize",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 1569 of 539155 compute units",
		"Program return: TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA pQAAAAAAAAA=",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
		"Program 11111111111111111111111111111111 invoke [3]",
		"Program 11111111111111111111111111111111 success",
		"Program log: Initialize the associated token account",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [3]",
		"Program log: Instruction: InitializeImmutableOwner",
		"Program log: Please upgrade to SPL Token 2022 for immutable owner support",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 1405 of 532568 compute units",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [3]",
		"Program log: Instruction: InitializeAccount3",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 3158 of 528686 compute units",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
		"Program ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL consumed 19307 of 544552 compute units",
		"Program ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL success",
		"Program 11111111111111111111111111111111 invoke [2]",
		"Program 11111111111111111111111111111111 success",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [2]",
		"Program log: Instruction: SyncNative",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 3045 of 521783 compute units",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
		"Program DF1ow4tspfHX9JwWJsAb9epbkA8hmpSEAtxXy1V27QBH consumed 31276 of 549767 compute units",
		"Program DF1ow4tspfHX9JwWJsAb9epbkA8hmpSEAtxXy1V27QBH success",
		"Program DF1ow4tspfHX9JwWJsAb9epbkA8hmpSEAtxXy1V27QBH invoke [1]",
		"Program log: Instruction: TransferFee",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [2]",
		"Program log: Instruction: Transfer",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 4736 of 511672 compute units",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
		"Program DF1ow4tspfHX9JwWJsAb9epbkA8hmpSEAtxXy1V27QBH invoke [2]",
		"Program DF1ow4tspfHX9JwWJsAb9epbkA8hmpSEAtxXy1V27QBH consumed 2004 of 504428 compute units",
		"Program DF1ow4tspfHX9JwWJsAb9epbkA8hmpSEAtxXy1V27QBH success",
		"Program DF1ow4tspfHX9JwWJsAb9epbkA8hmpSEAtxXy1V27QBH consumed 16296 of 518491 compute units",
		"Program DF1ow4tspfHX9JwWJsAb9epbkA8hmpSEAtxXy1V27QBH success",
		"Program DF1ow4tspfHX9JwWJsAb9epbkA8hmpSEAtxXy1V27QBH invoke [1]",
		"Program log: Instruction: Swap2",
		"Program pAMMBay6oceH9fJKBRHGP5D4bD4sWpmSwMn52FMfXEA invoke [2]",
		"Program log: Instruction: Buy",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [3]",
		"Program log: Instruction: TransferChecked",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 6147 of 443742 compute units",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [3]",
		"Program log: Instruction: TransferChecked",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 6238 of 434796 compute units",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [3]",
		"Program log: Instruction: TransferChecked",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 6238 of 425773 compute units",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [3]",
		"Program log: Instruction: TransferChecked",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 6238 of 416748 compute units",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
		"Program data: Z/RSHyz1d3eYOlxoAAAAAH43BgEMAAAAfwZRVwAAAAAAAAAAAAAAAKHD818AAAAAC+yHXf0aAADn2P9kwwAAALYqDlcAAAAAFAAAAAAAAACFkiwAAAAAAAUAAAAAAAAAoiQLAAAAAAA7vTpXAAAAAH8GUVcAAAAAhk/4liHBkhF4HcBtgqYhWZzhNhKDJViTCFsP4uhMlONIX8qmVzagoSB6sDdk6lKaboYLxDag1o7HhdfO7EbgWpiUze6cg/NCfSJ/6p8D6Q5jX6u4O1owHLAMS3z47cbKqzEqHLmC+5FJslenfVmY4TlNtoqj6520TpcdH+Mg9mtgjMwd/OlhtDt3nBkVBabi079F1aTbRhitdsgtYXVFNV0OMm1Matf/fY9TP8MJCFS9H+8ykQhAD19V155Lh4BRHwWkuyQjYGtXJX8CYeHR6OVlndW7a/8+9vxuyRVJ31sFAAAAAAAAAKIkCwAAAAAA",
		"Program pAMMBay6oceH9fJKBRHGP5D4bD4sWpmSwMn52FMfXEA invoke [3]",
		"Program pAMMBay6oceH9fJKBRHGP5D4bD4sWpmSwMn52FMfXEA consumed 2009 of 404039 compute units",
		"Program pAMMBay6oceH9fJKBRHGP5D4bD4sWpmSwMn52FMfXEA success",
		"Program pAMMBay6oceH9fJKBRHGP5D4bD4sWpmSwMn52FMfXEA consumed 73618 of 475112 compute units",
		"Program pAMMBay6oceH9fJKBRHGP5D4bD4sWpmSwMn52FMfXEA success",
		"Program DF1ow4tspfHX9JwWJsAb9epbkA8hmpSEAtxXy1V27QBH invoke [2]",
		"Program DF1ow4tspfHX9JwWJsAb9epbkA8hmpSEAtxXy1V27QBH consumed 2004 of 398105 compute units",
		"Program DF1ow4tspfHX9JwWJsAb9epbkA8hmpSEAtxXy1V27QBH success",
		"Program ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL invoke [2]",
		"Program log: Create",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [3]",
		"Program log: Instruction: GetAccountDataSize",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 1622 of 387270 compute units",
		"Program return: TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA pQAAAAAAAAA=",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
		"Program 11111111111111111111111111111111 invoke [3]",
		"Program 11111111111111111111111111111111 success",
		"Program log: Initialize the associated token account",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [3]",
		"Program log: Instruction: InitializeImmutableOwner",
		"Program log: Please upgrade to SPL Token 2022 for immutable owner support",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 1405 of 380630 compute units",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [3]",
		"Program log: Instruction: InitializeAccount3",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 4241 of 376748 compute units",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
		"Program ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL consumed 20443 of 392667 compute units",
		"Program ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL success",
		"Program SoLFiHG9TfgtdUXUjWAxi3LtvYuFyDLVhBWxdMZxyCe invoke [2]",
		"Program data: tmoD75Zmz4Gqu59SB+c5FZQ72puP+kpcOH0XBK6lJ2Oj0ZsCAAAAAN6zzxQAAAAAAAAAAAAAAAA=",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [3]",
		"Program log: Instruction: Transfer",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 4736 of 207190 compute units",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [3]",
		"Program log: Instruction: Transfer",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 4645 of 200314 compute units",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
		"Program SoLFiHG9TfgtdUXUjWAxi3LtvYuFyDLVhBWxdMZxyCe consumed 171718 of 367147 compute units",
		"Program SoLFiHG9TfgtdUXUjWAxi3LtvYuFyDLVhBWxdMZxyCe success",
		"Program DF1ow4tspfHX9JwWJsAb9epbkA8hmpSEAtxXy1V27QBH invoke [2]",
		"Program DF1ow4tspfHX9JwWJsAb9epbkA8hmpSEAtxXy1V27QBH consumed 2004 of 192298 compute units",
		"Program DF1ow4tspfHX9JwWJsAb9epbkA8hmpSEAtxXy1V27QBH success",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [2]",
		"Program log: Instruction: CloseAccount",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 2915 of 188386 compute units",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
		"Program pAMMBay6oceH9fJKBRHGP5D4bD4sWpmSwMn52FMfXEA invoke [2]",
		"Program log: Instruction: Buy",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [3]",
		"Program log: Instruction: TransferChecked",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 6147 of 139166 compute units",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [3]",
		"Program log: Instruction: TransferChecked",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 6200 of 130220 compute units",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [3]",
		"Program log: Instruction: TransferChecked",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 6200 of 121235 compute units",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
		"Program data: Z/RSHyz1d3eYOlxoAAAAAKYZnS8BAAAAob89AQAAAAB+NwYBDAAAAKG/PQEAAAAArhAjR04BAAD20rpbAQAAAMX0PAEAAAAAFAAAAAAAAABJogAAAAAAAAUAAAAAAAAAkygAAAAAAAAOlz0BAAAAAKG/PQEAAAAAQD67U8Qzds+CwERPN/g0wWtR9F/xGD0+3bER9D8HiVlIX8qmVzagoSB6sDdk6lKaboYLxDag1o7HhdfO7EbgWpiUze6cg/NCfSJ/6p8D6Q5jX6u4O1owHLAMS3z47cbKy74M0Wx+QW0tvqFHOLv4LljBjRFBgoEWbZz/9++ZhGZKwvjQ3Vy8l+MonBl8tQYqVPPZVrnOblEV+WVnqlyz5qEI5CrzFl4Yln3NDKIWs/ApsdEXfwZIT4j1EHP1D2Z5AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAFAAAAAAAAAAAAAAAAAAAA",
		"Program pAMMBay6oceH9fJKBRHGP5D4bD4sWpmSwMn52FMfXEA invoke [3]",
		"Program pAMMBay6oceH9fJKBRHGP5D4bD4sWpmSwMn52FMfXEA consumed 2009 of 108562 compute units",
		"Program pAMMBay6oceH9fJKBRHGP5D4bD4sWpmSwMn52FMfXEA success",
		"Program pAMMBay6oceH9fJKBRHGP5D4bD4sWpmSwMn52FMfXEA consumed 67258 of 173275 compute units",
		"Program pAMMBay6oceH9fJKBRHGP5D4bD4sWpmSwMn52FMfXEA success",
		"Program DF1ow4tspfHX9JwWJsAb9epbkA8hmpSEAtxXy1V27QBH invoke [2]",
		"Program DF1ow4tspfHX9JwWJsAb9epbkA8hmpSEAtxXy1V27QBH consumed 2004 of 102628 compute units",
		"Program DF1ow4tspfHX9JwWJsAb9epbkA8hmpSEAtxXy1V27QBH success",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [2]",
		"Program log: Instruction: CloseAccount",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 2916 of 98717 compute units",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
		"Program DF1ow4tspfHX9JwWJsAb9epbkA8hmpSEAtxXy1V27QBH consumed 410425 of 502195 compute units",
		"Program DF1ow4tspfHX9JwWJsAb9epbkA8hmpSEAtxXy1V27QBH success",
		"Log truncated",
		"Program 11111111111111111111111111111111 success",
	}

	output, err := ParseProgramLogs(logs)
	require.NoError(t, err)
	require.Len(t, output, 5)

	require.False(t, output[0].Truncated)
	require.Len(t, output[0].Events, 0)
	require.Equal(t, "ComputeBudget111111111111111111111111111111", output[0].Program)

	require.False(t, output[1].Truncated)
	require.Len(t, output[1].Events, 0)
	require.Equal(t, "ComputeBudget111111111111111111111111111111", output[1].Program)

	require.False(t, output[2].Truncated)
	require.Len(t, output[2].Events, 0)
	require.Equal(t, "DF1ow4tspfHX9JwWJsAb9epbkA8hmpSEAtxXy1V27QBH", output[2].Program)

	require.False(t, output[3].Truncated)
	require.Len(t, output[3].Events, 0)
	require.Equal(t, "DF1ow4tspfHX9JwWJsAb9epbkA8hmpSEAtxXy1V27QBH", output[3].Program)

	require.True(t, output[4].Truncated)
	require.Len(t, output[4].Events, 3)
	require.Equal(t, "DF1ow4tspfHX9JwWJsAb9epbkA8hmpSEAtxXy1V27QBH", output[4].Program)
}

// Tests based on https://github.com/solana-foundation/anchor/blob/d9ef37b/ts/packages/anchor/tests/events.spec.ts
func TestLogDataParse_MultipleInstructions(t *testing.T) {
	t.Parallel()

	logs := []string{
		"Program 11111111111111111111111111111111 invoke [1]",
		"Program 11111111111111111111111111111111 success",
		"Program J2XMGdW2qQLx7rAdwWtSZpTXDgAQ988BLP9QTgUZvm54 invoke [1]",
		"Program J2XMGdW2qQLx7rAdwWtSZpTXDgAQ988BLP9QTgUZvm54 consumed 17867 of 200000 compute units",
		"Program J2XMGdW2qQLx7rAdwWtSZpTXDgAQ988BLP9QTgUZvm54 success",
	}

	output, err := ParseProgramLogs(logs)
	require.NoError(t, err)

	expected := []types.ProgramOutput{
		{
			Program: "11111111111111111111111111111111",
		},
		{
			Program:      "J2XMGdW2qQLx7rAdwWtSZpTXDgAQ988BLP9QTgUZvm54",
			ComputeUnits: 17867,
		},
	}

	requireExpected(t, expected, output)
}

func TestLogDataParse_MultipleTopLevelInstructions(t *testing.T) {
	t.Parallel()

	logs := []string{
		"Upgraded program J2XMGdW2qQLx7rAdwWtSZpTXDgAQ988BLP9QTgUZvm54", // this is ignored, we don't care about upgrades
		"Program J1zQwrBNBngz26jRPNWsUSZMHJwBwpkoDitXRV95LdK4 invoke [1]",
		"Program log: Instruction: CreateLog",
		"Program data: HDQnaQjSWwkNAAAASGVsbG8sIFdvcmxkISoAAAAAAAAA", // base64 encoded; borsh encoded with identifier
		"Program J1zQwrBNBngz26jRPNWsUSZMHJwBwpkoDitXRV95LdK4 consumed 1477 of 200000 compute units",
		"Program J1zQwrBNBngz26jRPNWsUSZMHJwBwpkoDitXRV95LdK4 success",
		"Program J2XMGdW2qQLx7rAdwWtSZpTXDgAQ988BLP9QTgUZvm54 invoke [1]",
		"Program J2XMGdW2qQLx7rAdwWtSZpTXDgAQ988BLP9QTgUZvm54 consumed 17867 of 200000 compute units",
		"Program J2XMGdW2qQLx7rAdwWtSZpTXDgAQ988BLP9QTgUZvm54 success",
	}

	output, err := ParseProgramLogs(logs)
	require.NoError(t, err)

	expected := []types.ProgramOutput{
		{
			Program:      "J1zQwrBNBngz26jRPNWsUSZMHJwBwpkoDitXRV95LdK4",
			ComputeUnits: 1477,
			Events: []types.ProgramEvent{
				{
					Program: "J1zQwrBNBngz26jRPNWsUSZMHJwBwpkoDitXRV95LdK4",
					Data:    "HDQnaQjSWwkNAAAASGVsbG8sIFdvcmxkISoAAAAAAAAA",
				},
			},
			Logs: []types.ProgramLog{{
				Text:   "Instruction: CreateLog",
				Prefix: ">",
			}},
		},
		{
			Program:      "J2XMGdW2qQLx7rAdwWtSZpTXDgAQ988BLP9QTgUZvm54",
			ComputeUnits: 17867,
		},
	}

	requireExpected(t, expected, output)
}

func TestLogDataParse_DifferentStartLog(t *testing.T) {
	t.Parallel()

	// example program log output from solana explorer
	logs := []string{
		"Upgraded program J2XMGdW2qQLx7rAdwWtSZpTXDgAQ988BLP9QTgUZvm54",
		"Program J2XMGdW2qQLx7rAdwWtSZpTXDgAQ988BLP9QTgUZvm54 invoke [1]",
		"Program log: Instruction: BuyNft",
		"Program 11111111111111111111111111111111 invoke [2]",
		"Program data: UhUxVlc2hGeTBjNPCGmmZjvNSuBOYpfpRPJLfJmTLZueJAmbgEtIMGl9lLKKH6YKy1AQd8lrsdJPPc7joZ6kCkEKlNLKhbUv",
		"Program 11111111111111111111111111111111 success",
		"Program 11111111111111111111111111111111 invoke [2]",
		"Program 11111111111111111111111111111111 success",
		"Program 11111111111111111111111111111111 invoke [2]",
		"Program 11111111111111111111111111111111 success",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [2]",
		"Program log: Instruction: Transfer",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 2549 of 141128 compute units",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [2]",
		"Program log: Instruction: CloseAccount",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 1745 of 135127 compute units",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
		"Program data: UhUxVlc2hGeTBjNPCGmmZjvNSuBOYpfpRPJLfJmTLZueJAmbgEtIMGl9lLKKH6YKy1AQd8lrsdJPPc7joZ6kCkEKlNLKhbUv",
		"Program J2XMGdW2qQLx7rAdwWtSZpTXDgAQ988BLP9QTgUZvm54 consumed 73106 of 200000 compute units",
		"Program J2XMGdW2qQLx7rAdwWtSZpTXDgAQ988BLP9QTgUZvm54 success",
	}

	output, err := ParseProgramLogs(logs)
	require.NoError(t, err)

	expected := []types.ProgramOutput{{
		Program: "J2XMGdW2qQLx7rAdwWtSZpTXDgAQ988BLP9QTgUZvm54",
		Logs: []types.ProgramLog{
			{Prefix: ">", Text: "Instruction: BuyNft"},
			{Prefix: ">>", Text: "Instruction: Transfer"},
			{Prefix: ">>", Text: "Instruction: CloseAccount"},
		},
		Events: []types.ProgramEvent{
			{
				Program: "11111111111111111111111111111111",
				Data:    "UhUxVlc2hGeTBjNPCGmmZjvNSuBOYpfpRPJLfJmTLZueJAmbgEtIMGl9lLKKH6YKy1AQd8lrsdJPPc7joZ6kCkEKlNLKhbUv",
			},
			{
				Program: "J2XMGdW2qQLx7rAdwWtSZpTXDgAQ988BLP9QTgUZvm54",
				Data:    "UhUxVlc2hGeTBjNPCGmmZjvNSuBOYpfpRPJLfJmTLZueJAmbgEtIMGl9lLKKH6YKy1AQd8lrsdJPPc7joZ6kCkEKlNLKhbUv",
				BlockData: types.BlockData{
					TransactionLogIndex: 1,
				},
			},
		},
		ComputeUnits: 73106,
	}}

	requireExpected(t, expected, output)
}

func TestLogDataParse_FindEvent(t *testing.T) {
	t.Parallel()

	// example program log output from solana explorer
	logs := []string{
		"Program J2XMGdW2qQLx7rAdwWtSZpTXDgAQ988BLP9QTgUZvm54 invoke [1]",
		"Program log: Instruction: CancelListing",
		"Program log: TRANSFERRED SOME TOKENS",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [2]",
		"Program log: Instruction: Transfer",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 2549 of 182795 compute units",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
		"Program log: TRANSFERRED SOME TOKENS",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [2]",
		"Program log: Instruction: CloseAccount",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 1745 of 176782 compute units",
		"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
		"Program data: Vtv9xLjCsE60Ati9kl3VVU/5y8DMMeC4LaGdMLkX8WU+G59Wsi3wfky8rnO9otGb56CTRerWx3hB5M/SlRYBdht0fi+crAgFYsJcx2CHszpSWRkXNxYQ6DxQ/JqIvKnLC/8Mln7310A=",
		"Program J2XMGdW2qQLx7rAdwWtSZpTXDgAQ988BLP9QTgUZvm54 consumed 31435 of 200000 compute units",
		"Program J2XMGdW2qQLx7rAdwWtSZpTXDgAQ988BLP9QTgUZvm54 success",
	}

	output, err := ParseProgramLogs(logs)
	require.NoError(t, err)

	expected := []types.ProgramOutput{{
		Program: "J2XMGdW2qQLx7rAdwWtSZpTXDgAQ988BLP9QTgUZvm54",
		Logs: []types.ProgramLog{
			{Prefix: ">", Text: "Instruction: CancelListing"},
			{Prefix: ">", Text: "TRANSFERRED SOME TOKENS"},
			{Prefix: ">>", Text: "Instruction: Transfer"},
			{Prefix: ">", Text: "TRANSFERRED SOME TOKENS"},
			{Prefix: ">>", Text: "Instruction: CloseAccount"},
		},
		Events: []types.ProgramEvent{{
			Program: "J2XMGdW2qQLx7rAdwWtSZpTXDgAQ988BLP9QTgUZvm54",
			Data:    "Vtv9xLjCsE60Ati9kl3VVU/5y8DMMeC4LaGdMLkX8WU+G59Wsi3wfky8rnO9otGb56CTRerWx3hB5M/SlRYBdht0fi+crAgFYsJcx2CHszpSWRkXNxYQ6DxQ/JqIvKnLC/8Mln7310A=",
		}},
		ComputeUnits: 31435,
	}}

	requireExpected(t, expected, output)
}

func TestLogDataParse_ProgramLogSuccess(t *testing.T) {
	t.Parallel()

	// example program log output from solana explorer
	logs := []string{
		"Program fake111111111111111111111111111111111111112 invoke [1]",
		"Program log: i logged success",
		"Program log: i logged success",
		"Program fake111111111111111111111111111111111111112 consumed 1411 of 200000 compute units",
		"Program fake111111111111111111111111111111111111112 success",
	}

	output, err := ParseProgramLogs(logs)
	require.NoError(t, err)

	expected := []types.ProgramOutput{{
		Program: "fake111111111111111111111111111111111111112",
		Logs: []types.ProgramLog{
			{Prefix: ">", Text: "i logged success"},
			{Prefix: ">", Text: "i logged success"},
		},
		ComputeUnits: 1411,
	}}

	requireExpected(t, expected, output)
}

func requireExpected(t *testing.T, expected []types.ProgramOutput, output []types.ProgramOutput) {
	t.Helper()
	require.Len(t, output, len(expected))
	for i, o := range output {
		require.Equal(t, expected[i], o, "mismatch at index %d", i)
	}
}

var _ = debugPrintOutput // hack to mark the debug function as used, so the linter doesn't complain

// helper function to print output for debugging, to be used on-demand during development
func debugPrintOutput(t *testing.T, output []types.ProgramOutput) {
	t.Helper()
	j, err := json.MarshalIndent(output, "", "  ")
	require.NoError(t, err)
	fmt.Printf("%s\n", j)
}
