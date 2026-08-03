package commoncodec

import (
	"fmt"
	"reflect"
	"strconv"
)

// ExtractField extracts the value of a field or nested subfield from a composite datatype composed
// of a series of nested structs and maps. Pointers at any level are automatically dereferenced, as long
// as they aren't nil. path is an ordered list of nested subfield names to traverse. For now, slices and
// arrays are not supported. (If the need arises, we could support them by converting the field to an
// integer to extract a specific element from a slice or array.)
func ExtractField(data any, path []string) (any, error) {
	v := reflect.ValueOf(data)
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			if len(path) > 0 {
				return nil, fmt.Errorf("cannot extract field '%s' from a nil pointer", path[0])
			}
			return nil, nil // as long as this is the last field in the path, nil pointer is not a problem
		}
		v = v.Elem()
	}

	if len(path) == 0 {
		return v.Interface(), nil
	}
	field, path := path[0], path[1:]

	switch v.Kind() {
	case reflect.Struct:
		v = v.FieldByName(field)
		if !v.IsValid() {
			return nil, fmt.Errorf("field '%s' of struct %v does not exist", field, data)
		}
		return ExtractField(v.Interface(), path)
	case reflect.Map:
		var keyVal reflect.Value
		if keyType := v.Type().Key(); keyType.Kind() != reflect.String {
			// This map does not have string keys, so let's try int (or anything convertible to int)
			intKey, err := strconv.Atoi(field)
			if err != nil {
				return nil, fmt.Errorf("map key '%s' for non-string type '%T' is not convertable to an integer", field, v.Type())
			}
			if !keyType.ConvertibleTo(reflect.TypeOf(intKey)) {
				return nil, fmt.Errorf("map has type '%T', must be a string or convertable to an integer", v.Type())
			}
			keyVal = reflect.ValueOf(intKey)
		} else {
			keyVal = reflect.ValueOf(field)
		}
		v = v.MapIndex(keyVal)
		if !v.IsValid() {
			return nil, fmt.Errorf("key '%s' of map %v does not exist", field, data)
		}
		return ExtractField(v.Interface(), path)
	default:
		return nil, fmt.Errorf("extracting a field from a %s type is not supported", v.Kind().String())
	}
}
