package solana

import (
	"encoding/binary"
	"math/big"
	"testing"

	sol_binary "github.com/gagliardetto/binary"
	consensustypes "github.com/smartcontractkit/chainlink-common/pkg/capabilities/consensus/ocr3/types"
	ocr3types "github.com/smartcontractkit/chainlink-common/pkg/capabilities/consensus/ocr3/types"
	"github.com/smartcontractkit/chainlink-protos/cre/go/values"
	"github.com/stretchr/testify/require"
)

var (
	workflowID       = "15c631d295ef5e32deb99a10ee6804bc4af1385568f9b3363f6552ac6dbb2cef"
	workflowName     = "aabbccddeeaabbccddee"
	donID            = uint32(2)
	executionID      = "8d4e66421db647dd916d3ec28d56188c8d7dae5f808e03d03339ed2562f13bb0"
	workflowOwnerID  = "0000000000000000000000000000000000000000"
	reportID         = "9988"
	timestampInt     = uint32(1234567890)
	configVersionInt = uint32(1)
)

func Test_capEncoder(t *testing.T) {
	cfg := map[string]any{
		reportSchemaKey: `{
      "kind": "struct",
      "fields": [
        { "name": "payload", "type": { "vec": { "defined": "DecimalReport" } } }
      ]
    }`,
		definedTypes: `
		[
      {
        "name":"DecimalReport",
         "type":{
          "kind":"struct",
          "fields":[
            { "name":"timestamp", "type":"u32" },
            { "name":"answer",    "type":"u128" },
            { "name": "dataId",   "type": {"array": ["u8",16]}}
          ]
        }
      }
    ]`,
	}
	mcfg, err := values.NewMap(cfg)
	require.NoError(t, err, "failed to make map")
	enc, err := NewEncoder(mcfg)
	require.NoError(t, err, "failed to create encoder")
	expTS := uint32(10)
	expAnswer := big.NewInt(14)
	expDataID := [16]byte{1, 2, 3, 4}
	expHash := [32]byte{7, 8, 9}
	m := map[string]any{
		"account_context_hash": expHash,
		"payload": []any{
			map[string]any{
				"Timestamp": expTS,
				"Answer":    expAnswer,
				"DataId":    expDataID,
			},
		},
		consensustypes.MetadataFieldName: getMetadata(workflowID),
	}

	in, err := values.NewMap(m)
	require.NoError(t, err, "failed to create in map")

	b, err := enc.Encode(t.Context(), *in)
	require.NoError(t, err, "failed to encode payload")
	_, trail, err := ocr3types.Decode(b)
	require.NoError(t, err, "failed to decode metadata")

	var fr ForwarderReport
	err = sol_binary.UnmarshalBorsh(&fr, trail)
	require.NoError(t, err, "failed to unmarshal borsh forwarder report")
	require.Equal(t, expHash, fr.Hash)
	type Result struct {
		Timestamp uint32
		Answer    [16]byte // little-endian
		DataID    [16]byte
	}

	var r []Result
	err = sol_binary.UnmarshalBorsh(&r, fr.Payload)
	require.NoError(t, err, "failed unmarshal borsh")
	require.Equal(t, expTS, r[0].Timestamp)
	n := binary.LittleEndian.Uint64(r[0].Answer[:])
	require.Equal(t, expAnswer, new(big.Int).SetUint64(n))
	require.Equal(t, expDataID, r[0].DataID)
}

func getMetadata(cid string) consensustypes.Metadata {
	return consensustypes.Metadata{
		Version:          1,
		ExecutionID:      executionID,
		Timestamp:        timestampInt,
		DONID:            donID,
		DONConfigVersion: configVersionInt,
		WorkflowID:       cid,
		WorkflowName:     workflowName,
		WorkflowOwner:    workflowOwnerID,
		ReportID:         reportID,
	}
}
