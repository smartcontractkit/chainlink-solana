package codecv2_test

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"testing"

	anchoridl "github.com/gagliardetto/anchor-go/idl"
	"github.com/test-go/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings/binary"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codecv2"
)

//go:embed testutils/data_storage.json
var dataStorageIdl string

func TestNewEventArgsEntry(t *testing.T) {
	var idl anchoridl.Idl
	if err := json.Unmarshal([]byte(dataStorageIdl), &idl); err != nil {
		t.Fatalf("unexpected error: invalid Data Storage IDL, error: %v", err)
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

func TestCreateCodecEntry(t *testing.T) {
	var idl anchoridl.Idl
	if err := json.Unmarshal([]byte(dataStorageIdl), &idl); err != nil {
		t.Fatalf("unexpected error: invalid Data Storage IDL, error: %v", err)
	}
	for i, event := range idl.Events {
		entry, err := codecv2.CreateCodecEntry(event, fmt.Sprintf("test%d", i), idl, nil)
		require.NoError(t, err)
		require.NotNil(t, entry)
	}
	for i, instruction := range idl.Instructions {
		if instruction.Name == "initialize_data_account" {
			entry, err := codecv2.CreateCodecEntry(instruction, fmt.Sprintf("test%d", i), idl, nil)
			require.NoError(t, err)
			require.NotNil(t, entry)
		}
	}
}
