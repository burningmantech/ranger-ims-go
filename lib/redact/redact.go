//
// See the file COPYRIGHT for copyright information.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package redact

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"strings"
)

func ToBytes(pointerToStruct any) []byte {
	output := &bytesBuffer{}
	toBuffer(output, reflect.ValueOf(pointerToStruct).Elem(), "")
	return output.Bytes()
}

const nestIndent = "    "

func toBuffer(w *bytesBuffer, v reflect.Value, indent string) {
	s := v
	typeOfT := s.Type()
	for i := range s.NumField() {
		f := s.Field(i)

		redact := strings.EqualFold(typeOfT.Field(i).Tag.Get("redact"), "true")

		switch f.Kind() {
		case reflect.Struct:
			writeStructField(w, typeOfT.Field(i).Name, f, redact, indent)
		case reflect.Slice:
			writeSliceFields(w, typeOfT.Field(i).Name, f, redact, indent)
		case reflect.String, reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32,
			reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Uintptr, reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128, reflect.Pointer:
			// For simple types, we can just print them on one line
			printVal := "🤐🤐🤐"
			if !redact {
				if root, _ := reflect.TypeAssert[*os.Root](f); root != nil {
					printVal = root.Name()
				} else {
					printVal = fmt.Sprint(f.Interface())
				}
			}
			w.fprintf("%v%v = %v\n", indent, typeOfT.Field(i).Name, printVal)
		case reflect.Invalid, reflect.Array, reflect.Chan, reflect.Func, reflect.Interface, reflect.Map,
			reflect.UnsafePointer:
			fallthrough
		default:
			w.fprintf("%v%v [Unsupported field kind (%v)]\n", indent, typeOfT.Field(i).Name, f.Kind())
		}
	}
}

func writeSliceFields(w *bytesBuffer, fieldName string, fieldVal reflect.Value, redact bool, indent string) {
	x1 := reflect.ValueOf(fieldVal.Interface())
	sliceElemType := fieldVal.Type().Elem()

	// If it's a slice of structs, we'll need to descend into each of those structs
	switch sliceElemType.Kind() {
	case reflect.Struct:
		if x1.Len() == 0 {
			w.fprintf("%v%v[]: [empty]\n", indent, fieldName)
		}
		for j := range x1.Len() {
			w.fprintf("%v%v[%d]\n", indent, fieldName, j)
			if redact {
				w.fprintf("%v🤐🤐\n", indent+nestIndent)
			} else {
				toBuffer(w, x1.Index(j), indent+nestIndent)
			}
		}
	default:
		printVal := "[🤐🤐🤐🤐]"
		if !redact {
			printVal = fmt.Sprint(fieldVal.Interface())
		}
		w.fprintf("%v%v = %v\n", indent, fieldName, printVal)
	}
}

func writeStructField(w *bytesBuffer, fieldName string, fieldVal reflect.Value, redact bool, indent string) {
	// If this field is redacted, we just print that out.
	// If it's a nonredacted zero-valued struct, we print out that fact.
	// Otherwise, we do a recursive call to print the field's own fields.
	if redact {
		w.fprintf("%v%v\n", indent, fieldName)
		w.fprintf("%v🤐🤐🤐🤐🤐\n", indent+nestIndent)
		return
	}
	if fieldVal.IsZero() {
		w.fprintf("%v%v is zero value\n", indent, fieldName)
		return
	}
	w.fprintf("%v%v\n", indent, fieldName)
	x1 := reflect.ValueOf(fieldVal.Interface())
	toBuffer(w, x1, indent+nestIndent)
}

type bytesBuffer struct {
	bytes.Buffer
}

func (b *bytesBuffer) fprintf(format string, a ...any) (n int) {
	n, err := fmt.Fprintf(&b.Buffer, format, a...)
	if err != nil {
		// See https://pkg.go.dev/bytes#Buffer.Write
		panic("this cannot happen, because bytes.Buffer can't return an error on write")
	}
	return n
}
