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

package api_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/burningmantech/ranger-ims-go/api"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeActionLogger struct {
	rows []imsdb.AddActionLogParams
}

func (f *fakeActionLogger) Log(_ context.Context, record imsdb.AddActionLogParams) {
	f.rows = append(f.rows, record)
}

// drainBody is what a real handler does with its body, which is the only reason
// the capture ever sees anything.
func drainBody(_ http.ResponseWriter, r *http.Request) {
	_, _ = io.ReadAll(r.Body)
}

func serveWithActionLogging(
	logger *fakeActionLogger, mode api.ActionLogMode, req *http.Request, handler http.HandlerFunc,
) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	api.Adapt(handler, api.LogRequest(mode, logger, nil)).ServeHTTP(recorder, req)
	return recorder
}

func jsonRequest(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestLogRequest_RecordsMutationBody(t *testing.T) {
	t.Parallel()
	logger := &fakeActionLogger{}

	req := jsonRequest(http.MethodPost, "/ims/api/events/2026/incidents/1", `{"summary": "A new summary"}`)
	serveWithActionLogging(logger, api.LogMutation, req, drainBody)

	require.Len(t, logger.rows, 1)
	row := logger.rows[0]
	assert.Equal(t, "/ims/api/events/2026/incidents/1", row.Path.String)
	// Compacted, since the row stores what was sent rather than how it was typed.
	compacted := `{"summary":"A new summary"}`
	assert.Equal(t, compacted, row.RequestBody.String)
}

func TestLogRequest_RecordsBodyOfFailedRequest(t *testing.T) {
	t.Parallel()
	logger := &fakeActionLogger{}

	req := jsonRequest(http.MethodPost, "/ims/api/events/2026/incidents/1", `{"summary":"Nope"}`)
	serveWithActionLogging(logger, api.LogMutation, req, func(w http.ResponseWriter, r *http.Request) {
		drainBody(w, r)
		w.WriteHeader(http.StatusForbidden)
	})

	require.Len(t, logger.rows, 1)
	row := logger.rows[0]
	// An attempted mutation is worth keeping; the status is what says it didn't land.
	assert.JSONEq(t, `{"summary":"Nope"}`, row.RequestBody.String)
	assert.EqualValues(t, http.StatusForbidden, row.HttpStatus.Int16)
}

func TestLogRequest_RedactsPasswords(t *testing.T) {
	t.Parallel()
	logger := &fakeActionLogger{}

	req := jsonRequest(http.MethodPost, "/ims/api/directory/persons/5/password", `{"password":"hunter2"}`)
	serveWithActionLogging(logger, api.LogMutation, req, drainBody)

	require.Len(t, logger.rows, 1)
	body := logger.rows[0].RequestBody.String
	redacted := `{"password":"[redacted]"}`
	assert.Equal(t, redacted, body)
	assert.NotContains(t, body, "hunter2")
}

func TestLogRequest_RedactsNestedPasswords(t *testing.T) {
	t.Parallel()
	logger := &fakeActionLogger{}

	req := jsonRequest(http.MethodPost, "/ims/api/directory/persons",
		`{"persons":[{"handle":"Hubcap","new_password":"hunter2"}]}`)
	serveWithActionLogging(logger, api.LogMutation, req, drainBody)

	require.Len(t, logger.rows, 1)
	body := logger.rows[0].RequestBody.String
	assert.Contains(t, body, `"handle":"Hubcap"`)
	assert.Contains(t, body, `"new_password":"[redacted]"`)
	assert.NotContains(t, body, "hunter2")
}

func TestLogRequest_TruncatesOversizedBody(t *testing.T) {
	t.Parallel()
	logger := &fakeActionLogger{}

	huge := `{"summary":"` + strings.Repeat("x", 32*1024) + `"}`
	req := jsonRequest(http.MethodPost, "/ims/api/events/2026/incidents/1", huge)
	serveWithActionLogging(logger, api.LogMutation, req, drainBody)

	require.Len(t, logger.rows, 1)
	body := logger.rows[0].RequestBody.String
	assert.Less(t, len(body), len(huge))
	assert.True(t, strings.HasSuffix(body, "…[truncated]"), "body should be marked as truncated")
	assert.True(t, strings.HasPrefix(body, `{"summary":"xxx`))
}

func TestLogRequest_TruncatedPasswordIsStillRedacted(t *testing.T) {
	t.Parallel()
	logger := &fakeActionLogger{}

	// A body too long to parse, so redaction falls back to the regex.
	body := `{"password":"hunter2","notes":"` + strings.Repeat("y", 32*1024) + `"}`
	req := jsonRequest(http.MethodPost, "/ims/api/directory/persons", body)
	serveWithActionLogging(logger, api.LogMutation, req, drainBody)

	require.Len(t, logger.rows, 1)
	logged := logger.rows[0].RequestBody.String
	assert.Contains(t, logged, `"password":"[redacted]"`)
	assert.NotContains(t, logged, "hunter2")
}

func TestLogRequest_KeepsUnparseableBodyAsText(t *testing.T) {
	t.Parallel()
	logger := &fakeActionLogger{}

	req := jsonRequest(http.MethodPost, "/ims/api/events/2026/incidents/1", `{"summary": `)
	serveWithActionLogging(logger, api.LogMutation, req, drainBody)

	require.Len(t, logger.rows, 1)
	// Malformed JSON still says what the client tried to do.
	assert.Equal(t, `{"summary": `, logger.rows[0].RequestBody.String)
}

func TestLogRequest_IgnoresNonJSONBody(t *testing.T) {
	t.Parallel()
	logger := &fakeActionLogger{}

	req := httptest.NewRequest(http.MethodPost,
		"/ims/api/events/2026/incidents/1/attachments", strings.NewReader("\x89PNG\r\n\x1a\n"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=xyz")
	serveWithActionLogging(logger, api.LogMutation, req, drainBody)

	require.Len(t, logger.rows, 1)
	assert.False(t, logger.rows[0].RequestBody.Valid)
}

func TestLogRequest_MetadataModeOmitsBody(t *testing.T) {
	t.Parallel()
	logger := &fakeActionLogger{}

	req := jsonRequest(http.MethodPost, "/ims/api/auth", `{"identification":"Hubcap","password":"hunter2"}`)
	serveWithActionLogging(logger, api.LogMetadata, req, drainBody)

	require.Len(t, logger.rows, 1)
	row := logger.rows[0]
	assert.Equal(t, "/ims/api/auth", row.Path.String)
	assert.False(t, row.RequestBody.Valid)
}

func TestLogRequest_LogNothingWritesNoRow(t *testing.T) {
	t.Parallel()
	logger := &fakeActionLogger{}

	req := httptest.NewRequest(http.MethodGet, "/ims/api/events/2026/incidents", nil)
	serveWithActionLogging(logger, api.LogNothing, req, drainBody)

	assert.Empty(t, logger.rows)
}

func TestLogRequest_UnreadBodyIsNotLogged(t *testing.T) {
	t.Parallel()
	logger := &fakeActionLogger{}

	// A handler that rejects the request before reading it (no permission, say)
	// leaves nothing to capture.
	req := jsonRequest(http.MethodPost, "/ims/api/events/2026/incidents/1", `{"summary":"Unread"}`)
	serveWithActionLogging(logger, api.LogMutation, req, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	require.Len(t, logger.rows, 1)
	assert.False(t, logger.rows[0].RequestBody.Valid)
}
