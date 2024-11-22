package logpoller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	output := parseProgramLogs(logs)

	require.Len(t, output, 2)

	// first program has no logs, no events, no compute units and succeeded
	assert.Equal(t, ProgramOutput{
		Program: "ComputeBudget111111111111111111111111111111",
	}, output[0])

	// second program should have one log, no events, 6504 compute units and failed with error message
	expected := ProgramOutput{
		Program: "cjg3oHmg9uuPsP8D6g29NWvhySJkdYdAo9D25PRbKXJ",
		Logs: []ProgramLog{
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

	output := parseProgramLogs(logs)

	require.Len(t, output, 2)

	// first program has no logs, no events, no compute units and succeeded
	assert.Equal(t, ProgramOutput{
		Program: "ComputeBudget111111111111111111111111111111",
	}, output[0])

	// second program should have one log, no events, 6504 compute units and failed with error message
	expected := ProgramOutput{
		Program: "SAGE2HAwep459SNq61LHvjxPk4pLPEJLoMETef7f7EE",
		Logs: []ProgramLog{
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

	output := parseProgramLogs(logs)

	require.Len(t, output, 11)

	// first two programs have no logs, no events, no compute units and succeeded
	for idx := range 1 {
		assert.Equal(t, ProgramOutput{
			Program: "ComputeBudget111111111111111111111111111111",
		}, output[idx])
	}

	expectedSystemProgramIdxs := []int{2, 7, 9}
	for _, idx := range expectedSystemProgramIdxs {
		assert.Equal(t, ProgramOutput{
			Program: "11111111111111111111111111111111",
		}, output[idx])
	}

	require.Len(t, output[4].Logs, 6)
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

	output := parseProgramLogs(logs)

	require.Len(t, output, 1)
	assert.Len(t, output[0].Events, 1)
}
