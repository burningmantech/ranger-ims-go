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

package rand

import (
	"github.com/stretchr/testify/assert"
	"strings"
	"testing"
)

func TestNonCryptoHash64(t *testing.T) {
	t.Parallel()
	// want a stable value for a given input
	assert.Equal(t, int64(1992615242312982639), NonCryptoHash64("abcdefg"))
	// negative values are possible too
	assert.Equal(t, int64(-2820157060406071861), NonCryptoHash64("abc"))
}

func FuzzNonCryptoHash64(f *testing.F) {
	testcases := []string{"", "dog", "🔥", strings.Repeat("some text,", 1000)}
	for _, testcase := range testcases {
		f.Add(testcase)
	}
	hashesSoFar := make(map[int64]bool)
	f.Fuzz(func(t *testing.T, input string) {
		hashed := NonCryptoHash64(input)
		// it's vanishingly unlikely we'd get a bug-free zero
		assert.NotZero(t, hashed)
		hashesSoFar[hashed] = true
	})
	// We should definitely expect no collisions
	assert.Len(f, hashesSoFar, len(testcases))
}
