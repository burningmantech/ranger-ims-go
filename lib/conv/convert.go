package conv

import (
	"database/sql"
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

func ParseInt64(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}
