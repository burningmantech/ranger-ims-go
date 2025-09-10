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

package herr

import (
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"
)

func TestNew(t *testing.T) {
	t.Parallel()
	err := New(http.StatusOK, "ok", nil)
	assert.Equal(t, http.StatusOK, err.Code)
	assert.Equal(t, "ok", err.InternalErr.Error())
	assert.Equal(t, "ok", err.ResponseMessage)

	err = New(http.StatusOK, "ok", errors.New("some error"))
	assert.Equal(t, "some error", err.InternalErr.Error())
}

func TestError(t *testing.T) {
	t.Parallel()
	err := New(http.StatusOK, "ok", nil)
	assert.Equal(t, "HTTP 200: ResponseMessage:'ok', InternalError:'ok'", err.Error())
}

func TestWrap(t *testing.T) {
	t.Parallel()
	innerErr := errors.New("serious problem")
	outerErr := New(http.StatusTeapot, "message to user", innerErr)
	assert.Equal(t, innerErr, errors.Unwrap(outerErr))
	assert.ErrorIs(t, outerErr, innerErr)
}

func TestSrcWrap(t *testing.T) {
	t.Parallel()
	err := sampleFunction()
	require.Error(t, err)
	assert.Equal(t, "Hey user! something went wrong", err.ResponseMessage)
	assert.Equal(t, "[outer]: [middle]: [inner]: something bad", err.InternalErr.Error())
	assert.Equal(t, 500, err.Code)

	// The error is a wrapped version of the innermost error
	assert.ErrorIs(t, err, errInternal)
}

func TestAsHTTPError(t *testing.T) {
	t.Parallel()
	// take an HTTPError, convert it to error, then use AsHTTPError to recover it
	errHTTP := Unauthorized("hi user", errors.New("some error")).SetExpectedError()
	err := error(errHTTP)
	assert.Equal(t, errHTTP, AsHTTPError(err))

	err = errors.New("some error")
	errHTTP = AsHTTPError(err)
	assert.Equal(t, New(500, "Unknown server error", err), errHTTP)
}

func TestWriteResponse(t *testing.T) {
	t.Parallel()

	// use synctest to get a consistent time.Now()
	synctest.Test(t, func(t *testing.T) {
		t.Helper()
		rec := httptest.NewRecorder()
		errHTTP := Unauthorized("hi user", errors.New("some error"))
		errHTTP.WriteResponse(rec)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)

		problem := Problem{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
		assert.Equal(t, Problem{
			Status:    http.StatusUnauthorized,
			Detail:    "hi user",
			Timestamp: time.Now().UTC(),
		}, problem)
	})
}

var errInternal = errors.New("something bad")

func inner() *HTTPError {
	return New(http.StatusInternalServerError, "Hey user! something went wrong", errInternal)
}

func middle() *HTTPError {
	err := inner()
	if err != nil {
		return err.From("[inner]")
	}
	return nil
}

func outer() *HTTPError {
	err := middle()
	if err != nil {
		return err.From("[middle]")
	}
	return nil
}

func sampleFunction() *HTTPError {
	err := outer()
	if err != nil {
		return err.From("[outer]")
	}
	return nil
}
