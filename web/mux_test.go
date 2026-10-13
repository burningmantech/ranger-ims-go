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
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/burningmantech/ranger-ims-go/web"
	"github.com/stretchr/testify/require"
)

var templEndpoints = []string{
	"/ims/app",
	"/ims/app/admin",
	"/ims/app/admin/errorlogs",
	"/ims/app/admin/places",
	"/ims/app/admin/events",
	"/ims/app/admin/types",
	"/ims/app/events/SomeEvent/places",
	"/ims/app/events/SomeEvent/field_reports",
	"/ims/app/events/SomeEvent/field_reports/123",
	"/ims/app/events/SomeEvent/incidents",
	"/ims/app/events/SomeEvent/incidents/123",
	"/ims/app/events/SomeEvent/visits",
	"/ims/app/events/SomeEvent/visits/123",
	"/ims/app/settings",
	"/ims/app/help",
	"/ims/app/search",
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
	s := httptest.NewTestServer(t, web.AddToMux(nil, cfg))
	// The server speaks over an in-memory network, so only its own client can
	// reach it. Asking for that client is also what populates s.URL.
	client := s.Client()
	serverURL, err := url.Parse(s.URL)
	require.NoError(t, err)
	for _, endpoint := range templEndpoints {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL.JoinPath(endpoint).String(), nil)
		require.NoError(t, err)
		// #nosec G704 // SSRF via taint analysis. This test controls the URLs.
		resp, err := client.Do(httpReq)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equalf(t, "text/html; charset=utf-8", resp.Header.Get("Content-Type"),
			"Wrong content type for templ endpoint %v", endpoint)
		csp := resp.Header.Get("Content-Security-Policy")
		require.Containsf(t, csp, "frame-ancestors 'self'",
			"Templ endpoint %v may be framed by another origin", endpoint)
		require.Equalf(t, "same-origin", resp.Header.Get("Referrer-Policy"),
			"Templ endpoint %v may leak its referrer to other origins", endpoint)
		bod, err := io.ReadAll(resp.Body)
		require.NoError(t, resp.Body.Close())
		require.NoError(t, err)
		require.Contains(t, string(bod), "IMS © Burning Man Project")
		requireScriptsAllowedByCSP(t, endpoint, string(bod), csp)
	}
}

var (
	scriptElementRE   = regexp.MustCompile(`(?s)<script([^>]*)>(.*?)</script>`)
	handlerAttrRE     = regexp.MustCompile(`(?i)<[^>]*\son[a-z]+\s*=`)
	scriptSrcPolicyRE = regexp.MustCompile(`(?:^|;\s*)script-src ([^;]*)`)
)

// requireScriptsAllowedByCSP checks that the CSP would let a page's scripts
// run, and that it wouldn't let anything else run: script-src may only allow
// IMS's own files and hashes of inline scripts, and the page may have no
// inline event handlers, since the CSP blocks those.
func requireScriptsAllowedByCSP(t *testing.T, endpoint, page, csp string) {
	t.Helper()
	scriptSrc := scriptSrcPolicyRE.FindStringSubmatch(csp)
	require.NotNilf(t, scriptSrc, "Templ endpoint %v has no script-src in its CSP", endpoint)
	sources := strings.Fields(scriptSrc[1])
	require.Equalf(t, "'self'", sources[0], "Templ endpoint %v script-src", endpoint)
	for _, source := range sources[1:] {
		require.Truef(t, strings.HasPrefix(source, "'sha256-"),
			"Templ endpoint %v allows script source %v", endpoint, source)
	}
	for _, script := range scriptElementRE.FindAllStringSubmatch(page, -1) {
		if strings.Contains(script[1], "src=") {
			continue
		}
		sum := sha256.Sum256([]byte(script[2]))
		hash := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
		require.Containsf(t, sources, hash,
			"Templ endpoint %v has an inline script that its CSP blocks: %v", endpoint, script[0])
	}
	require.Emptyf(t, handlerAttrRE.FindAllString(page, -1),
		"Templ endpoint %v has inline event handlers, which its CSP blocks", endpoint)
}

func TestRedirects(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	cfg := conf.DefaultIMS()
	require.NoError(t, cfg.Validate())
	s := httptest.NewTestServer(t, web.AddToMux(nil, cfg))
	// The server speaks over an in-memory network, so only its own client can
	// reach it. Asking for that client is also what populates s.URL.
	client := s.Client()
	serverURL, err := url.Parse(s.URL)
	require.NoError(t, err)

	// Note the trailing slash. This should get caught by the catchall handler,
	// which will send us to the same URL without that trailing slash.
	path := serverURL.JoinPath("/ims/app/events/SomeEvent/incidents/../")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, path.String(), nil)
	require.NoError(t, err)
	// #nosec G704 // SSRF via taint analysis.
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
	// #nosec G704 // SSRF via taint analysis.
	resp, err = client.Do(httpReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, "/ims/app/events/SomeEvent/incidents", resp.Request.URL.Path)

	// This won't match any endpoint
	path = serverURL.JoinPath("/ims/app/events/SomeEvent/book_reports")
	httpReq, err = http.NewRequestWithContext(ctx, http.MethodGet, path.String(), nil)
	require.NoError(t, err)
	// #nosec G704 // SSRF via taint analysis.
	resp, err = client.Do(httpReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}

type exampleAction struct {
	output *bytes.Buffer
}

func (e exampleAction) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	_, _ = fmt.Fprintln(e.output, "      in the action")
}

func firstAdapter(output *bytes.Buffer) web.Adapter {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprintln(output, "firstAdapter before")
			next.ServeHTTP(w, r)
			_, _ = fmt.Fprintln(output, "firstAdapter after")
		})
	}
}

func secondAdapter(output *bytes.Buffer) web.Adapter {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprintln(output, "  secondAdapter before")
			next.ServeHTTP(w, r)
			_, _ = fmt.Fprintln(output, "  secondAdapter after")
		})
	}
}

func thirdAdapter(output *bytes.Buffer) web.Adapter {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprintln(output, "    thirdAdapter before")
			next.ServeHTTP(w, r)
			_, _ = fmt.Fprintln(output, "    thirdAdapter after")
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
