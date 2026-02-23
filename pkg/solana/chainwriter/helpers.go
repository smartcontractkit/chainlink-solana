package chainwriter

import (
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-common/pkg/types/ccipocr3"
)

type TestArgs struct {
	Inner []InnerArgs
}

type InnerArgs struct {
	Address       []byte
	SecondAddress []byte
}

type DataAccount struct {
	Version              uint8
	Administrator        solana.PublicKey
	PendingAdministrator solana.PublicKey
	LookupTable          solana.PublicKey
}

//go:embed testContractIDL.json
var testContractIDL string

// FetchTestContractIDL returns the IDL for chain components test contract
func FetchTestContractIDL() string {
	return testContractIDL
}

//go:embed testBufferContractIDL.json
var testBufferContractIDL string

// FetchTestContractIDL returns the IDL for chain components test contract
func FetchTestBufferContractIDL() string {
	return testBufferContractIDL
}

var (
	errFieldNotFound = errors.New("key not found")
)

// GetValuesAtLocation parses through nested types and arrays to find all locations of values
func GetValuesAtLocation(args any, location string) ([][]byte, error) {
	var vals [][]byte
	// If the user specified no location, just return empty (no-op).
	if location == "" {
		return nil, nil
	}

	path := strings.Split(location, ".")

	items, err := traversePath(args, path)
	if err != nil {
		return nil, err
	}

	for _, item := range items {
		rv := reflect.ValueOf(item)
		if rv.Kind() == reflect.Ptr && !rv.IsNil() {
			item = rv.Elem().Interface()
		}

		switch value := item.(type) {
		case []byte:
			vals = append(vals, value)
		case solana.PublicKey:
			vals = append(vals, value.Bytes())
		case ccipocr3.UnknownAddress:
			vals = append(vals, value)
		case ccipocr3.UnknownEncodedAddress:
			decoded, err := solana.PublicKeyFromBase58(string(value))
			if err != nil {
				return nil, err
			}
			vals = append(vals, decoded[:])
		case uint64:
			buf := make([]byte, 8)
			binary.LittleEndian.PutUint64(buf, value)
			vals = append(vals, buf)
		case uint8:
			buf := make([]byte, 8)
			binary.LittleEndian.PutUint64(buf, uint64(value)) // convert up and encode
			vals = append(vals, buf)
		case ccipocr3.ChainSelector:
			buf := make([]byte, 8)
			binary.LittleEndian.PutUint64(buf, uint64(value))
			vals = append(vals, buf)
		case ccipocr3.Bytes32:
			vals = append(vals, value[:])
		case [32]uint8:
			vals = append(vals, value[:])
		default:
			return nil, fmt.Errorf("invalid value format at path: %s, type: %s", location, reflect.TypeOf(value).String())
		}
	}
	return vals, nil
}

func GetDebugIDAtLocation(args any, location string) (string, error) {
	debugIDList, err := GetValuesAtLocation(args, location)
	if err != nil {
		return "", err
	}

	if len(debugIDList) == 0 {
		return "", errors.New("no debug ID found at location: " + location)
	}
	// there should only be one debug ID, others will be ignored.
	debugID := ccipocr3.Bytes32(debugIDList[0])

	return debugID.String(), nil
}

func errorWithDebugID(err error, debugID string) error {
	if debugID == "" {
		return err
	}
	return fmt.Errorf("Debug ID: %s: Error: %w", debugID, err)
}

// traversePath recursively traverses the given structure based on the provided path.
func traversePath(data any, path []string) ([]any, error) {
	if len(path) == 0 {
		val := reflect.ValueOf(data)

		// If the final data is a slice or array, flatten it into multiple items,
		if val.Kind() == reflect.Slice || val.Kind() == reflect.Array {
			// don't flatten []byte
			if val.Type().Elem().Kind() == reflect.Uint8 {
				return []any{val.Interface()}, nil
			}

			var results []any
			for i := 0; i < val.Len(); i++ {
				results = append(results, val.Index(i).Interface())
			}
			return results, nil
		}
		// Otherwise, return just this one item
		return []any{data}, nil
	}

	var result []any

	val := reflect.ValueOf(data)

	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	switch val.Kind() {
	case reflect.Struct:
		field := val.FieldByName(path[0])
		if !field.IsValid() {
			return []any{}, errFieldNotFound
		}
		return traversePath(field.Interface(), path[1:])

	case reflect.Slice, reflect.Array:
		for i := 0; i < val.Len(); i++ {
			element := val.Index(i).Interface()
			elements, err := traversePath(element, path)
			if err == nil {
				result = append(result, elements...)
			}
		}
		if len(result) > 0 {
			return result, nil
		}
		return []any{}, errFieldNotFound

	case reflect.Map:
		key := reflect.ValueOf(path[0])
		value := val.MapIndex(key)
		if !value.IsValid() {
			return []any{}, errFieldNotFound
		}
		return traversePath(value.Interface(), path[1:])
	default:
		return nil, errors.New("unexpected type encountered at path: " + path[0])
	}
}
