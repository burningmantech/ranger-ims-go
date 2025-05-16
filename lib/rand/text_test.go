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
	mathrand "math/rand/v2"
	"testing"
)

func TestNonCryptoText(t *testing.T) {
	t.Parallel()
	someVal := NonCryptoText()
	// for an unknown seed, all we can say is that this is a 26-byte string
	assert.Len(t, someVal, 26)

	// with a known seed, we get consistent values
	dummySeed := []byte("this is thirty two bytes of nada")
	chacha = mathrand.NewChaCha8(([32]byte)(dummySeed))
	assert.Equal(t, "GYEZQNJ3T4KGVFD6SHQDDXOF6P", NonCryptoText())
	assert.Equal(t, "4557VJCAULCJTN4NA3PKDWRI7H", NonCryptoText())
	assert.Equal(t, "W5PPJS5D6SP4YTN7NVYEIBDYNG", NonCryptoText())
}
