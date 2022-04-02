package event

import (
	"encoding/binary"
	"testing"

	bin "github.com/gagliardetto/binary"
	"github.com/stretchr/testify/require"
)

func TestExtractEvents(t *testing.T) {
	programIDBase58 := "STGhiM1ZaLjDLZDGcVFp3ppdetggLAs6MXezw5DXXH3"
	groupsOfLogs := [][]string{
		{
			"Program STGhiM1ZaLjDLZDGcVFp3ppdetggLAs6MXezw5DXXH3 invoke [1]",
			"Program log: gjbLTR5rT6iaSQcAAAMQumV5CqMwMWjU5bBudJS4G7Kr1YGm1javi5Tpf4Y3dOLMJAAAAAAAAAAAAAAAAigtRmIQCAEOCQ8EBgcFAwoLDA0CAAAAAADKmjsAAAAAiBMAAAAAAAA=",
			"Program STGxAk2tuSMv7iwt2vRRuijRp1ageiRcwrjhdPBsAXn invoke [2]",
			"Program log: Instruction: Submit",
			"Program STGxAk2tuSMv7iwt2vRRuijRp1ageiRcwrjhdPBsAXn consumed 3587 of 22659 compute units",
			"Program STGxAk2tuSMv7iwt2vRRuijRp1ageiRcwrjhdPBsAXn success",
			"Program STGhiM1ZaLjDLZDGcVFp3ppdetggLAs6MXezw5DXXH3 consumed 181502 of 200000 compute units",
			"Program STGhiM1ZaLjDLZDGcVFp3ppdetggLAs6MXezw5DXXH3 success",
		},
		{
			"Program STGhiM1ZaLjDLZDGcVFp3ppdetggLAs6MXezw5DXXH3 invoke [1]",
			"Program log: gjbLTR5rT6gSuQgAAAMuN5qPxmWZqcAitDRnFkdaJhqJ0WBRnjrLH9CzWkg3dOLMJAAAAAAAAAAAAAAACQ8tRmIQCAEOCQ8EBgcFAwoLDA0CAAAAAADKmjsAAAAAiBMAAAAAAAA=",
			"Program STGxAk2tuSMv7iwt2vRRuijRp1ageiRcwrjhdPBsAXn invoke [2]",
			"Program log: Instruction: Submit",
			"Program STGxAk2tuSMv7iwt2vRRuijRp1ageiRcwrjhdPBsAXn consumed 3587 of 123958 compute units",
			"Program STGxAk2tuSMv7iwt2vRRuijRp1ageiRcwrjhdPBsAXn success",
			"Program STGhiM1ZaLjDLZDGcVFp3ppdetggLAs6MXezw5DXXH3 consumed 80203 of 200000 compute units",
			"Program STGhiM1ZaLjDLZDGcVFp3ppdetggLAs6MXezw5DXXH3 success",
		},
		{
			"Program STGhiM1ZaLjDLZDGcVFp3ppdetggLAs6MXezw5DXXH3 invoke [1]",
			"Program log: gjbLTR5rT6inQwcAAANy+x/LIETrs7naC0mc49puOD3+fSA+Mmunk2j5gKg3dOLMJAAAAAAAAAAAAAAAAA8tRmIQCAEOCQ8EBgcFAwoLDA0CAAAAAADKmjsAAAAAiBMAAAAAAAA=",
			"Program STGxAk2tuSMv7iwt2vRRuijRp1ageiRcwrjhdPBsAXn invoke [2]",
			"Program log: Instruction: Submit",
			"Program STGxAk2tuSMv7iwt2vRRuijRp1ageiRcwrjhdPBsAXn consumed 3587 of 22709 compute units",
			"Program STGxAk2tuSMv7iwt2vRRuijRp1ageiRcwrjhdPBsAXn success",
			"Program STGhiM1ZaLjDLZDGcVFp3ppdetggLAs6MXezw5DXXH3 consumed 181452 of 200000 compute units",
			"Program STGhiM1ZaLjDLZDGcVFp3ppdetggLAs6MXezw5DXXH3 success",
		},
		{
			"Program STGhiM1ZaLjDLZDGcVFp3ppdetggLAs6MXezw5DXXH3 invoke [1]",
			"Program log: gjbLTR5rT6jhSwcAAAPLxuP0SjlzlEc3F2dlPyLOzIAeQnF05dG067WUiq43dOLMJAAAAAAAAAAAAAAAChAtRmIQCAEOCQ8EBgcFAwoLDA0CAAAAAADKmjsAAAAAiBMAAAAAAAA=",
			"Program STGxAk2tuSMv7iwt2vRRuijRp1ageiRcwrjhdPBsAXn invoke [2]",
			"Program log: Instruction: Submit",
			"Program STGxAk2tuSMv7iwt2vRRuijRp1ageiRcwrjhdPBsAXn consumed 3587 of 22243 compute units",
			"Program STGxAk2tuSMv7iwt2vRRuijRp1ageiRcwrjhdPBsAXn success",
			"Program STGhiM1ZaLjDLZDGcVFp3ppdetggLAs6MXezw5DXXH3 consumed 181918 of 200000 compute units",
			"Program STGhiM1ZaLjDLZDGcVFp3ppdetggLAs6MXezw5DXXH3 success",
		},
		{
			"Program STGhiM1ZaLjDLZDGcVFp3ppdetggLAs6MXezw5DXXH3 invoke [1]",
			"Program log: gjbLTR5rT6iSSQcAAAMQumV5CqMwMWjU5bBudJS4G7Kr1YGm1javi5Tpf4Y3dOLMJAAAAAAAAAAAAAAABhAtRmIQCAEOCQ8EBgcFAwoLDA0CAAAAAADKmjsAAAAAiBMAAAAAAAA=",
			"Program STGxAk2tuSMv7iwt2vRRuijRp1ageiRcwrjhdPBsAXn invoke [2]",
			"Program log: Instruction: Submit",
			"Program STGxAk2tuSMv7iwt2vRRuijRp1ageiRcwrjhdPBsAXn consumed 3587 of 22513 compute units",
			"Program STGxAk2tuSMv7iwt2vRRuijRp1ageiRcwrjhdPBsAXn success",
			"Program STGhiM1ZaLjDLZDGcVFp3ppdetggLAs6MXezw5DXXH3 consumed 181648 of 200000 compute units",
			"Program STGhiM1ZaLjDLZDGcVFp3ppdetggLAs6MXezw5DXXH3 success",
		},
		{
			"Program STGhiM1ZaLjDLZDGcVFp3ppdetggLAs6MXezw5DXXH3 invoke [1]",
			"Program log: gjbLTR5rT6gFSwcAAAOO/bNYwbBGoNcZhvTwFVHkSRI9vN9nDBQaU9Ocfy03dOLMJAAAAAAAAAAAAAAAAREtRmIQCAEOCQ8EBgcFAwoLDA0CAAAAAADKmjsAAAAAiBMAAAAAAAA=",
			"Program STGxAk2tuSMv7iwt2vRRuijRp1ageiRcwrjhdPBsAXn invoke [2]",
			"Program log: Instruction: Submit",
			"Program STGxAk2tuSMv7iwt2vRRuijRp1ageiRcwrjhdPBsAXn consumed 3587 of 22743 compute units",
			"Program STGxAk2tuSMv7iwt2vRRuijRp1ageiRcwrjhdPBsAXn success",
			"Program STGhiM1ZaLjDLZDGcVFp3ppdetggLAs6MXezw5DXXH3 consumed 181418 of 200000 compute units",
			"Program STGhiM1ZaLjDLZDGcVFp3ppdetggLAs6MXezw5DXXH3 success",
		},
		{
			"Program STGhiM1ZaLjDLZDGcVFp3ppdetggLAs6MXezw5DXXH3 invoke [1]",
			"Program log: gjbLTR5rT6iwQwcAAAM6wHcIzwrEysN7tds4vrXRJIBZlnB3bbc/91U47g03dOLMJAAAAAAAAAAAAAAACBQtRmIQCAEOCQ8EBgcFAwoLDA0CAAAAAADKmjsAAAAAiBMAAAAAAAA=",
			"Program STGxAk2tuSMv7iwt2vRRuijRp1ageiRcwrjhdPBsAXn invoke [2]",
			"Program log: Instruction: Submit",
			"Program STGxAk2tuSMv7iwt2vRRuijRp1ageiRcwrjhdPBsAXn consumed 3587 of 22217 compute units",
			"Program STGxAk2tuSMv7iwt2vRRuijRp1ageiRcwrjhdPBsAXn success",
			"Program STGhiM1ZaLjDLZDGcVFp3ppdetggLAs6MXezw5DXXH3 consumed 181944 of 200000 compute units",
			"Program STGhiM1ZaLjDLZDGcVFp3ppdetggLAs6MXezw5DXXH3 success",
		},
		{
			"Program STGhiM1ZaLjDLZDGcVFp3ppdetggLAs6MXezw5DXXH3 invoke [1]",
			"Program log: gjbLTR5rT6iUSQcAAAMQumV5CqMwMWjU5bBudJS4G7Kr1YGm1javi5Tpf4Y3dOLMJAAAAAAAAAAAAAAAAhYtRmIQCAEOCQ8EBgcFAwoLDA0CAAAAAADKmjsAAAAAiBMAAAAAAAA=",
			"Program STGxAk2tuSMv7iwt2vRRuijRp1ageiRcwrjhdPBsAXn invoke [2]",
			"Program log: Instruction: Submit",
			"Program STGxAk2tuSMv7iwt2vRRuijRp1ageiRcwrjhdPBsAXn consumed 3587 of 22616 compute units",
			"Program STGxAk2tuSMv7iwt2vRRuijRp1ageiRcwrjhdPBsAXn success",
			"Program STGhiM1ZaLjDLZDGcVFp3ppdetggLAs6MXezw5DXXH3 consumed 181545 of 200000 compute units",
			"Program STGhiM1ZaLjDLZDGcVFp3ppdetggLAs6MXezw5DXXH3 success",
		},
		{
			"Program STGhiM1ZaLjDLZDGcVFp3ppdetggLAs6MXezw5DXXH3 invoke [1]",
			"Program log: gjbLTR5rT6iqQwcAAANy+x/LIETrs7naC0mc49puOD3+fSA+Mmunk2j5gKg3dOLMJAAAAAAAAAAAAAAAChgtRmIQCAEOCQ8EBgcFAwoLDA0CAAAAAADKmjsAAAAAiBMAAAAAAAA=",
			"Program STGxAk2tuSMv7iwt2vRRuijRp1ageiRcwrjhdPBsAXn invoke [2]",
			"Program log: Instruction: Submit",
			"Program STGxAk2tuSMv7iwt2vRRuijRp1ageiRcwrjhdPBsAXn consumed 3587 of 22329 compute units",
			"Program STGxAk2tuSMv7iwt2vRRuijRp1ageiRcwrjhdPBsAXn success",
			"Program STGhiM1ZaLjDLZDGcVFp3ppdetggLAs6MXezw5DXXH3 consumed 181832 of 200000 compute units",
			"Program STGhiM1ZaLjDLZDGcVFp3ppdetggLAs6MXezw5DXXH3 success",
		},
	}
	expectedEvents := [][]string{
		{"gjbLTR5rT6iaSQcAAAMQumV5CqMwMWjU5bBudJS4G7Kr1YGm1javi5Tpf4Y3dOLMJAAAAAAAAAAAAAAAAigtRmIQCAEOCQ8EBgcFAwoLDA0CAAAAAADKmjsAAAAAiBMAAAAAAAA="},
		{"gjbLTR5rT6gSuQgAAAMuN5qPxmWZqcAitDRnFkdaJhqJ0WBRnjrLH9CzWkg3dOLMJAAAAAAAAAAAAAAACQ8tRmIQCAEOCQ8EBgcFAwoLDA0CAAAAAADKmjsAAAAAiBMAAAAAAAA="},
		{"gjbLTR5rT6inQwcAAANy+x/LIETrs7naC0mc49puOD3+fSA+Mmunk2j5gKg3dOLMJAAAAAAAAAAAAAAAAA8tRmIQCAEOCQ8EBgcFAwoLDA0CAAAAAADKmjsAAAAAiBMAAAAAAAA="},
		{"gjbLTR5rT6jhSwcAAAPLxuP0SjlzlEc3F2dlPyLOzIAeQnF05dG067WUiq43dOLMJAAAAAAAAAAAAAAAChAtRmIQCAEOCQ8EBgcFAwoLDA0CAAAAAADKmjsAAAAAiBMAAAAAAAA="},
		{"gjbLTR5rT6iSSQcAAAMQumV5CqMwMWjU5bBudJS4G7Kr1YGm1javi5Tpf4Y3dOLMJAAAAAAAAAAAAAAABhAtRmIQCAEOCQ8EBgcFAwoLDA0CAAAAAADKmjsAAAAAiBMAAAAAAAA="},
		{"gjbLTR5rT6gFSwcAAAOO/bNYwbBGoNcZhvTwFVHkSRI9vN9nDBQaU9Ocfy03dOLMJAAAAAAAAAAAAAAAAREtRmIQCAEOCQ8EBgcFAwoLDA0CAAAAAADKmjsAAAAAiBMAAAAAAAA="},
		{"gjbLTR5rT6iwQwcAAAM6wHcIzwrEysN7tds4vrXRJIBZlnB3bbc/91U47g03dOLMJAAAAAAAAAAAAAAACBQtRmIQCAEOCQ8EBgcFAwoLDA0CAAAAAADKmjsAAAAAiBMAAAAAAAA="},
		{"gjbLTR5rT6iUSQcAAAMQumV5CqMwMWjU5bBudJS4G7Kr1YGm1javi5Tpf4Y3dOLMJAAAAAAAAAAAAAAAAhYtRmIQCAEOCQ8EBgcFAwoLDA0CAAAAAADKmjsAAAAAiBMAAAAAAAA="},
		{"gjbLTR5rT6iqQwcAAANy+x/LIETrs7naC0mc49puOD3+fSA+Mmunk2j5gKg3dOLMJAAAAAAAAAAAAAAAChgtRmIQCAEOCQ8EBgcFAwoLDA0CAAAAAADKmjsAAAAAiBMAAAAAAAA="},
	}
	for i, logs := range groupsOfLogs {
		actualEvents := ExtractEvents(logs, programIDBase58)
		require.Equal(t, expectedEvents[i], actualEvents)
	}
}

func TestDecode(t *testing.T) {
	encoded := "gjbLTR5rT6gW2QgAAAPLxuP0SjlzlEc3F2dlPyLOzIAeQnF05dG067WUiq7xYyfUMAAAAAAAAAAAAAAADZTbSmIQCAEOCQ8EBgcFAwoLDA0CAAAAAADKmjsAAAAAiBMAAAAAAAA="
	expected := NewTransmission{
		RoundID:               0x8d916,
		ConfigDigest:          [32]uint8{0x0, 0x3, 0xcb, 0xc6, 0xe3, 0xf4, 0x4a, 0x39, 0x73, 0x94, 0x47, 0x37, 0x17, 0x67, 0x65, 0x3f, 0x22, 0xce, 0xcc, 0x80, 0x1e, 0x42, 0x71, 0x74, 0xe5, 0xd1, 0xb4, 0xeb, 0xb5, 0x94, 0x8a, 0xae},
		Answer:                bin.Int128{Lo: 0x30d42763f1, Hi: 0x0, Endianness: binary.ByteOrder(nil)},
		Transmitter:           0xd,
		ObservationsTimestamp: 0x624adb94,
		ObserverCount:         0x10,
		Observers:             [19]uint8{0x8, 0x1, 0xe, 0x9, 0xf, 0x4, 0x6, 0x7, 0x5, 0x3, 0xa, 0xb, 0xc, 0xd, 0x2, 0x0, 0x0, 0x0, 0x0},
		JuelsPerLamport:       0x3b9aca00,
		ReimbursementGJuels:   0x1388,
	}
	decoded, err := Decode(encoded)
	require.NoError(t, err)
	require.Equal(t, expected, decoded)
}
