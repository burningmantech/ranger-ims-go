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

package authn

import (
	"crypto/rand"
	"crypto/sha1" //nolint: gosec // this is the algorithm Clubhouse uses
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"sync"

	"github.com/burningmantech/ranger-ims-go/lib/argon2id"
)

const (
	saltPasswordSep = ":"
)

// argonLocker is used to disallow concurrent calls into the Argon2id hash algorithm.
// Our standard Clubhouse parameters for Argon2id require the algorithm to use 8 MiB
// of memory. If too many logins are attempted at once, it's very easy for the Go
// program's memory use to go above what's allowed by our AWS ECS container, and then
// the server gets killed.
var argonLocker sync.Mutex

func Verify(password, storedValue string) (isValid bool, err error) {
	if strings.HasPrefix(storedValue, "$argon2id") {
		argonLocker.Lock()
		defer argonLocker.Unlock()
		return argon2id.ComparePasswordAndHash(password, storedValue)
	}

	salt, storedHash, found := strings.Cut(storedValue, saltPasswordSep)
	if !found {
		return false, errors.New("invalid hashed password")
	}
	correct := subtle.ConstantTimeCompare([]byte(Hash(password, salt)), []byte(storedHash)) == 1
	return correct, nil
}

func Hash(password, salt string) string {
	hasher := sha1.New() //nolint: gosec // this is the algorithm Clubhouse uses
	hasher.Write([]byte(salt + password))
	return hex.EncodeToString(hasher.Sum(nil))
}

func NewSaltedSha1(password string) string {
	salt := newSalt()
	return salt + ":" + Hash(password, salt)
}

func NewSaltedArgon2idDevOnly(password string) string {
	// do not use DevelopmentParams for production use!
	return argon2id.CreateHash(password, argon2id.DevelopmentParams)
}

// newSalt returns a 22-byte-long base64-encoded string with 128 bits of randomness.
func newSalt() string {
	var s [16]byte
	_, _ = rand.Read(s[:])
	return base64.RawStdEncoding.EncodeToString(s[:])
}
