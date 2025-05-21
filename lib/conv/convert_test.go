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
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestFloat64UnixSeconds(t *testing.T) {
	t.Parallel()
	// epochSec is when I wrote this test
	const (
		epochSec    int64 = 1747851186
		nanoseconds int64 = 123456789
	)
	nanoPreciseTime := float64(epochSec) + float64(nanoseconds)/1e9
	// convert to a time.Time
	tim := Float64UnixSeconds(nanoPreciseTime)
	// that time can't actually hold the full precision. It's 21 ns off, not that
	// the value itself actually matters
	epsilon := int(nanoseconds) - tim.Nanosecond()
	assert.NotZero(t, epsilon)
	assert.Less(t, epsilon, 100)

	// but, this is good enough for microsecond precision!
	assert.Equal(t, epochSec*1e6+nanoseconds/1e3, tim.UnixMicro())

	backToFloat := TimeFloat64(tim)
	assert.Less(t, nanoPreciseTime-backToFloat, 1e-7)
}
