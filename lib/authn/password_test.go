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
	"github.com/burningmantech/ranger-ims-go/lib/authn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestVerifyPassword_success(t *testing.T) {
	t.Parallel()
	pw, s := "Hardware", "my_little_salty"
	hashed := "ee9a23000af19a22acd0d9a22dfe9558580771dc"
	assert.Equal(t, hashed, authn.Hash(pw, s))

	stored := "my_little_salty:" + hashed
	vp, err := authn.Verify(pw, stored)
	require.NoError(t, err)
	assert.True(t, vp)

	vp, err = authn.Verify("wrong password", stored)
	require.NoError(t, err)
	assert.False(t, vp)
}

func TestVerifyPassword_badStoredValue(t *testing.T) {
	t.Parallel()
	noColonInThisString := "abcdefg"
	_, err := authn.Verify("some_password", noColonInThisString)
	require.Error(t, err)
	require.Contains(t, "invalid hashed password", err.Error())
}

func TestNewSalted(t *testing.T) {
	t.Parallel()
	pw := "this is my password"
	saltedPw := authn.NewSalted(pw)
	isValid, err := authn.Verify(pw, saltedPw)
	require.NoError(t, err)
	require.True(t, isValid)
}

func FuzzNewSaltedVerify(f *testing.F) {
	f.Add("pass")
	f.Add("")
	f.Add("🔥")
	f.Add(strings.Repeat("some text,", 1000))

	f.Fuzz(func(t *testing.T, pw string) {
		saltedHashed := authn.NewSalted(pw)
		assert.True(t, utf8.ValidString(saltedHashed))
		// the separator
		assert.Contains(t, saltedHashed, ":")
		// we use a base64-encoded salt that takes 22 bytes, then ":" is 1, and the hash is 40
		assert.Len(t, saltedHashed, 63)
		isValid, err := authn.Verify(pw, saltedHashed)
		require.NoError(t, err)
		assert.True(t, isValid)
	})
}

func TestHashArgon2id(t *testing.T) {
	t.Parallel()
	hash := authn.NewSaltedArgon2idDevOnly("my password 123")

	isValid, err := authn.Verify("my password wrong", hash)
	require.NoError(t, err)
	assert.False(t, isValid)

	isValid, err = authn.Verify("my password 123", hash)
	require.NoError(t, err)
	assert.True(t, isValid)
}
