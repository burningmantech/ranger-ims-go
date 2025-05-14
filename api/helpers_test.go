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
	"database/sql"
	"errors"
	_ "github.com/burningmantech/ranger-ims-go/lib/noopdb"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestMustWriteJSONErrors(t *testing.T) {
	t.Parallel()

	// error if the response can't be marshalled as JSON
	rec := httptest.NewRecorder()
	req := &http.Request{URL: &url.URL{}}
	cantBeMarshalled := complex64(1 + 1i)
	ok := mustWriteJSON(rec, req, cantBeMarshalled)
	assert.False(t, ok)
	assert.Equal(t, http.StatusInternalServerError, rec.Result().StatusCode)

	// error if the JSON can't be written to the response writer
	w := angryResponseWriter{httptest.NewRecorder()}
	ok = mustWriteJSON(w, req, "can be marshalled")
	assert.False(t, ok)
	assert.Equal(t, http.StatusInternalServerError, rec.Result().StatusCode)
}

func TestReadBodyAsErrors(t *testing.T) {
	t.Parallel()

	// error if the request body read fails
	req := &http.Request{
		Body: angryReader{},
	}
	_, errHTTP := readBodyAs[any](req)
	require.NotNil(t, errHTTP)
	assert.Equal(t, http.StatusBadRequest, errHTTP.Code)

	// error if the request body isn't valid JSON
	req = &http.Request{
		Body: io.NopCloser(strings.NewReader("this isn't json")),
	}
	_, errHTTP = readBodyAs[any](req)
	require.NotNil(t, errHTTP)
	require.Equal(t, http.StatusBadRequest, errHTTP.Code)
}

func TestEventFromFormValue(t *testing.T) {
	t.Parallel()

	// error if the form can't be parsed
	req := &http.Request{URL: &url.URL{RawQuery: "this;is;invalid"}}
	_, errHTTP := eventFromFormValue(req, nil)
	require.NotNil(t, errHTTP)
	require.Equal(t, http.StatusBadRequest, errHTTP.Code)
	require.Contains(t, errHTTP.ResponseMessage, "Failed to parse form")

	// error if no event_id in request path
	req = &http.Request{}
	_, errHTTP = eventFromFormValue(req, nil)
	require.NotNil(t, errHTTP)
	require.Equal(t, http.StatusBadRequest, errHTTP.Code)
	require.Contains(t, errHTTP.ResponseMessage, "No event_id")

	// error if we can't read the event from the DB
	u, err := url.Parse("a?event_id=0")
	require.NoError(t, err)
	db, err := sql.Open("noop", "")
	require.NoError(t, err)
	req = &http.Request{URL: u}
	_, errHTTP = eventFromFormValue(req, &store.DB{DB: db})
	require.NotNil(t, errHTTP)
	require.Equal(t, http.StatusInternalServerError, errHTTP.Code)
	require.Contains(t, errHTTP.ResponseMessage, "Failed to get event")
}

func TestGetEvent(t *testing.T) {
	t.Parallel()
	dbNoop, err := sql.Open("noop", "")
	db := &store.DB{DB: dbNoop}
	require.NoError(t, err)

	// no eventName provided
	req := &http.Request{}
	_, errHTTP := getEvent(req, "", db)
	require.NotNil(t, errHTTP)
	require.Equal(t, http.StatusBadRequest, errHTTP.Code)
	require.Contains(t, errHTTP.ResponseMessage, "No eventName")

	// other DB failure
	_, errHTTP = getEvent(req, "dummy", db)
	require.NotNil(t, errHTTP)
	require.Equal(t, http.StatusInternalServerError, errHTTP.Code)
	require.Contains(t, errHTTP.ResponseMessage, "Failed to fetch")
}

// angryResponseWriter is an http.ResponseWriter that complains if
// you try to write to it.
type angryResponseWriter struct {
	*httptest.ResponseRecorder
}

func (angryResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("go away!")
}

type angryReader struct {
	io.ReadCloser
}

func (angryReader) Read([]byte) (int, error) {
	return 0, errors.New("go away!")
}

func (angryReader) Close() error {
	return nil
}
