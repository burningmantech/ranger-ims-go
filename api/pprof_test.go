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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/stretchr/testify/require"
)

// gzipMagic starts every binary pprof profile.
var gzipMagic = []byte{0x1f, 0x8b}

func servePprof(t *testing.T, profile, handle, query string) *httptest.ResponseRecorder {
	t.Helper()
	action := GetPprof{nil, testUserStore(), []string{"SomeAdmin"}, profile}
	req := requestWithClaims(t, authz.IMSClaims{Handle: handle})
	req.URL.RawQuery = query
	rec := httptest.NewRecorder()
	action.ServeHTTP(rec, req)
	return rec
}

func TestGetPprof(t *testing.T) {
	t.Parallel()

	// A non-admin gets nothing, not even the cheap profiles.
	rec := servePprof(t, "heap", "SomeRanger", "")
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "GlobalAdministrateDebugging")

	rec = servePprof(t, "profile", "SomeRanger", "seconds=1")
	require.Equal(t, http.StatusForbidden, rec.Code)

	// A caller with no handle holds no roles at all.
	rec = servePprof(t, "goroutine", "", "")
	require.Equal(t, http.StatusForbidden, rec.Code)

	// An admin gets a binary heap profile.
	rec = servePprof(t, "heap", "SomeAdmin", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, gzipMagic, rec.Body.Bytes()[:2])

	// An admin can ask for goroutine stacks as text.
	rec = servePprof(t, "goroutine", "SomeAdmin", "debug=2")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "goroutine ")

	// An admin can't hold the CPU profiler beyond the cap.
	rec = servePprof(t, "profile", "SomeAdmin", "seconds=61")
	require.Equal(t, http.StatusBadRequest, rec.Code)

	rec = servePprof(t, "trace", "SomeAdmin", "seconds=forever")
	require.Equal(t, http.StatusBadRequest, rec.Code)

	// But can take a short CPU profile and trace.
	rec = servePprof(t, "profile", "SomeAdmin", "seconds=1")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, gzipMagic, rec.Body.Bytes()[:2])

	rec = servePprof(t, "trace", "SomeAdmin", "seconds=1")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "go 1.")
}
