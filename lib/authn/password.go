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
	"errors"
	"strings"
	"sync"

	"github.com/burningmantech/ranger-ims-go/lib/argon2id"
)

// argonLocker is used to disallow concurrent calls into the Argon2id hash algorithm.
// Our standard Clubhouse parameters for Argon2id require the algorithm to use 8 MiB
// of memory. If too many logins are attempted at once, it's very easy for the Go
// program's memory use to go above what's allowed by our AWS ECS container, and then
// the server gets killed.
var argonLocker sync.Mutex

func Verify(password, storedValue string) (isValid bool, err error) {
	if !strings.HasPrefix(storedValue, "$argon2id") {
		return false, errors.New("unsupported non-argon2id stored password")
	}
	argonLocker.Lock()
	defer argonLocker.Unlock()
	return argon2id.ComparePasswordAndHash(password, storedValue)
}

func NewSaltedArgon2idDevOnly(password string) string {
	// do not use DevelopmentParams for production use!
	return argon2id.CreateHash(password, argon2id.DevelopmentParams)
}
