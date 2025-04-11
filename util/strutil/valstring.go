package strutil

import (
	"fmt"
	"reflect"
	"strconv"
)

// Vtos converts any numeric or common type value to a string.
func Vtos(value any) (string, error) {
	v := reflect.ValueOf(value)

	// Handle nil case
	if !v.IsValid() {
		return "", fmt.Errorf("invalid value")
	}

	// Handle pointer dereferencing
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	// Handle different types
	switch v.Kind() {
	case reflect.String:
		return v.String(), nil

	case reflect.Float32, reflect.Float64:
		return fmt.Sprintf("%f", v.Float()), nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fmt.Sprintf("%d", v.Int()), nil

	case reflect.Bool:
		return fmt.Sprintf("%v", v.Bool()), nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return fmt.Sprintf("%d", v.Uint()), nil

	default:
		// For unsupported kinds, return an error
		return "", fmt.Errorf("unsupported kind %s", v.Kind())
	}
}

// Stov converts a string to a specific type (int, float, string, etc.)
func Stov(value string, typ reflect.Type) (any, error) {
	// Handle the string conversion
	if typ.Kind() == reflect.String {
		return value, nil
	}

	// Handle the numeric types
	switch typ.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.ParseInt(value, 10, 64)
	case reflect.Float32, reflect.Float64:
		return strconv.ParseFloat(value, 64)
	case reflect.Bool:
		return strconv.ParseBool(value)
	}

	// Unsupported types
	return nil, fmt.Errorf("unsupported type %s", typ.Kind())
}
