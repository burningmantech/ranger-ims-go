package web

import (
	"github.com/srabraham/ranger-ims-go/conf"
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
	cfg := conf.DefaultIMS()
	s := httptest.NewServer(AddToMux(nil, cfg))
	defer s.Close()
	serverURL, err := url.Parse(s.URL)
	require.NoError(t, err)
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	for _, endpoint := range templEndpoints {
		httpReq, err := http.NewRequest("GET", serverURL.JoinPath(endpoint).String(), nil)
		require.NoError(t, err)
		resp, err := client.Do(httpReq)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, "text/html; charset=utf-8", resp.Header.Get("Content-Type"))
		bod, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Contains(t, string(bod), "IMS Software © Burning Man Project and its contributors")
	}
}

func TestCatchall(t *testing.T) {
	cfg := conf.DefaultIMS()
	s := httptest.NewServer(AddToMux(nil, cfg))
	defer s.Close()
	serverURL, err := url.Parse(s.URL)
	require.NoError(t, err)
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Note the trailing slash. This should get caught by the catchall handler,
	// which will send us to the same URL without that trailing slash.
	path := serverURL.JoinPath("/ims/app/events/SomeEvent/incidents/")
	httpReq, err := http.NewRequest("GET", path.String(), nil)
	require.NoError(t, err)
	resp, err := client.Do(httpReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	// Ta-da! Now there's no trailing slash
	require.Equal(t, "/ims/app/events/SomeEvent/incidents", resp.Request.URL.Path)

	// This won't match any endpoint
	path = serverURL.JoinPath("/ims/app/events/SomeEvent/book_reports")
	httpReq, err = http.NewRequest("GET", path.String(), nil)
	require.NoError(t, err)
	resp, err = client.Do(httpReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}
