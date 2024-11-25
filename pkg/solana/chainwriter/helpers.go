package chainwriter

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/gagliardetto/solana-go"
)

// GetValuesAtLocation parses through nested types and arrays to find all locations of values
func GetValuesAtLocation(args any, location string, debugID string) ([][]byte, error) {
	var vals [][]byte

	path := strings.Split(location, ".")

	addressList, err := traversePath(args, path)
	if err != nil {
		return nil, err
	}

	for _, value := range addressList {
		if byteArray, ok := value.([]byte); ok {
			vals = append(vals, byteArray)
		} else if address, ok := value.(solana.PublicKey); ok {
			vals = append(vals, address.Bytes())
		} else {
			return nil, errorWithDebugID(fmt.Errorf("invalid value format at path: %s", location), debugID)
		}
	}

	return vals, nil
}

func GetDebugIDAtLocation(args any, location string) (string, error) {
	debugIDList, err := GetValueAtLocation(args, location)
	if err != nil {
		return "", err
	}

	// there should only be one debug ID, others will be ignored.
	debugID := string(debugIDList[0])

	return debugID, nil
}

func GetValueAtLocation(args any, location string) ([][]byte, error) {
	path := strings.Split(location, ".")

	valueList, err := traversePath(args, path)
	if err != nil {
		return nil, err
	}

	var values [][]byte
	for _, value := range valueList {
		if byteArray, ok := value.([]byte); ok {
			values = append(values, byteArray)
		} else {
			return nil, fmt.Errorf("invalid value format at path: %s", location)
		}
	}

	return values, nil
}

func errorWithDebugID(err error, debugID string) error {
	if debugID == "" {
		return err
	}
	return fmt.Errorf("Debug ID: %s: Error: %s", debugID, err)
}

// traversePath recursively traverses the given structure based on the provided path.
func traversePath(data any, path []string) ([]any, error) {
	if len(path) == 0 {
		return []any{data}, nil
	}

	var result []any

	val := reflect.ValueOf(data)

	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	fmt.Printf("Current path: %v, Current value type: %v\n", path, val.Kind())

	switch val.Kind() {
	case reflect.Struct:
		field := val.FieldByName(path[0])
		if !field.IsValid() {
			return nil, errors.New("field not found: " + path[0])
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
		return nil, errors.New("no matching field found in array")

	case reflect.Map:
		key := reflect.ValueOf(path[0])
		value := val.MapIndex(key)
		if !value.IsValid() {
			return nil, errors.New("key not found: " + path[0])
		}
		return traversePath(value.Interface(), path[1:])
	default:
		if len(path) == 1 && val.Kind() == reflect.Slice && val.Type().Elem().Kind() == reflect.Uint8 {
			return []any{val.Interface()}, nil
		}
		return nil, errors.New("unexpected type encountered at path: " + path[0])
	}
}
