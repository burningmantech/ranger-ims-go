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
	"github.com/stretchr/testify/assert"
	"math"
	"testing"
)

func TestFloatTimeConversions(t *testing.T) {
	t.Parallel()
	// epochSec is when I wrote this test
	const (
		epochSec    int64 = 1747851186
		nanoseconds int64 = 123456789
	)
	nanoPreciseTime := float64(epochSec) + float64(nanoseconds)/1e9
	// convert to a time.Time
	tim := FloatToTime(nanoPreciseTime)
	// that time can't actually hold the full precision. It's 21 ns off, not that
	// the value itself actually matters
	epsilon := int(nanoseconds) - tim.Nanosecond()
	assert.NotZero(t, epsilon)
	assert.Less(t, epsilon, 100)

	// but, this is good enough for microsecond precision!
	assert.Equal(t, epochSec*1e6+nanoseconds/1e3, tim.UnixMicro())

	// convert back to float
	backToFloat := TimeToFloat(tim)
	assert.Less(t, nanoPreciseTime-backToFloat, 1e-7)
}

func TestFormatInt(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "123", FormatInt(123))
}

func TestFormatSqlInt16(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "42", *FormatSqlInt16(sql.NullInt16{Valid: true, Int16: 42}))
	assert.Nil(t, FormatSqlInt16(sql.NullInt16{}))
}

func TestParseSqlInt16(t *testing.T) {
	t.Parallel()

	s123 := "123"
	assert.Equal(t, sql.NullInt16{Valid: true, Int16: 123}, ParseSqlInt16(&s123))
	assert.Equal(t, sql.NullInt16{}, ParseSqlInt16(nil))
}

func TestMustInt(t *testing.T) {
	t.Parallel()

	assert.Equal(t, int32(math.MaxInt32), MustInt32(math.MaxInt32))
	assert.Panics(t, func() {
		MustInt32(math.MaxInt32 + 1)
	})
}
