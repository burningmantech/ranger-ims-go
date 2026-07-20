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
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
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
		Header: jsonContentType(),
		Body:   angryReader{},
	}
	_, errHTTP := readBodyAs[any](req)
	require.NotNil(t, errHTTP)
	assert.Equal(t, http.StatusBadRequest, errHTTP.Code)

	// error if the request body isn't valid JSON
	req = &http.Request{
		Header: jsonContentType(),
		Body:   io.NopCloser(strings.NewReader("this isn't json")),
	}
	_, errHTTP = readBodyAs[any](req)
	require.NotNil(t, errHTTP)
	require.Equal(t, http.StatusBadRequest, errHTTP.Code)
}

// TestReadBodyAsRequiresJSONContentType covers the CSRF defense in readBodyAs:
// only a JSON Content-Type is accepted, so no cross-site HTML form can reach a
// handler that reads a request body.
func TestReadBodyAsRequiresJSONContentType(t *testing.T) {
	t.Parallel()

	bodyOf := func(header http.Header) *http.Request {
		return &http.Request{
			Header: header,
			Body:   io.NopCloser(strings.NewReader(`{"identification": "Hardware"}`)),
		}
	}

	// a plain JSON Content-Type is accepted
	req := bodyOf(http.Header{"Content-Type": []string{"application/json"}})
	body, errHTTP := readBodyAs[map[string]string](req)
	require.Nil(t, errHTTP)
	assert.Equal(t, "Hardware", body["identification"])

	// a JSON Content-Type with parameters and odd casing is accepted too
	req = bodyOf(http.Header{"Content-Type": []string{"Application/JSON; charset=utf-8"}})
	body, errHTTP = readBodyAs[map[string]string](req)
	require.Nil(t, errHTTP)
	assert.Equal(t, "Hardware", body["identification"])

	// a missing Content-Type is rejected
	req = bodyOf(http.Header{})
	_, errHTTP = readBodyAs[map[string]string](req)
	require.NotNil(t, errHTTP)
	assert.Equal(t, http.StatusUnsupportedMediaType, errHTTP.Code)
	require.ErrorIs(t, errHTTP, errNoContentType)

	// text/plain is rejected. This is the Content-Type a cross-site HTML form
	// would use to smuggle a JSON body to IMS.
	req = bodyOf(http.Header{"Content-Type": []string{"text/plain"}})
	_, errHTTP = readBodyAs[map[string]string](req)
	require.NotNil(t, errHTTP)
	assert.Equal(t, http.StatusUnsupportedMediaType, errHTTP.Code)
	require.ErrorIs(t, errHTTP, errNonJSONContentType)

	// form encodings are rejected
	req = bodyOf(http.Header{"Content-Type": []string{"application/x-www-form-urlencoded"}})
	_, errHTTP = readBodyAs[map[string]string](req)
	require.NotNil(t, errHTTP)
	assert.Equal(t, http.StatusUnsupportedMediaType, errHTTP.Code)

	req = bodyOf(http.Header{"Content-Type": []string{"multipart/form-data; boundary=xyz"}})
	_, errHTTP = readBodyAs[map[string]string](req)
	require.NotNil(t, errHTTP)
	assert.Equal(t, http.StatusUnsupportedMediaType, errHTTP.Code)

	// an unparseable Content-Type is rejected
	req = bodyOf(http.Header{"Content-Type": []string{"application/json; charset"}})
	_, errHTTP = readBodyAs[map[string]string](req)
	require.NotNil(t, errHTTP)
	assert.Equal(t, http.StatusUnsupportedMediaType, errHTTP.Code)
	require.ErrorIs(t, errHTTP, errNonJSONContentType)
}

func jsonContentType() http.Header {
	return http.Header{"Content-Type": []string{"application/json"}}
}

func TestParseIfMatch(t *testing.T) {
	t.Parallel()

	requestWithIfMatch := func(value string) *http.Request {
		req := &http.Request{Header: http.Header{}}
		if value != "" {
			req.Header.Set("If-Match", value)
		}
		return req
	}

	// absent header means no version check
	version, errHTTP := parseIfMatch(requestWithIfMatch(""))
	require.Nil(t, errHTTP)
	assert.Nil(t, version)

	// "*" matches any current version, so it also means no version check
	version, errHTTP = parseIfMatch(requestWithIfMatch("*"))
	require.Nil(t, errHTTP)
	assert.Nil(t, version)

	// a quoted ETag is the normal case
	version, errHTTP = parseIfMatch(requestWithIfMatch(`"7"`))
	require.Nil(t, errHTTP)
	require.NotNil(t, version)
	assert.Equal(t, int32(7), *version)

	// an unquoted value is tolerated too
	version, errHTTP = parseIfMatch(requestWithIfMatch("12"))
	require.Nil(t, errHTTP)
	require.NotNil(t, version)
	assert.Equal(t, int32(12), *version)

	// a weak ETag is accepted, since compressing proxies weaken the strong
	// ETag we send and the browser echoes back whatever it was given
	version, errHTTP = parseIfMatch(requestWithIfMatch(`W/"7"`))
	require.Nil(t, errHTTP)
	require.NotNil(t, version)
	assert.Equal(t, int32(7), *version)

	// multiple ETags aren't supported
	_, errHTTP = parseIfMatch(requestWithIfMatch(`"7", "8"`))
	require.NotNil(t, errHTTP)
	assert.Equal(t, http.StatusBadRequest, errHTTP.Code)

	// garbage isn't a version
	_, errHTTP = parseIfMatch(requestWithIfMatch(`"unversioned"`))
	require.NotNil(t, errHTTP)
	assert.Equal(t, http.StatusBadRequest, errHTTP.Code)
}

func TestSetETag(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	setETag(rec, 42)
	assert.Equal(t, `"42"`, rec.Header().Get("ETag"))
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
	_, errHTTP = eventFromFormValue(req, store.NewDBQ(db, imsdb.New()))
	require.NotNil(t, errHTTP)
	require.Equal(t, http.StatusInternalServerError, errHTTP.Code)
	require.Contains(t, errHTTP.ResponseMessage, "Failed to get event")
}

func TestGetEvent(t *testing.T) {
	t.Parallel()
	dbNoop, err := sql.Open("noop", "")
	db := store.NewDBQ(dbNoop, imsdb.New())
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
