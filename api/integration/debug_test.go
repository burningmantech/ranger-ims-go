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
	"net/http"
	"strings"
	"testing"

	"github.com/burningmantech/ranger-ims-go/lib/testctr"
	"github.com/stretchr/testify/require"
)

func TestGetDBInfo(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)

	body, resp := srv.admin(ctx).imsGetBodyBytes(ctx, srv.url.JoinPath("/ims/api/debug/dbinfo").String())
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)
	info := string(body)

	require.Contains(t, info, "IMS database")
	require.Contains(t, info, "Directory database")
	require.Contains(t, info, shared.cfg.Store.MariaDB.Database)
	require.Contains(t, info, shared.cfg.Directory.ClubhouseDB.Database)
	require.NotContains(t, info, shared.cfg.Store.MariaDB.Password)
	require.NotContains(t, info, shared.cfg.Directory.ClubhouseDB.Password)
	// Both servers answered, rather than either section reporting a failure.
	require.Equal(t, 2, strings.Count(info, "VERSION():                   "+testctr.MariaDBVersion))
	require.NotContains(t, info, "Failed")
	require.NotContains(t, info, "(not reported)")
}

func TestGetDBVariables(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)

	body, resp := srv.admin(ctx).imsGetBodyBytes(ctx, srv.url.JoinPath("/ims/api/debug/dbvariables").String())
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)
	vars := string(body)

	require.Equal(t, 2, strings.Count(vars, "\n    max_connections = "))
	require.Equal(t, 2, strings.Count(vars, "\n    Uptime = "))
	require.NotContains(t, vars, "Failed")
}
