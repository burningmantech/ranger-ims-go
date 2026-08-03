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
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/burningmantech/ranger-ims-go/api"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

type exampleAction struct {
	output *bytes.Buffer
}

func (e exampleAction) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintln(e.output, "      in the action")
}

func firstAdapter(output *bytes.Buffer) api.Adapter {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(output, "firstAdapter before")
			next.ServeHTTP(w, r)
			fmt.Fprintln(output, "firstAdapter after")
		})
	}
}

func secondAdapter(output *bytes.Buffer) api.Adapter {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(output, "  secondAdapter before")
			next.ServeHTTP(w, r)
			fmt.Fprintln(output, "  secondAdapter after")
		})
	}
}

func thirdAdapter(output *bytes.Buffer) api.Adapter {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(output, "    thirdAdapter before")
			next.ServeHTTP(w, r)
			fmt.Fprintln(output, "    thirdAdapter after")
		})
	}
}

// TestAdapt demonstrates how the Adapter pattern works.
func TestAdapt(t *testing.T) {
	t.Parallel()
	b := bytes.Buffer{}
	api.Adapt(
		exampleAction{output: &b},
		firstAdapter(&b),
		secondAdapter(&b),
		thirdAdapter(&b),
	).ServeHTTP(nil, nil)
	require.Equal(t, ""+
		"firstAdapter before\n"+
		"  secondAdapter before\n"+
		"    thirdAdapter before\n"+
		"      in the action\n"+
		"    thirdAdapter after\n"+
		"  secondAdapter after\n"+
		"firstAdapter after\n",
		b.String(),
	)
}

// fakeErrorLogger stands in for the real *errorlog.Logger, so that the capture
// path can be exercised without a database.
type fakeErrorLogger struct {
	rows []imsdb.AddErrorLogParams
}

func (f *fakeErrorLogger) Log(_ context.Context, record imsdb.AddErrorLogParams) {
	f.rows = append(f.rows, record)
}

func serveWithErrorRecording(logger *fakeErrorLogger, handler http.HandlerFunc) *httptest.ResponseRecorder {
	return serveRequestWithErrorRecording(
		logger, httptest.NewRequest(http.MethodGet, "/ims/api/whatever", nil), handler,
	)
}

func serveRequestWithErrorRecording(
	logger *fakeErrorLogger, req *http.Request, handler http.HandlerFunc,
) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	api.Adapt(
		handler,
		api.RecordErrors(logger),
		api.RecoverFromPanic(),
	).ServeHTTP(recorder, req)
	return recorder
}

// disconnectedRequest is a request whose client has already gone away, which is
// what the server sees when someone navigates off a page mid-fetch.
func disconnectedRequest(t *testing.T) *http.Request {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	return httptest.NewRequest(http.MethodGet, "/ims/api/whatever", nil).WithContext(ctx)
}

func TestRecordErrors_ServerError(t *testing.T) {
	t.Parallel()
	logger := &fakeErrorLogger{}

	recorder := serveWithErrorRecording(logger, func(w http.ResponseWriter, _ *http.Request) {
		herr.InternalServerError("Something went sideways", errors.New("the db said no")).
			From("[theHandler]").WriteResponse(w)
	})

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.Len(t, logger.rows, 1)
	row := logger.rows[0]
	require.EqualValues(t, http.StatusInternalServerError, row.HttpStatus)
	require.Equal(t, "Something went sideways", row.ResponseMessage.String)
	require.Equal(t, "[theHandler]: the db said no", row.InternalError.String)
	require.Equal(t, http.MethodGet, row.Method.String)
	require.Equal(t, "/ims/api/whatever", row.Path.String)
	require.False(t, row.StackTrace.Valid)
}

func TestRecordErrors_ClientError(t *testing.T) {
	t.Parallel()
	logger := &fakeErrorLogger{}

	recorder := serveWithErrorRecording(logger, func(w http.ResponseWriter, _ *http.Request) {
		herr.BadRequest("You sent nonsense", errors.New("bad json")).WriteResponse(w)
	})

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Empty(t, logger.rows)
}

func TestRecordErrors_Panic(t *testing.T) {
	t.Parallel()
	logger := &fakeErrorLogger{}

	recorder := serveWithErrorRecording(logger, func(_ http.ResponseWriter, _ *http.Request) {
		panic("everything is on fire")
	})

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.Len(t, logger.rows, 1)
	row := logger.rows[0]
	require.EqualValues(t, http.StatusInternalServerError, row.HttpStatus)
	require.Equal(t, "The server malfunctioned", row.ResponseMessage.String)
	require.Contains(t, row.InternalError.String, "everything is on fire")
	require.Contains(t, row.StackTrace.String, "goroutine")
}

func TestRecordErrors_ClientDisconnected(t *testing.T) {
	t.Parallel()
	logger := &fakeErrorLogger{}

	serveRequestWithErrorRecording(logger, disconnectedRequest(t), func(w http.ResponseWriter, r *http.Request) {
		herr.InternalServerError("Failed to fetch Incidents", r.Context().Err()).
			From("[getIncidents]").WriteResponse(w)
	})

	require.Empty(t, logger.rows)
}

// The disconnect only excuses the errors it actually caused. A request that
// broke for its own reasons still gets a row, even if the client then left.
func TestRecordErrors_ClientDisconnectedAfterUnrelatedError(t *testing.T) {
	t.Parallel()
	logger := &fakeErrorLogger{}

	serveRequestWithErrorRecording(logger, disconnectedRequest(t), func(w http.ResponseWriter, _ *http.Request) {
		herr.InternalServerError("Failed to fetch Incidents", errors.New("the db said no")).
			From("[getIncidents]").WriteResponse(w)
	})

	require.Len(t, logger.rows, 1)
	require.Equal(t, "[getIncidents]: the db said no", logger.rows[0].InternalError.String)
}

// A cancellation that isn't the client's doing is a bug in our own use of
// contexts, and mustn't hide behind the disconnect filter.
func TestRecordErrors_CancellationWithLiveClient(t *testing.T) {
	t.Parallel()
	logger := &fakeErrorLogger{}

	serveWithErrorRecording(logger, func(w http.ResponseWriter, _ *http.Request) {
		herr.InternalServerError("Failed to fetch Incidents", context.Canceled).
			From("[getIncidents]").WriteResponse(w)
	})

	require.Len(t, logger.rows, 1)
	require.Contains(t, logger.rows[0].InternalError.String, context.Canceled.Error())
}

// A blown deadline is the server failing to keep a promise, so it stays in the
// log even when the client has since given up.
func TestRecordErrors_DeadlineExceeded(t *testing.T) {
	t.Parallel()
	logger := &fakeErrorLogger{}

	serveRequestWithErrorRecording(logger, disconnectedRequest(t), func(w http.ResponseWriter, _ *http.Request) {
		herr.New(http.StatusServiceUnavailable, "Search took too long", context.DeadlineExceeded).
			From("[getSearch]").WriteResponse(w)
	})

	require.Len(t, logger.rows, 1)
	require.EqualValues(t, http.StatusServiceUnavailable, logger.rows[0].HttpStatus)
}

func TestRecordErrors_Success(t *testing.T) {
	t.Parallel()
	logger := &fakeErrorLogger{}

	recorder := serveWithErrorRecording(logger, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("all good"))
	})

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Empty(t, logger.rows)
}
