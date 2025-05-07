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

package web_test

import (
	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/burningmantech/ranger-ims-go/web"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

var templEndpoints = []string{
	"/ims/app",
	"/ims/app/admin",
	"/ims/app/admin/events",
	"/ims/app/admin/streets",
	"/ims/app/admin/types",
	"/ims/app/events/SomeEvent/field_reports",
	"/ims/app/events/SomeEvent/field_reports/123",
	"/ims/app/events/SomeEvent/incidents",
	"/ims/app/events/SomeEvent/incidents/123",
	"/ims/auth/login",
	"/ims/auth/logout",
}

// TestTemplEndpoints tests that the IMS server can render all the
// HTML pages and serve them at the correct paths.
func TestTemplEndpoints(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	cfg := conf.DefaultIMS()
	require.NoError(t, cfg.Validate())
	s := httptest.NewServer(web.AddToMux(nil, cfg))
	defer s.Close()
	serverURL, err := url.Parse(s.URL)
	require.NoError(t, err)
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	for _, endpoint := range templEndpoints {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL.JoinPath(endpoint).String(), nil)
		require.NoError(t, err)
		resp, err := client.Do(httpReq)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, "text/html; charset=utf-8", resp.Header.Get("Content-Type"))
		bod, err := io.ReadAll(resp.Body)
		require.NoError(t, resp.Body.Close())
		require.NoError(t, err)
		require.Contains(t, string(bod), "IMS Software © Burning Man Project and its contributors")
	}
}

func TestCatchall(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	cfg := conf.DefaultIMS()
	require.NoError(t, cfg.Validate())
	s := httptest.NewServer(web.AddToMux(nil, cfg))
	defer s.Close()
	serverURL, err := url.Parse(s.URL)
	require.NoError(t, err)
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Note the trailing slash. This should get caught by the catchall handler,
	// which will send us to the same URL without that trailing slash.
	path := serverURL.JoinPath("/ims/app/events/SomeEvent/incidents/")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, path.String(), nil)
	require.NoError(t, err)
	resp, err := client.Do(httpReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	// Ta-da! Now there's no trailing slash
	require.Equal(t, "/ims/app/events/SomeEvent/incidents", resp.Request.URL.Path)

	// This won't match any endpoint
	path = serverURL.JoinPath("/ims/app/events/SomeEvent/book_reports")
	httpReq, err = http.NewRequestWithContext(ctx, http.MethodGet, path.String(), nil)
	require.NoError(t, err)
	resp, err = client.Do(httpReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}
