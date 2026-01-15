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

package authn_test

import (
	"testing"

	"github.com/burningmantech/ranger-ims-go/lib/authn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerify_success(t *testing.T) {
	t.Parallel()
	hash := authn.NewSaltedArgon2idDevOnly("my password 123")

	isValid, err := authn.Verify("my password wrong", hash)
	require.NoError(t, err)
	assert.False(t, isValid)

	isValid, err = authn.Verify("my password 123", hash)
	require.NoError(t, err)
	assert.True(t, isValid)
}

func TestVerify_failure(t *testing.T) {
	t.Parallel()
	isValid, err := authn.Verify("some password", "this is not an argon2id hash")
	assert.False(t, isValid)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported non-argon2id stored password")
}

func BenchmarkHashArgon2id(b *testing.B) {
	for b.Loop() {
		authn.NewSaltedArgon2idDevOnly("my password 123")
	}
}
