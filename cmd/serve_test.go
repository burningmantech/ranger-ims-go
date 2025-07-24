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
	"context"
	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"testing"
)

func TestRunServer(t *testing.T) {
	t.Parallel()
	imsCfg := conf.DefaultIMS()

	// This will have the server pick a random port
	imsCfg.Core.Port = 0
	imsCfg.Directory.Directory = conf.DirectoryTypeFake
	imsCfg.Store.Type = conf.DBStoreTypeFake

	// * Start the server with a cancellable Context
	// * Wait for the server to start listening (when addrChan responds)
	// * Cancel the context, thus starting server shutdown
	ctx, cancel := context.WithCancel(t.Context())
	addrChan := make(chan string, 1)
	go func() {
		<-addrChan
		cancel()
	}()
	exitCode := runServerInternal(ctx, imsCfg, true, addrChan)
	assert.Equal(t, 69, exitCode)
}

const someMemoryLimit int64 = 5555555555555

// exampleMemoryStatFile contains a hierarchical_memory_limit value that will be
// used by TestTuneMemoryLimit.
var exampleMemoryStatFile = `active_file 0
hierarchical_memory_limit ` + strconv.FormatInt(someMemoryLimit, 10) + `
hierarchical_memsw_limit 9223372036854771712
total_cache 24576`

func TestTuneMemoryLimit(t *testing.T) {
	t.Parallel()
	initialMemLimit := getMemoryLimit()
	defer debug.SetMemoryLimit(initialMemLimit)

	tempFilePath := filepath.Join(t.TempDir(), "memory.stat")
	err := os.WriteFile(tempFilePath, []byte(exampleMemoryStatFile), 0600)
	require.NoError(t, err)
	tuneMemoryLimit(tempFilePath)

	newGoMemLimit := getMemoryLimit()
	// this must match the calculation in tuneMemoryLimit
	expected := someMemoryLimit / 5 * 4
	assert.Equal(t, expected, newGoMemLimit)
}

// getMemoryLimit returns the runtime's current memory limit (a.k.a. GOMEMLIMIT), without
// changing that memory limit.
func getMemoryLimit() int64 {
	return debug.SetMemoryLimit(-1)
}
