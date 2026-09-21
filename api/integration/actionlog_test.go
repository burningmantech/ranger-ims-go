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
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
	"time"
)

func TestGetActionLog(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)

	referrer := "testGetActionLog"
	apisAdmin := srv.admin(ctx).withReferrer(referrer)

	// admin user can authenticate
	_, resp := apisAdmin.getAuth(ctx, "")
	require.NotNil(t, resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	longAgo := time.Now().Add(-500 * time.Hour).UnixMilli()
	longFromNow := time.Now().Add(500 * time.Hour).UnixMilli()
	logs, response := apisAdmin.getActionLogs(ctx, conv.FormatInt(longAgo), conv.FormatInt(longFromNow))
	require.NotNil(t, response)
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())

	var foundLog imsjson.ActionLog
	for _, al := range logs {
		if al.Referrer == referrer {
			foundLog = al
		}
	}
	assert.NotZero(t, foundLog)
	assert.Equal(t, "/ims/api/auth", foundLog.Path)
	assert.Equal(t, "GET", foundLog.Method)

	// Now test error cases
	_, response = apisAdmin.getActionLogs(ctx, "not a valid time", "")
	require.NotNil(t, response)
	require.Equal(t, http.StatusBadRequest, response.StatusCode)
	require.NoError(t, response.Body.Close())
	_, response = apisAdmin.getActionLogs(ctx, "", "not a valid time")
	require.NotNil(t, response)
	require.Equal(t, http.StatusBadRequest, response.StatusCode)
	require.NoError(t, response.Body.Close())
}

func TestActionLogRecordsMutation(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)

	apisAdmin := srv.admin(ctx)
	apis := srv.alice(ctx)
	eventName := newEventWithWriter(t, apisAdmin)

	num := apis.newIncidentSuccess(ctx, sampleIncident1(eventName))

	summary := "The summary this test is looking for"
	resp := apis.updateIncident(ctx, eventName, num, imsjson.Incident{
		Event:   eventName,
		Number:  num,
		Summary: &summary,
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	longAgo := time.Now().Add(-500 * time.Hour).UnixMilli()
	longFromNow := time.Now().Add(500 * time.Hour).UnixMilli()
	logs, response := apisAdmin.getActionLogs(ctx, conv.FormatInt(longAgo), conv.FormatInt(longFromNow))
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())

	editPath := "/ims/api/events/" + eventName + "/incidents/" + conv.FormatInt(int64(num))
	var editLog imsjson.ActionLog
	for _, al := range logs {
		if al.Path == editPath && al.Method == http.MethodPost {
			editLog = al
		}
	}
	require.NotZero(t, editLog, "no action log row for the incident edit")
	assert.Contains(t, editLog.RequestBody, summary)

	// A login is logged, but its body (which holds a password) is not.
	var authLog imsjson.ActionLog
	for _, al := range logs {
		if al.Path == "/ims/api/auth" && al.Method == http.MethodPost {
			authLog = al
		}
	}
	require.NotZero(t, authLog, "no action log row for the login")
	assert.Empty(t, authLog.RequestBody)
}
