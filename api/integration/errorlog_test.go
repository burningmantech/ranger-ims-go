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
	"testing"
	"time"

	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetErrorLog(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	referrer := "testGetErrorLog"
	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t), referrer: referrer}

	// Provoke a real 500 by way of a handler that panics.
	_, resp := apisAdmin.imsGet(ctx, apisAdmin.serverURL.JoinPath(panicPath).String(), nil)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	longAgo := time.Now().Add(-500 * time.Hour).UnixMilli()
	longFromNow := time.Now().Add(500 * time.Hour).UnixMilli()
	logs, response := apisAdmin.getErrorLogs(ctx, conv.FormatInt(longAgo), conv.FormatInt(longFromNow))
	require.NotNil(t, response)
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())

	var foundLog imsjson.ErrorLog
	for _, el := range logs {
		if el.Referrer == referrer {
			foundLog = el
		}
	}
	require.NotZero(t, foundLog)
	assert.Equal(t, panicPath, foundLog.Path)
	assert.Equal(t, "GET", foundLog.Method)
	assert.EqualValues(t, http.StatusInternalServerError, foundLog.HttpStatus)
	assert.Equal(t, userAdminHandle, foundLog.UserName)
	assert.Equal(t, "The server malfunctioned", foundLog.ResponseMessage)
	assert.Contains(t, foundLog.InternalError, "this handler always panics")
	assert.Contains(t, foundLog.StackTrace, "goroutine")
}

// A 4xx is the client's problem, not the server's, so it stays out of the
// error log.
func TestGetErrorLog_ClientErrorsAreNotRecorded(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	referrer := "testErrorLogClientError"
	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t), referrer: referrer}

	notFound := apisAdmin.serverURL.JoinPath("/ims/api/events/SomeFakeEvent/incidents/1").String()
	_, resp := apisAdmin.imsGet(ctx, notFound, nil)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	longAgo := time.Now().Add(-500 * time.Hour).UnixMilli()
	longFromNow := time.Now().Add(500 * time.Hour).UnixMilli()
	logs, response := apisAdmin.getErrorLogs(ctx, conv.FormatInt(longAgo), conv.FormatInt(longFromNow))
	require.NotNil(t, response)
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())

	for _, el := range logs {
		assert.NotEqual(t, referrer, el.Referrer)
	}
}

func TestGetErrorLog_BadTimeFilters(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}

	_, response := apisAdmin.getErrorLogs(ctx, "not a valid time", "")
	require.NotNil(t, response)
	require.Equal(t, http.StatusBadRequest, response.StatusCode)
	require.NoError(t, response.Body.Close())

	_, response = apisAdmin.getErrorLogs(ctx, "", "not a valid time")
	require.NotNil(t, response)
	require.Equal(t, http.StatusBadRequest, response.StatusCode)
	require.NoError(t, response.Body.Close())
}
