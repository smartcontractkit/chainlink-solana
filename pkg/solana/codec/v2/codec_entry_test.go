package codecv2_test

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"testing"

	anchoridl "github.com/gagliardetto/anchor-go/idl"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings/binary"
	codecv2 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v2"
)

//go:embed testutils/data_storage.json
var dataStorageIdl string

//go:embed testutils/testIDL.json
var testIDL string

//go:embed testutils/itemArray1TypeIDL.json
var itemArray1TypeIDL string

//go:embed testutils/eventItemTypeIDL.json
var eventItemTypeIDL string

//go:embed testutils/itemSliceTypeIDL.json
var itemSliceTypeIDL string

//go:embed testutils/itemArray2TypeIDL.json
var itemArray2TypeIDL string

//go:embed testutils/itemIDL.json
var itemIDL string

func TestNewEventArgsEntry(t *testing.T) {
	for _, idlString := range []string{dataStorageIdl, testIDL, itemArray1TypeIDL, eventItemTypeIDL, itemSliceTypeIDL, itemArray2TypeIDL, itemIDL} {
		var idl anchoridl.Idl
		if err := json.Unmarshal([]byte(idlString), &idl); err != nil {
			t.Fatalf("unexpected error: invalid IDL, error: %v", err)
		}
		for i, event := range idl.Events {
			entry, err := codecv2.NewEventArgsEntry(fmt.Sprintf("test%d", i), codecv2.EventIDLTypes{
				Event: event,
				Types: idl.Types,
			}, false, nil, binary.LittleEndian())
			require.NoError(t, err)
			require.NotNil(t, entry)
		}
	}
}

func TestNewInstructionArgsEntry(t *testing.T) {
	for _, idlString := range []string{dataStorageIdl, testIDL, itemArray1TypeIDL, eventItemTypeIDL, itemSliceTypeIDL, itemArray2TypeIDL, itemIDL} {
		var idl anchoridl.Idl
		if err := json.Unmarshal([]byte(idlString), &idl); err != nil {
			t.Fatalf("unexpected error: invalid IDL, error: %v", err)
		}
		for i, instruction := range idl.Instructions {
			if len(instruction.Args) > 0 {
				entry, err := codecv2.NewInstructionArgsEntry(fmt.Sprintf("test%d", i), codecv2.InstructionArgsIDLTypes{
					Instruction: instruction,
					Types:       idl.Types,
				}, nil, binary.LittleEndian())
				require.NoError(t, err)
				require.NotNil(t, entry)
			}
		}
	}
}
