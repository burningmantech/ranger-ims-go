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

package ecsmetadata

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAllFetchesEveryDocument(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// #nosec G705 // XSS via taint analysis. It's a test.
		_, _ = w.Write([]byte(`{"path":"` + r.URL.Path + `"}`))
	}))
	t.Cleanup(srv.Close)

	out := All(t.Context(), srv.Client(), srv.URL+"/v4/abc")

	require.Contains(t, out, "Container metadata ("+srv.URL+"/v4/abc)\n")
	require.Contains(t, out, "{\n  \"path\": \"/v4/abc\"\n}")
	require.Contains(t, out, "Task metadata ("+srv.URL+"/v4/abc/task)\n")
	require.Contains(t, out, "{\n  \"path\": \"/v4/abc/task\"\n}")
	require.Contains(t, out, "Container stats ("+srv.URL+"/v4/abc/stats)\n")
	require.Contains(t, out, "{\n  \"path\": \"/v4/abc/stats\"\n}")
	require.Contains(t, out, "Task stats ("+srv.URL+"/v4/abc/task/stats)\n")
	require.Contains(t, out, "{\n  \"path\": \"/v4/abc/task/stats\"\n}")
}

func TestAllReportsFailuresInline(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v4/abc/stats" {
			http.Error(w, "nope", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`not json`))
	}))
	t.Cleanup(srv.Close)

	out := All(t.Context(), srv.Client(), srv.URL+"/v4/abc")

	require.Contains(t, out, "Failed with status 500 Internal Server Error: nope")
	// A body that isn't JSON is shown as-is, and the other documents still come through.
	require.Contains(t, out, "Task stats ("+srv.URL+"/v4/abc/task/stats)\n")
	require.Contains(t, out, "not json")
}

func TestAllReportsUnreachableEndpoint(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.NotFoundHandler())
	uri := srv.URL + "/v4/abc"
	srv.Close()

	out := All(t.Context(), http.DefaultClient, uri)

	require.Contains(t, out, "Failed to fetch:")
}
