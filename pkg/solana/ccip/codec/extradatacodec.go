package codec

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"reflect"

	agbinary "github.com/gagliardetto/binary"

	"github.com/smartcontractkit/chainlink-common/pkg/types/ccipocr3"

	"github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/latest/fee_quoter"
)

const (
	svmDestExecDataKey = "destGasAmount"
	evmGasLimitKey     = "GasLimit"
)

var (
	// tag definition https://github.com/smartcontractkit/chainlink-ccip/blob/1b2ee24da54bddef8f3943dc84102686f2890f87/chains/solana/contracts/programs/ccip-router/src/extra_args.rs#L8C21-L11C45
	// this should be moved to msghasher.go once merged

	// bytes4(keccak256("CCIP SVMExtraArgsV1"));
	// Omit 0x prefix for hex decoder
	svmExtraArgsV1Tag = "1f3b3aba"

	// bytes4(keccak256("CCIP EVMExtraArgsV2"));
	// Omit 0x prefix for hex decoder
	evmExtraArgsV2Tag = "181dcf10"
)

// TODO: generate mocks in chainlink-common for interface
// Temporarily copied from chainlink-common so mocks can be generated locally
// https://github.com/smartcontractkit/chainlink-common/blob/04a897dcb3618fbfaa9d7309eee606597af5cd76/pkg/types/ccipocr3/plugincodec.go#L32
type SourceChainExtraDataCodec interface {
	// DecodeExtraArgsToMap reformat bytes into a chain agnostic map[string]any representation for extra args
	DecodeExtraArgsToMap(extraArgs ccipocr3.Bytes) (map[string]any, error)
	// DecodeDestExecDataToMap reformat bytes into a chain agnostic map[string]interface{} representation for dest exec data
	DecodeDestExecDataToMap(destExecData ccipocr3.Bytes) (map[string]any, error)
}

// ExtraDataDecoder is a helper struct for decoding extra data
type ExtraDataDecoder struct{}

func NewExtraDataDecoder() ExtraDataDecoder {
	return ExtraDataDecoder{}
}

// DecodeExtraArgsToMap is a helper function for converting Borsh encoded extra args bytes into map[string]any
func (d ExtraDataDecoder) DecodeExtraArgsToMap(extraArgs ccipocr3.Bytes) (map[string]any, error) {
	if len(extraArgs) < 4 {
		return nil, fmt.Errorf("extra args too short: %d, should be at least 4 (i.e the extraArgs tag)", len(extraArgs))
	}

	var decodedEvmTag, decodedSvmTag []byte
	var err error
	if decodedEvmTag, err = hex.DecodeString(evmExtraArgsV2Tag); err != nil {
		return nil, fmt.Errorf("failed to decode evm extra args tag %s: %w", evmExtraArgsV2Tag, err)
	}
	if decodedSvmTag, err = hex.DecodeString(svmExtraArgsV1Tag); err != nil {
		return nil, fmt.Errorf("failed to decode evm extra args tag %s: %w", evmExtraArgsV2Tag, err)
	}

	var val reflect.Value
	var typ reflect.Type
	outputMap := make(map[string]any)
	switch string(extraArgs[:4]) {
	case string(decodedEvmTag):
		var args fee_quoter.GenericExtraArgsV2
		decoder := agbinary.NewBorshDecoder(extraArgs[4:])
		err := args.UnmarshalWithDecoder(decoder)
		if err != nil {
			return nil, fmt.Errorf("failed to decode extra args: %w", err)
		}
		val = reflect.ValueOf(args)
		typ = reflect.TypeOf(args)
	case string(decodedSvmTag):
		var args fee_quoter.SVMExtraArgsV1
		decoder := agbinary.NewBorshDecoder(extraArgs[4:])
		err := args.UnmarshalWithDecoder(decoder)
		if err != nil {
			return nil, fmt.Errorf("failed to decode extra args: %w", err)
		}
		val = reflect.ValueOf(args)
		typ = reflect.TypeOf(args)
	default:
		return nil, fmt.Errorf("unknown extra args tag: %x", extraArgs[:4])
	}

	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		fieldValue := val.Field(i).Interface()
		if field.Name == evmGasLimitKey {
			// convert SVM Borsh specific type uint128 to *big.Int for EVM gas limit
			gl, ok := fieldValue.(agbinary.Uint128)
			if !ok {
				return nil, fmt.Errorf("expected field %s to be of type agbinary.Uint128, got %T", field.Name, fieldValue)
			}

			fieldValue = gl.BigInt()
		}
		outputMap[field.Name] = fieldValue
	}

	return outputMap, nil
}

// DecodeDestExecDataToMap is a helper function for converting dest exec data bytes into map[string]any
func (d ExtraDataDecoder) DecodeDestExecDataToMap(destExecData ccipocr3.Bytes) (map[string]any, error) {
	return map[string]any{
		svmDestExecDataKey: binary.BigEndian.Uint32(destExecData),
	}, nil
}

// Ensure ExtraDataDecoder implements the SourceChainExtraDataCodec interface
var _ ccipocr3.SourceChainExtraDataCodec = &ExtraDataDecoder{}
