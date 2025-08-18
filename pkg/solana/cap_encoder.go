package solana

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	consensustypes "github.com/smartcontractkit/chainlink-common/pkg/capabilities/consensus/ocr3/types"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/values"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
)

var (
	encoderName       = "user"
	chainSpecificName = "payload"
)

func NewEncoder(config *values.Map) (consensustypes.Encoder, error) {
	// TODO create config from input config
	idlJSON := `
{
  "version": "0.1.0",
  "name": "data_feeds_cache",
  "accounts": [
 {
      "name": "DecimalReports",
      "type": {
        "kind": "struct",
        "fields": [
          { "name": "reports", "type": { "vec": { "defined": "DecimalReport" } } }
        ]
      }
    }
  ],
  "types": [
    {
      "name": "DecimalReport",
      "type": {
        "kind": "struct",
        "fields": [
          { "name": "timestamp", "type": "u32" },
          { "name": "answer", "type": "u128" }
        ]
      }
    }
  ]
}`
	var idl codec.IDL
	err := json.Unmarshal([]byte(idlJSON), &idl)
	if err != nil {
		return nil, err
	}
	parsed := &codec.ParsedTypes{
		EncoderDefs: make(map[string]codec.Entry),
	}
	idlDef, err := codec.FindDefinitionFromIDL(codec.ChainConfigTypeAccountDef, "DecimalReports", idl)
	if err != nil {
		return nil, err
	}

	accountIDLDef, ok := idlDef.(codec.IdlTypeDef)
	if !ok {
		return nil, errors.New("invalid cast")
	}

	cEntry, err := codec.CreateCodecEntry(accountIDLDef, "DecimalReports", idl, nil)
	parsed.EncoderDefs[codec.WrapItemType(true, "user", "DecimalReports")] = cEntry
	c, err := parsed.ToCodec()
	if err != nil {
		return nil, err
	}

	return &capEncoder{codec: c}, err
}

type capEncoder struct {
	codec commontypes.RemoteCodec
}

func (e *capEncoder) Encode(ctx context.Context, input values.Map) ([]byte, error) {
	unwrappedInput, err := input.Unwrap()
	if err != nil {
		return nil, err
	}

	unwrappedMap, ok := unwrappedInput.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected unwrapped input to be a map")
	}

	userPayload, err := e.codec.Encode(ctx, unwrappedMap, "input.user.DecimalReports")
	if err != nil {
		return nil, err
	}

	// add metadata
	return userPayload, nil
}
