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

package bmapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/burningmantech/ranger-ims-go/lib/bmapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchCamps(t *testing.T) {
	t.Parallel()

	var gotPath, gotQuery, gotAPIKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotPath = req.URL.Path
		gotQuery = req.URL.RawQuery
		gotAPIKey = req.Header.Get("X-API-Key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"uid": "a1", "name": "Camp Fun Times", "location_string": "4:15 & E", "year": 2025},
			{"uid": "a2", "name": "Camp Quiet", "location_string": null, "year": 2025}
		]`))
	}))
	defer server.Close()

	records, err := bmapi.NewClient(server.URL, "secret-key").
		Fetch(t.Context(), bmapi.KindCamp, 2025)
	require.NoError(t, err)

	assert.Equal(t, "/api/camp", gotPath)
	assert.Equal(t, "year=2025", gotQuery)
	assert.Equal(t, "secret-key", gotAPIKey)

	require.Len(t, records, 2)
	assert.Equal(t, "Camp Fun Times", records[0].Name)
	assert.Equal(t, "4:15 & E", records[0].LocationString)
	// A null location_string is just an empty one, not an error.
	assert.Equal(t, "Camp Quiet", records[1].Name)
	assert.Empty(t, records[1].LocationString)

	// The whole object comes back untouched, since that's what IMS stores as a
	// Place's external data.
	var raw map[string]any
	require.NoError(t, json.Unmarshal(records[0].Raw, &raw))
	assert.Equal(t, map[string]any{
		"uid": "a1", "name": "Camp Fun Times", "location_string": "4:15 & E", "year": float64(2025),
	}, raw)
}

func TestFetchArtAndMV(t *testing.T) {
	t.Parallel()

	var gotPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotPaths = append(gotPaths, req.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"uid": "a1", "name": "Something"}]`))
	}))
	defer server.Close()

	client := bmapi.NewClient(server.URL, "secret-key")

	_, err := client.Fetch(t.Context(), bmapi.KindArt, 2025)
	require.NoError(t, err)
	// Mutant vehicles have no location of their own.
	mvs, err := client.Fetch(t.Context(), bmapi.KindMV, 2026)
	require.NoError(t, err)
	require.Len(t, mvs, 1)
	assert.Empty(t, mvs[0].LocationString)

	assert.Equal(t, []string{"/api/art", "/api/mv"}, gotPaths)
}

func TestFetchEmptyYear(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	// A year the API has nothing for isn't an error here. Deciding what to do
	// about that is the caller's business.
	records, err := bmapi.NewClient(server.URL, "secret-key").
		Fetch(t.Context(), bmapi.KindCamp, 1999)
	require.NoError(t, err)
	assert.Empty(t, records)
}

func TestFetchErrorStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"detail": "year must be greater than 2025"}`))
	}))
	defer server.Close()

	_, err := bmapi.NewClient(server.URL, "secret-key").
		Fetch(t.Context(), bmapi.KindMV, 2020)
	require.Error(t, err)
	// The upstream complaint is the useful part of the message.
	assert.Contains(t, err.Error(), "422")
	assert.Contains(t, err.Error(), "year must be greater than 2025")
}

func TestFetchNotAnArray(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html>who knows</html>`))
	}))
	defer server.Close()

	_, err := bmapi.NewClient(server.URL, "secret-key").
		Fetch(t.Context(), bmapi.KindCamp, 2025)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "other than a JSON array")
}

func TestFetchUnreachable(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	serverURL := server.URL
	server.Close()

	_, err := bmapi.NewClient(serverURL, "secret-key").
		Fetch(t.Context(), bmapi.KindCamp, 2025)
	require.Error(t, err)
}

func TestParseKind(t *testing.T) {
	t.Parallel()

	kind, err := bmapi.ParseKind("camp")
	require.NoError(t, err)
	assert.Equal(t, bmapi.KindCamp, kind)

	kind, err = bmapi.ParseKind("art")
	require.NoError(t, err)
	assert.Equal(t, bmapi.KindArt, kind)

	kind, err = bmapi.ParseKind("mv")
	require.NoError(t, err)
	assert.Equal(t, bmapi.KindMV, kind)

	// "Other" places are hand-written in IMS, with no upstream API.
	_, err = bmapi.ParseKind("other")
	require.Error(t, err)

	_, err = bmapi.ParseKind("")
	require.Error(t, err)
}

func TestTrailingSlashInBaseURL(t *testing.T) {
	t.Parallel()

	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotPath = req.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	_, err := bmapi.NewClient(server.URL+"/", "secret-key").
		Fetch(t.Context(), bmapi.KindCamp, 2025)
	require.NoError(t, err)
	assert.Equal(t, "/api/camp", gotPath)
}
