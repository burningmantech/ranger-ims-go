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

package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/burningmantech/ranger-ims-go/lib/argon2id"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This runs through rootCmd, which is shared global state, so it must stay
// the only test in the package that executes a command.
func TestHashPassword(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	t.Cleanup(func() { rootCmd.SetOut(nil) })
	rootCmd.SetArgs([]string{"hash_password", "--password", "correct horse"})
	t.Cleanup(func() { rootCmd.SetArgs(nil) })

	require.NoError(t, rootCmd.Execute())

	hash := strings.TrimSpace(out.String())
	params, _, _, err := argon2id.DecodeHash(hash)
	require.NoError(t, err)
	assert.Equal(t, argon2id.ClubhouseParams.MemoryKiB, params.MemoryKiB)
	assert.Equal(t, argon2id.ClubhouseParams.Iterations, params.Iterations)

	match, err := argon2id.ComparePasswordAndHash("correct horse", hash)
	require.NoError(t, err)
	assert.True(t, match)

	match, err = argon2id.ComparePasswordAndHash("wrong horse", hash)
	require.NoError(t, err)
	assert.False(t, match)
}
