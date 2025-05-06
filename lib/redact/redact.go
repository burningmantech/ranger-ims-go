package redact

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"strings"
)

func ToBytes(pointerToStruct any) ([]byte, error) {
	output := &bytes.Buffer{}
	err := toWriter(output, reflect.ValueOf(pointerToStruct).Elem(), "")
	if err != nil {
		return nil, fmt.Errorf("[toWriter]: %w", err)
	}
	return output.Bytes(), nil
}

const nestIndent = "    "

func toWriter(w io.Writer, v reflect.Value, indent string) error {
	s := v
	typeOfT := s.Type()
	for i := range s.NumField() {
		f := s.Field(i)

		redact := strings.EqualFold(typeOfT.Field(i).Tag.Get("redact"), "true")

		switch f.Kind() {
		case reflect.Struct:
			err := writeStructField(w, typeOfT.Field(i).Name, f, redact, indent)
			if err != nil {
				return err
			}
		case reflect.Slice:
			err := writeSliceFields(w, typeOfT.Field(i).Name, f, redact, indent)
			if err != nil {
				return err
			}
		case reflect.String, reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32,
			reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Uintptr, reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
			// For simple types, we can just print them on one line
			printVal := "🤐🤐🤐"
			if !redact {
				printVal = fmt.Sprint(f.Interface())
			}
			_, err := fmt.Fprintf(w, "%v%v = %v\n", indent, typeOfT.Field(i).Name, printVal)
			if err != nil {
				return err
			}
		case reflect.Invalid, reflect.Array, reflect.Chan, reflect.Func, reflect.Interface, reflect.Map,
			reflect.Pointer, reflect.UnsafePointer:
			fallthrough
		default:
			return fmt.Errorf("unsupported field kind: %v", f.Kind().String())
		}
	}
	return nil
}

func writeSliceFields(w io.Writer, fieldName string, fieldVal reflect.Value, redact bool, indent string) error {
	x1 := reflect.ValueOf(fieldVal.Interface())
	sliceElemType := fieldVal.Type().Elem()

	// If it's a slice of structs, we'll need to descend into each of those structs
	switch sliceElemType.Kind() {
	case reflect.Struct:
		for j := range x1.Len() {
			_, err := fmt.Fprintf(w, "%v%v[%d]\n", indent, fieldName, j)
			if err != nil {
				return err
			}
			if redact {
				_, err = fmt.Fprintf(w, "%v🤐🤐\n", indent+nestIndent)
			} else {
				err = toWriter(w, x1.Index(j), indent+nestIndent)
			}
			if err != nil {
				return err
			}
		}
	default:
		printVal := "[🤐🤐🤐🤐]"
		if !redact {
			printVal = fmt.Sprint(fieldVal.Interface())
		}
		_, err := fmt.Fprintf(w, "%v%v = %v\n", indent, fieldName, printVal)
		if err != nil {
			return err
		}
	}
	return nil
}

func writeStructField(w io.Writer, fieldName string, fieldVal reflect.Value, redact bool, indent string) error {
	x1 := reflect.ValueOf(fieldVal.Interface())
	_, err := fmt.Fprintf(w, "%v%v\n", indent, fieldName)
	if err != nil {
		return err
	}
	// If this field is redacted, we just print that out.
	// If it's not redacted, we do a recursive call to print the field's own fields.
	if redact {
		_, err = fmt.Fprintf(w, "%v🤐🤐🤐🤐🤐\n", indent+nestIndent)
	} else {
		err = toWriter(w, x1, indent+nestIndent)
	}
	return err
}
