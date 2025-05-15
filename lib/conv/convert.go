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
)

func FormatSqlInt16(i sql.NullInt16) *string {
	if i.Valid {
		result := strconv.FormatInt(int64(i.Int16), 10)
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

func StringOrNil(v sql.NullString) *string {
	if v.Valid {
		return &v.String
	}
	return nil
}

func SQLNullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{
		String: s,
		Valid:  true,
	}
}

func Int32OrNil(v sql.NullInt32) *int32 {
	if v.Valid {
		return &v.Int32
	}
	return nil
}

func ParseInt16(s string) (int16, error) {
	i, err := strconv.ParseInt(s, 10, 16)
	if err != nil {
		return 0, err
	}
	return int16(i), nil
}

func ParseInt32(s string) (int32, error) {
	i, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0, err
	}
	return int32(i), nil
}

func FormatInt32(i int32) string {
	return strconv.FormatInt(int64(i), 10)
}

func ParseInt64(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

func MustInt32(i int64) int32 {
	if i < math.MinInt32 || i > math.MaxInt32 {
		panic("int32 overflow")
	}
	return int32(i)
}
