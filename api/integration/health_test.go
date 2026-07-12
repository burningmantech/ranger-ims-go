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

package integration_test

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLiveness(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	path := shared.serverURL.JoinPath("healthz")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path.String(), nil)
	require.NoError(t, err)
	client := http.Client{Timeout: 10 * time.Second}

	// #nosec G704 // SSRF via taint analysis.
	resp, err := client.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "ack", strings.TrimSpace(string(body)))
}

func TestReadiness(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	// The test server has a live IMS database at the current schema version
	// and a live directory source, so the server must report ready.
	path := shared.serverURL.JoinPath("readyz")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path.String(), nil)
	require.NoError(t, err)
	client := http.Client{Timeout: 10 * time.Second}

	// #nosec G704 // SSRF via taint analysis.
	resp, err := client.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "ready", strings.TrimSpace(string(body)))
}
