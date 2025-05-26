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

package cmd

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthCheckSuccess(t *testing.T) {
	t.Parallel()

	ser := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ims/api/ping" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ack"))
		}
	}))

	exitCode := runHealthCheckInternal(t.Context(), ser.URL)
	if exitCode != 0 {
		t.Errorf("wanted exit code 0, got %v", exitCode)
	}
}

func TestHealthCheckBadStatus(t *testing.T) {
	t.Parallel()

	ser := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ims/api/ping" {
			w.WriteHeader(http.StatusTeapot)
			_, _ = w.Write([]byte("ack"))
		}
	}))

	exitCode := runHealthCheckInternal(t.Context(), ser.URL)
	if exitCode != 5 {
		t.Errorf("wanted exit code 5, got %v", exitCode)
	}
}

func TestHealthCheckBadResponse(t *testing.T) {
	t.Parallel()

	ser := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ims/api/ping" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("nack"))
		}
	}))

	exitCode := runHealthCheckInternal(t.Context(), ser.URL)
	if exitCode != 6 {
		t.Errorf("wanted exit code 6, got %v", exitCode)
	}
}
