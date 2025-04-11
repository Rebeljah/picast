package sdp

import (
	"fmt"
	"reflect"

	"github.com/pion/sdp"
	"github.com/rebeljah/picast/util/strutil"
)

// Fills the struct fields based on the provided SDP attributes.
// The struct `v` should be a pointer to a struct, and each field should have an
// `sdp` tag corresponding to an attribute key from the `attributes` slice.
// This function uses reflection to match the attribute keys with struct field tags,
// and attempts to convert the attribute values to the appropriate types for each field.
// If a field is a pointer, it will be initialized before setting the value.
//
// Fields without an `sdp` key tag are skipped and left unmodified.
//
// Returns an error if a type conversion fails for a matched field.
func PopulateStructFromAttributes(v any, attributes []sdp.Attribute) error {
	// Get the struct type and value
	val := reflect.ValueOf(v).Elem()
	typ := reflect.TypeOf(v).Elem()

	// Iterate over the attributes to populate the struct fields
	for _, attr := range attributes {
		// Find the field in the struct that matches the SDP key
		fieldFound := false
		for i := range typ.NumField() {
			field := typ.Field(i)
			sdpTag := field.Tag.Get("sdp")

			// Skip fields without an sdp tag
			if sdpTag == "" {
				continue
			}

			// Match the SDP key with the struct field tag
			if sdpTag == attr.Key {
				// Set the struct field with the value from the SDP attribute
				fieldValue := val.Field(i)

				// Convert the attribute value to the correct type and set the struct field
				convertedValue, err := strutil.Stov(attr.Value, field.Type)
				if err != nil {
					return fmt.Errorf("error converting value for field `%s`: %w", field.Name, err)
				}

				// Set the value to the struct field (handle pointers as well)
				if fieldValue.Kind() == reflect.Ptr {
					fieldValue.Set(reflect.New(field.Type.Elem()))
					fieldValue = fieldValue.Elem()
				}
				fieldValue.Set(reflect.ValueOf(convertedValue))
				fieldFound = true
				break
			}
		}
		if !fieldFound {
			return fmt.Errorf("no struct field with an sdp tag matching the attribute key: %s", attr.Key)
		}
	}

	return nil
}

// Convenience function useful for attaching flat structs to
// a sdp.SessionDescription by converting the struct fields to
// sdp.Attribute(s).
//   - the function will look for any struct field
//     that includes a tag like `sdp:"key-name"` and create an
//     sdp.Attribute with that key name, attempting to string
//     format the value.
//   - If a struct field has an sdp tag, but the field value is
//     not Stringable, this function will return an error.
func NewAttributesFromStruct(v *any) ([]sdp.Attribute, error) {
	var attributes []sdp.Attribute

	anyValue := reflect.ValueOf(v).Elem()
	typ := reflect.TypeOf(v).Elem()

	for i := range typ.NumField() {
		sdpKey := typ.Field(i).Tag.Get("sdp")

		// No sdp key is defined
		if sdpKey == "" {
			continue
		}

		fieldValue := anyValue.Field(i)

		structValue, err := strutil.Vtos(fieldValue.Interface())
		if err != nil {
			return nil, fmt.Errorf("field `%s` cannot be made into string: %w", typ.Field(i).Name, err)
		}

		// Append the attribute
		attributes = append(attributes, sdp.NewAttribute(sdpKey, structValue))
	}

	return attributes, nil
}
