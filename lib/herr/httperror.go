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
	"errors"
	"fmt"
	"log/slog"
	"net/http"
)

type HTTPError struct {
	Code            int
	ResponseMessage string
	InternalErr     error
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

// S500 returns an HTTP Internal Server Error HTTPError.
func S500(message string, err error) *HTTPError {
	return New(http.StatusInternalServerError, message, err)
}

// S400 returns an HTTP Bad Request HTTPError.
func S400(message string, err error) *HTTPError {
	return New(http.StatusBadRequest, message, err)
}

// S401 returns an HTTP Unauthorized HTTPError.
func S401(message string, err error) *HTTPError {
	return New(http.StatusUnauthorized, message, err)
}

// S403 returns an HTTP Forbidden HTTPError.
func S403(message string, err error) *HTTPError {
	return New(http.StatusForbidden, message, err)
}

// S404 returns an HTTP Not Found HTTPError.
func S404(message string, err error) *HTTPError {
	return New(http.StatusNotFound, message, err)
}

// Src wraps the InternalErr using fmt.Sprintf. This should be used to specify
// the name of a function that returned an error. See httperror_test.go for
// examples of wrapping.
func (e *HTTPError) Src(fun string) *HTTPError {
	return &HTTPError{
		InternalErr:     fmt.Errorf("%v: %w", fun, e.InternalErr),
		Code:            e.Code,
		ResponseMessage: e.ResponseMessage,
	}
}

func (e *HTTPError) Unwrap() error {
	return e.InternalErr
}

func (e *HTTPError) WriteResponse(w http.ResponseWriter) {
	slog.Info("Sending HTTP response",
		"code", e.Code,
		"message", e.ResponseMessage,
		"internalError", e.InternalErr,
	)
	http.Error(w, e.ResponseMessage, e.Code)
}
