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

import "hash/fnv"

// NonCryptoHash64 returns an int64 hash of the provided string. It's fast rather than secure, so don't use
// this for cryptographic purposes.
func NonCryptoHash64(s string) int64 {
	hasher := fnv.New64()
	// this never returns an error
	_, _ = hasher.Write([]byte(s))
	// allow twos-complement wraparound, because we just want any number
	return int64(hasher.Sum64())
}
