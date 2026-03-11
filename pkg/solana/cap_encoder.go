package solana

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	sol_binary "github.com/gagliardetto/binary"

	consensustypes "github.com/smartcontractkit/chainlink-common/pkg/capabilities/consensus/ocr3/types"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"

	"github.com/smartcontractkit/chainlink-protos/cre/go/values"

	solcommoncodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/common"
	codecv1 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v1"
)

var (
	encoderName     = "user"
	reportSchemaKey = "report_schema"
	definedTypes    = "defined_types"
	accountCtxHash  = "account_context_hash"
)

func NewEncoder(config *values.Map) (consensustypes.Encoder, error) {
	idl, err := getIDLFromConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse borsh encoder config: %w", err)
	}
	parsed := &solcommoncodec.ParsedTypes{
		EncoderDefs: make(map[string]solcommoncodec.Entry),
	}
	idlDef, err := codecv1.FindDefinitionFromIDL(solcommoncodec.ChainConfigTypeAccountDef, idl.Accounts[0].Name, idl)
	if err != nil {
		return nil, fmt.Errorf("failed to find definition: %w", err)
	}

	accountIDLDef, ok := idlDef.(codecv1.IdlTypeDef)
	if !ok {
		return nil, errors.New("invalid cast")
	}

	cEntry, err := codecv1.CreateCodecEntry(accountIDLDef, idl.Accounts[0].Name, idl, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create codec entry: %w", err)
	}
	itemType := solcommoncodec.WrapItemType(true, encoderName, idl.Accounts[0].Name)
	parsed.EncoderDefs[itemType] = cEntry

	c, err := parsed.ToCodec()
	if err != nil {
		return nil, fmt.Errorf("failed to create remote codec: %w", err)
	}

	return &capEncoder{codec: c, itemType: itemType}, err
}

func getIDLFromConfig(config *values.Map) (codecv1.IDL, error) {
	var idl codecv1.IDL
	inputSchema, ok := config.Underlying[reportSchemaKey]
	if !ok {
		return idl, errors.New("missing field report_schema")
	}
	var reportSchema string
	err := inputSchema.UnwrapTo(&reportSchema)
	if err != nil {
		return idl, fmt.Errorf("failed to unwrap report_schema: %w", err)
	}

	inputTypes, ok := config.Underlying[definedTypes]
	if !ok {
		return idl, errors.New("missing field defined_types")
	}
	var types string
	err = inputTypes.UnwrapTo(&types)
	if err != nil {
		return idl, fmt.Errorf("failed to unwrap defined types: %w", err)
	}

	idlJSON := fmt.Sprintf(`
 	{
  	"accounts": [
    	{
      	"name": "Reports",
      	"type": %s
    	}
  	],
  	"types": %s}`, reportSchema, types)

	err = json.Unmarshal([]byte(idlJSON), &idl)
	if err != nil {
		return idl, err
	}

	return idl, nil
}

type capEncoder struct {
	codec    commontypes.RemoteCodec
	itemType string
}

var (
	anchorDescrLength = 8
)

type ForwarderReport struct {
	Hash    [32]byte
	Payload []byte
}

func (e *capEncoder) Encode(ctx context.Context, input values.Map) ([]byte, error) {
	// 1. encode users payload
	unwrappedInput, err := input.Unwrap()
	if err != nil {
		return nil, err
	}

	unwrappedMap, ok := unwrappedInput.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected unwrapped input to be a map")
	}

	userPayload, err := e.codec.Encode(ctx, unwrappedMap, e.itemType)
	if err != nil {
		return nil, err
	}

	userPayload = userPayload[anchorDescrLength:]

	inAccCtx, ok := input.Underlying[accountCtxHash]
	if !ok {
		return nil, fmt.Errorf("missing expected field: %s", accountCtxHash)
	}
	// 2. encode forwarder report
	var hash [32]byte
	err = inAccCtx.UnwrapTo(&hash)
	if err != nil {
		return nil, fmt.Errorf("failed to unwrap account ctx hash: %w", err)
	}
	forwarderReport := ForwarderReport{hash, userPayload}

	encReport, err := sol_binary.MarshalBorsh(forwarderReport)
	if err != nil {
		return nil, fmt.Errorf("failed to encode forwarder report: %w", err)
	}

	// 3. encode metadata
	metaMap, ok := input.Underlying[consensustypes.MetadataFieldName]
	if !ok {
		return nil, fmt.Errorf("expected metadata field to be present: %s", consensustypes.MetadataFieldName)
	}

	var meta consensustypes.Metadata
	err = metaMap.UnwrapTo(&meta)
	if err != nil {
		return nil, err
	}

	encodedMeta, err := meta.Encode()
	if err != nil {
		return nil, err
	}

	return append(encodedMeta, encReport...), nil
}
