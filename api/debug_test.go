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

package api

import (
	"net/http"
	"testing"

	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/stretchr/testify/require"
)

func TestGetConfigForbiddenWithoutGlobalAdministrateDebugging(t *testing.T) {
	t.Parallel()

	cfg := &conf.IMSConfig{}
	cfg.Core.JWTSecret = "hunter2"
	// A non-admin handle picks up AnyAuthenticatedUser, but not the debugging permission.
	action := GetConfig{nil, testUserStore(), []string{"SomeAdmin"}, cfg}

	_, errHTTP := action.getConfig(requestWithClaims(t, authz.IMSClaims{Handle: "SomeRanger"}))

	require.NotNil(t, errHTTP)
	require.Equal(t, http.StatusForbidden, errHTTP.Code)
	require.Contains(t, errHTTP.ResponseMessage, "GlobalAdministrateDebugging")
}

func TestGetConfigReturnsRedactedConfigForAdmin(t *testing.T) {
	t.Parallel()

	cfg := &conf.IMSConfig{}
	cfg.Core.Host = "some-host"
	cfg.Core.JWTSecret = "hunter2"
	cfg.Store.MariaDB.Password = "swordfish"
	action := GetConfig{nil, testUserStore(), []string{"SomeAdmin"}, cfg}

	configString, errHTTP := action.getConfig(requestWithClaims(t, authz.IMSClaims{Handle: "SomeAdmin"}))

	require.Nil(t, errHTTP)
	require.Contains(t, configString, "some-host")
	require.NotContains(t, configString, "hunter2")
	require.NotContains(t, configString, "swordfish")
}
