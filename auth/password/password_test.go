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

package password_test

import (
	"github.com/burningmantech/ranger-ims-go/auth/password"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestVerifyPassword_success(t *testing.T) {
	t.Parallel()
	pw, s := "Hardware", "my_little_salty"
	hashed := "ee9a23000af19a22acd0d9a22dfe9558580771dc"
	assert.Equal(t, hashed, password.Hash(pw, s))

	stored := "my_little_salty:" + hashed
	vp, err := password.Verify(pw, stored)
	require.NoError(t, err)
	assert.True(t, vp)

	vp, err = password.Verify("wrong password", stored)
	require.NoError(t, err)
	assert.False(t, vp)
}

func TestVerifyPassword_badStoredValue(t *testing.T) {
	t.Parallel()
	noColonInThisString := "abcdefg"
	_, err := password.Verify("some_password", noColonInThisString)
	require.Error(t, err)
	require.Contains(t, "invalid hashed password", err.Error())
}

func TestNewSalted(t *testing.T) {
	t.Parallel()
	pw := "this is my password"
	saltedPw := password.NewSalted(pw)
	isValid, err := password.Verify(pw, saltedPw)
	require.NoError(t, err)
	require.True(t, isValid)
}
