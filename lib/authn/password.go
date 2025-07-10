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
	"context"
	"crypto/rand"
	"crypto/sha1" //nolint: gosec // this is the algorithm Clubhouse uses
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/burningmantech/ranger-ims-go/lib/argon2id"
	"golang.org/x/sync/semaphore"
	"strings"
)

const (
	saltPasswordSep        = ":"
	maxArgon2idConcurrency = 3
)

// argonSemaphore limits the number of goroutines that can concurrently call into
// the argon2id code. Our standard Clubhouse parameters for Argon2id require the
// algorithm to use 64 MiB of memory. If too many logins are attempted at once,
// it's very easy for the Go program's memory use to go above what's allowed by
// our AWS ECS container, and then the server gets killed.
var argonSemaphore = semaphore.NewWeighted(maxArgon2idConcurrency)

func Verify(ctx context.Context, password, storedValue string) (isValid bool, err error) {
	if strings.HasPrefix(storedValue, "$argon2id") {
		err := argonSemaphore.Acquire(ctx, 1)
		if err != nil {
			return false, fmt.Errorf("[Acquire]: %w", err)
		}
		defer argonSemaphore.Release(1)
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
