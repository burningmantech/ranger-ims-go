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
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/burningmantech/ranger-ims-go/web"
	"github.com/stretchr/testify/require"
)

var templEndpoints = []string{
	"/ims/app",
	"/ims/app/admin",
	"/ims/app/admin/destinations",
	"/ims/app/admin/events",
	"/ims/app/admin/streets",
	"/ims/app/admin/types",
	"/ims/app/events/SomeEvent/destinations",
	"/ims/app/events/SomeEvent/field_reports",
	"/ims/app/events/SomeEvent/field_reports/123",
	"/ims/app/events/SomeEvent/incidents",
	"/ims/app/events/SomeEvent/incidents/123",
	"/ims/app/events/SomeEvent/stays",
	"/ims/app/events/SomeEvent/stays/123",
	"/ims/app/settings",
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
		require.Equalf(t, "text/html; charset=utf-8", resp.Header.Get("Content-Type"),
			"Wrong content type for templ endpoint %v", endpoint)
		bod, err := io.ReadAll(resp.Body)
		require.NoError(t, resp.Body.Close())
		require.NoError(t, err)
		require.Contains(t, string(bod), "IMS © Burning Man Project")
	}
}

func TestRedirects(t *testing.T) {
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

	// This will get redirected to the Incidents page
	path = serverURL.JoinPath("/ims/app/events/SomeEvent")
	httpReq, err = http.NewRequestWithContext(ctx, http.MethodGet, path.String(), nil)
	require.NoError(t, err)
	resp, err = client.Do(httpReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
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

type exampleAction struct {
	output *bytes.Buffer
}

func (e exampleAction) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintln(e.output, "      in the action")
}

func firstAdapter(output *bytes.Buffer) web.Adapter {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(output, "firstAdapter before")
			next.ServeHTTP(w, r)
			fmt.Fprintln(output, "firstAdapter after")
		})
	}
}

func secondAdapter(output *bytes.Buffer) web.Adapter {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(output, "  secondAdapter before")
			next.ServeHTTP(w, r)
			fmt.Fprintln(output, "  secondAdapter after")
		})
	}
}

func thirdAdapter(output *bytes.Buffer) web.Adapter {
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
	web.Adapt(
		exampleAction{output: &b}.ServeHTTP,
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
