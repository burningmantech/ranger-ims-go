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

package cache_test

import (
	"context"
	"github.com/burningmantech/ranger-ims-go/lib/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"sync/atomic"
	"testing"
	"time"
)

type cacheVal struct {
	nonThreadSafe int64
	threadSafe    int64
}

func doTest(t *testing.T, ttl time.Duration, rounds int64) (numRefreshes int64) {
	t.Helper()
	var refreshCountNonThreadSafe int64
	var refreshCountAtomic atomic.Int64
	cacher := cache.New[cacheVal](ttl, func(ctx context.Context) (cacheVal, error) {
		refreshCountNonThreadSafe++
		return cacheVal{
			nonThreadSafe: refreshCountNonThreadSafe,
			threadSafe:    refreshCountAtomic.Add(1),
		}, nil
	})

	group, ctx := errgroup.WithContext(t.Context())
	for range rounds {
		group.Go(func() error {
			cv, err := cacher.Get(ctx)
			require.NoError(t, err)
			require.Equal(t, cv.threadSafe, cv.nonThreadSafe)
			return err
		})
	}
	require.NoError(t, group.Wait())
	assert.Equal(t, refreshCountAtomic.Load(), refreshCountNonThreadSafe)

	return refreshCountAtomic.Load()
}

func TestInMemoryCache_WorksWithNoRace(t *testing.T) {
	t.Parallel()

	// With a ttl of 0 and 1000 concurrent calls to "get", verify we got 1000 calls to the refresher function
	// and no concurrent calls of refresher code (`go test -race` ought to fail here too, e.g. if the
	// mutex is removed from the code).
	numRefreshes := doTest(t, 0*time.Nanosecond, 1000)
	require.Equal(t, int64(1000), numRefreshes)

	// With a very long ttl and 1000 calls to "get", verify that only 1 call to the refresher function
	// actually occurred.
	numRefreshes = doTest(t, 1*time.Hour, 1000)
	require.Equal(t, int64(1), numRefreshes)

	// With some intermediate ttl durations, we expect the numRefreshes to be somewhere between the extremes.
	numRefreshes = doTest(t, 50*time.Nanosecond, 1000)
	require.LessOrEqual(t, int64(1), numRefreshes)
	require.GreaterOrEqual(t, int64(1000), numRefreshes)
	numRefreshes = doTest(t, 500*time.Nanosecond, 1000)
	require.GreaterOrEqual(t, int64(1000), numRefreshes)
	require.LessOrEqual(t, int64(1), numRefreshes)
	numRefreshes = doTest(t, 5*time.Microsecond, 1000)
	require.GreaterOrEqual(t, int64(1000), numRefreshes)
	require.LessOrEqual(t, int64(1), numRefreshes)
	numRefreshes = doTest(t, 50*time.Microsecond, 1000)
	require.GreaterOrEqual(t, int64(1000), numRefreshes)
	require.LessOrEqual(t, int64(1), numRefreshes)
}
