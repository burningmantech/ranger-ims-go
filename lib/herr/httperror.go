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
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// ApplicationProblemMediaType is described by RFC 9457.
// https://www.rfc-editor.org/rfc/rfc9457.html
const ApplicationProblemMediaType = "application/problem+json"

type HTTPError struct {
	Code            int
	ResponseMessage string
	InternalErr     error
	// ExpectedError indicates that this error should happen in normal operation.
	// It's just used to tell IMS not to bother logging this error.
	ExpectedError bool
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf(
		"HTTP %v: ResponseMessage:'%v', InternalError:'%v'",
		e.Code, e.ResponseMessage, e.InternalErr,
	)
}

func New(code int, message string, internalErr error) *HTTPError {
	if internalErr == nil {
		internalErr = errors.New(message)
	}
	return &HTTPError{
		Code:            code,
		ResponseMessage: message,
		InternalErr:     internalErr,
	}
}

// InternalServerError returns an http.StatusInternalServerError HTTPError.
func InternalServerError(userMessage string, err error) *HTTPError {
	return New(http.StatusInternalServerError, userMessage, err)
}

// BadRequest returns an http.StatusBadRequest HTTPError.
func BadRequest(userMessage string, err error) *HTTPError {
	return New(http.StatusBadRequest, userMessage, err)
}

func RequestEntityTooLarge(userMessage string, err error) *HTTPError {
	return New(http.StatusRequestEntityTooLarge, userMessage, err)
}

// Unauthorized returns an http.StatusUnauthorized HTTPError.
func Unauthorized(userMessage string, err error) *HTTPError {
	return New(http.StatusUnauthorized, userMessage, err)
}

// Forbidden returns an http.StatusForbidden HTTPError.
func Forbidden(userMessage string, err error) *HTTPError {
	return New(http.StatusForbidden, userMessage, err)
}

// NotFound returns an HTTP Not Found HTTPError.
func NotFound(userMessage string, err error) *HTTPError {
	return New(http.StatusNotFound, userMessage, err)
}

// From wraps the InternalErr using fmt.Sprintf. This should be used to specify
// the name of a function that returned an error. See httperror_test.go for
// examples of wrapping.
func (e *HTTPError) From(source string) *HTTPError {
	return &HTTPError{
		InternalErr:     fmt.Errorf("%v: %w", source, e.InternalErr),
		Code:            e.Code,
		ResponseMessage: e.ResponseMessage,
		ExpectedError:   e.ExpectedError,
	}
}

func (e *HTTPError) SetExpectedError() *HTTPError {
	e.ExpectedError = true
	return e
}

func (e *HTTPError) Unwrap() error {
	return e.InternalErr
}

func (e *HTTPError) WriteResponse(w http.ResponseWriter) {
	if !e.ExpectedError {
		slog.Error("Writing error HTTP response",
			"code", e.Code,
			"message", e.ResponseMessage,
			"internalError", e.InternalErr,
		)
	}

	p := Problem{
		Status:    e.Code,
		Detail:    e.ResponseMessage,
		Timestamp: time.Now(),
	}

	// Write headers, write status, write body
	w.Header().Set("Content-Type", ApplicationProblemMediaType)
	w.WriteHeader(e.Code)

	marshalled, err := json.Marshal(p)
	if err != nil {
		slog.Error("Failed to marshal problem response", "err", err)
		marshalled = []byte(`{"detail":"Failed to marshal problem response"}`)
	}
	_, _ = w.Write(marshalled)
}

// AsHTTPError converts an error into an HTTPError. The intended use is for
// when an error is known to actually be an HTTPError, but when it's declared
// as a different type. This function then asserts it's actually an HTTPError.
func AsHTTPError(err error) *HTTPError {
	errHTTP := &HTTPError{}
	if errors.As(err, &errHTTP) {
		return errHTTP
	}
	return InternalServerError(
		"Unknown server error",
		err,
	)
}
