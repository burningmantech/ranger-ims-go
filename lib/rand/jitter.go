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
	"encoding/binary"
	"time"
)

// Jitter returns a random duration in [d/2, d). It's for spreading out retries
// that would otherwise all fire at the same moment and collide again, so it
// uses the same fast non-cryptographic source as NonCryptoText.
//
// A non-positive d returns 0.
func Jitter(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	var buf [8]byte
	// This never returns an error.
	_, _ = cryptorand.Read(buf[:])
	// #nosec G115 // masked to 62 bits below, so this can't go negative
	n := int64(binary.LittleEndian.Uint64(buf[:]) >> 2)
	half := int64(d) / 2
	return time.Duration(half + n%(int64(d)-half))
}
