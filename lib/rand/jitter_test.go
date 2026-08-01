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

package rand_test

import (
	"testing"
	"time"

	"github.com/burningmantech/ranger-ims-go/lib/rand"
	"github.com/stretchr/testify/require"
)

func TestJitterStaysInRange(t *testing.T) {
	t.Parallel()

	const d = 100 * time.Millisecond
	for range 1000 {
		got := rand.Jitter(d)
		require.GreaterOrEqual(t, got, d/2)
		require.Less(t, got, d)
	}
}

func TestJitterVaries(t *testing.T) {
	t.Parallel()

	// The point of jittering is that callers don't all wake at once, so the
	// same input must not keep producing the same delay.
	seen := make(map[time.Duration]bool)
	for range 100 {
		seen[rand.Jitter(100*time.Millisecond)] = true
	}
	require.Greater(t, len(seen), 1)
}

func TestJitterOfNonPositiveIsZero(t *testing.T) {
	t.Parallel()

	require.Zero(t, rand.Jitter(0))
	require.Zero(t, rand.Jitter(-time.Second))
}

// A one-nanosecond delay has no room between d/2 and d, which must not panic
// with a division by zero or a modulus of zero.
func TestJitterOfSmallestDurationIsSafe(t *testing.T) {
	t.Parallel()

	require.Zero(t, rand.Jitter(1))
}
