package chainwriter

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/gagliardetto/solana-go"
)

// GetAddressesFromDecodedData parses through nested types and arrays to find all address locations.
func GetAddressesFromDecodedData(decoded any, addressLocations []string) ([]*solana.AccountMeta, error) {
	var addresses []*solana.AccountMeta

	for _, location := range addressLocations {
		path := strings.Split(location, ".")

		addressList, err := traversePath(decoded, path)
		if err != nil {
			return nil, err
		}

		for _, value := range addressList {
			if byteArray, ok := value.([]byte); ok {
				// TODO: How to handle IsSigner and IsWritable?
				accountMeta := &solana.AccountMeta{
					PublicKey:  solana.PublicKeyFromBytes(byteArray),
					IsSigner:   false,
					IsWritable: true,
				}
				addresses = append(addresses, accountMeta)
			} else {
				return nil, fmt.Errorf("invalid address format at path: %s", location)
			}
		}
	}

	return addresses, nil
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

	default:
		if len(path) == 1 && val.Kind() == reflect.Slice && val.Type().Elem().Kind() == reflect.Uint8 {
			return []any{val.Interface()}, nil
		}
		return nil, errors.New("unexpected type encountered at path: " + path[0])
	}
}
