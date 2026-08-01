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

package api

import (
	"errors"
	"fmt"
	"testing"

	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

func deadlockErr() error {
	return &mysql.MySQLError{Number: 1213, Message: "Deadlock found when trying to get lock; try restarting transaction"}
}

func TestIsDeadlockError(t *testing.T) {
	t.Parallel()

	require.True(t, isDeadlockError(deadlockErr()))

	// The handlers wrap the driver error several layers deep before it reaches
	// the retry, so it has to be found through the wrapping.
	require.True(t, isDeadlockError(fmt.Errorf("[attach]: %w", deadlockErr())))

	require.False(t, isDeadlockError(nil))
	require.False(t, isDeadlockError(errors.New("some other failure")))
	// A duplicate key is a different error that must not be retried this way.
	require.False(t, isDeadlockError(&mysql.MySQLError{Number: 1062}))
}

func TestRetryOnDeadlockReturnsFirstSuccessWithoutRetrying(t *testing.T) {
	t.Parallel()

	calls := 0
	got, errHTTP := retryOnDeadlock(func() (int32, *herr.HTTPError) {
		calls++
		return 7, nil
	})

	require.Nil(t, errHTTP)
	require.Equal(t, int32(7), got)
	require.Equal(t, 1, calls)
}

func TestRetryOnDeadlockRetriesUntilItSucceeds(t *testing.T) {
	t.Parallel()

	calls := 0
	got, errHTTP := retryOnDeadlock(func() (int32, *herr.HTTPError) {
		calls++
		if calls < 3 {
			return 0, herr.InternalServerError("Failed to attach", deadlockErr()).From("[attach]")
		}
		return 7, nil
	})

	require.Nil(t, errHTTP)
	require.Equal(t, int32(7), got)
	require.Equal(t, 3, calls)
}

func TestRetryOnDeadlockGivesUpAndReportsTheDeadlock(t *testing.T) {
	t.Parallel()

	calls := 0
	_, errHTTP := retryOnDeadlock(func() (int32, *herr.HTTPError) {
		calls++
		return 0, herr.InternalServerError("Failed to attach", deadlockErr()).From("[attach]")
	})

	require.NotNil(t, errHTTP)
	require.Equal(t, maxDeadlockAttempts, calls)
	// The underlying deadlock is still identifiable in what's returned, so the
	// logs say why the request failed rather than just that it did.
	require.True(t, isDeadlockError(errHTTP.InternalErr))
	require.Contains(t, errHTTP.InternalErr.Error(), "gave up after")
}

// Anything that isn't a deadlock is the caller's answer, returned as-is.
func TestRetryOnDeadlockDoesNotRetryOtherErrors(t *testing.T) {
	t.Parallel()

	calls := 0
	_, errHTTP := retryOnDeadlock(func() (int32, *herr.HTTPError) {
		calls++
		return 0, herr.NotFound("Incident not found", errors.New("no rows"))
	})

	require.NotNil(t, errHTTP)
	require.Equal(t, 1, calls)
	require.Equal(t, "Incident not found", errHTTP.ResponseMessage)
}

func TestRetryOnDeadlockErrRetriesTheSameWay(t *testing.T) {
	t.Parallel()

	calls := 0
	errHTTP := retryOnDeadlockErr(func() *herr.HTTPError {
		calls++
		if calls < 2 {
			return herr.InternalServerError("Failed to strike", deadlockErr()).From("[strike]")
		}
		return nil
	})

	require.Nil(t, errHTTP)
	require.Equal(t, 2, calls)
}
