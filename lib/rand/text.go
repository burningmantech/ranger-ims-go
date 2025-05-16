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
	cryptorand "crypto/rand"
	mathrand "math/rand/v2"
	"sync"
)

const base32alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"

var (
	chacha *mathrand.ChaCha8
	locker sync.Mutex
)

func init() {
	var seed [32]byte
	_, _ = cryptorand.Reader.Read(seed[:])
	chacha = mathrand.NewChaCha8(seed)
}

// NonCryptoText generates a random string comprised of 26 bytes (128 bits). It is like the standard library's
// rand.Text, but it's fast rather than cryptographically secure. Indeed, it is faster than uuid.NewString,
// and it generates nicer, shorter strings, so it's a good substitute for UUIDs when IDs are needed for testing.
func NonCryptoText() string {
	locker.Lock()
	defer locker.Unlock()
	src := make([]byte, 26)
	// This never returns an error
	_, _ = chacha.Read(src)
	for i := range src {
		src[i] = base32alphabet[src[i]%32]
	}
	return string(src)
}
