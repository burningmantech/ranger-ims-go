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

package conv

import (
	"database/sql"
	"math"
	"strconv"
	"time"
)

type IntLike interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

func FormatInt[T IntLike](i T) string {
	return strconv.FormatInt(int64(i), 10)
}

func FormatSqlInt16(i sql.NullInt16) *string {
	if i.Valid {
		result := FormatInt(i.Int16)
		return &result
	}
	return nil
}

func ParseSqlInt16(s *string) sql.NullInt16 {
	if s == nil {
		return sql.NullInt16{}
	}
	parsed, err := ParseInt16(*s)
	return sql.NullInt16{
		Int16: parsed,
		Valid: err == nil,
	}
}

func ParseInt16(s string) (int16, error) {
	i, err := strconv.ParseInt(s, 10, 16)
	if err != nil {
		return 0, err
	}
	return int16(i), nil
}

func SqlToInt32(v sql.NullInt32) *int32 {
	if v.Valid {
		return &v.Int32
	}
	return nil
}

func ParseInt32(s string) (int32, error) {
	i, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0, err
	}
	return int32(i), nil
}

func ParseInt64(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

// MustInt32 converts an int64 into an int32, and it panics if this would cause
// an overflow. This is intended for use when the input is known to be within
// bounds, because panics are bad.
func MustInt32(i int64) int32 {
	if i < math.MinInt32 || i > math.MaxInt32 {
		panic("int32 overflow")
	}
	return int32(i)
}

func SqlToString(v sql.NullString) *string {
	if v.Valid {
		return &v.String
	}
	return nil
}

// StringToSql converts a string pointer into a sql.NullString.
//
// The string will be truncated at maxLength, if maxLength > 0. This uses the fact
// that Go and IMS's MariaDB tables encode strings in UTF-8.
func StringToSql(s *string, maxLength int) sql.NullString {
	if s == nil || *s == "" {
		return sql.NullString{}
	}
	val := *s
	if maxLength > 0 && len(val) > maxLength {
		val = val[:maxLength]
	}
	return sql.NullString{String: val, Valid: true}
}

// FloatToTime converts the float number of seconds since Unix epoch into a time.Time.
func FloatToTime(f float64) time.Time {
	return time.Unix(int64(f), int64(f*1e9)%1e9)
}

// TimeToFloat converts a time.Time into the float number of seconds since Unix epoch.
func TimeToFloat(t time.Time) float64 {
	decimalPart := float64(t.Nanosecond()) / 1e9
	return decimalPart + float64(t.Unix())
}

func NullFloatToTime(f sql.NullFloat64) time.Time {
	if !f.Valid {
		return time.Time{}
	}
	return FloatToTime(f.Float64)
}

func NullFloatToTimePtr(f sql.NullFloat64) *time.Time {
	if !f.Valid {
		return nil
	}
	res := FloatToTime(f.Float64)
	return &res
}

func TimeToNullFloat(t time.Time) sql.NullFloat64 {
	if t.IsZero() {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{
		Valid:   true,
		Float64: TimeToFloat(t),
	}
}

func EmptyToNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
