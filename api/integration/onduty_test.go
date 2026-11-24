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
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/rand"
	"github.com/stretchr/testify/require"
)

func TestGetOnDutyRangers(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)
	apisAdmin := srv.admin(ctx)
	apisAlice := srv.alice(ctx)

	eventName := rand.NonCryptoText()
	_, resp := apisAdmin.createEvent(ctx, imsjson.Event{Name: &eventName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apisAdmin.editAccess(ctx, imsjson.EventsAccess{
		eventName: imsjson.EventAccess{
			Readers: []imsjson.AccessRule{{
				Expression: "person:" + userAliceHandle,
				Validity:   "always",
			}},
		},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// The admin has two open shifts in clubhousedb_test_seed.sql, and is listed
	// once, in the newer one's all_rangers position.
	onDuty, resp := apisAlice.getOnDuty(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, []imsjson.OnDutyRanger{
		{Handle: userAdminHandle, Position: "Dirt"},
		{Handle: userAliceHandle, Position: "Nooperator"},
	}, onDuty)

	// 90 minutes ago, only the admin's older shift had started.
	onDuty, resp = apisAlice.getOnDutyAt(ctx, eventName, time.Now().Add(-90*time.Minute))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, []imsjson.OnDutyRanger{
		{Handle: userAdminHandle, Position: "Nooperator"},
	}, onDuty)

	// 4 hours ago, Alice was midway through a shift she has since finished.
	onDuty, resp = apisAlice.getOnDutyAt(ctx, eventName, time.Now().Add(-4*time.Hour))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, []imsjson.OnDutyRanger{
		{Handle: userAliceHandle, Position: "Nooperator"},
	}, onDuty)

	// A day ago, nobody was on duty.
	onDuty, resp = apisAlice.getOnDutyAt(ctx, eventName, time.Now().Add(-24*time.Hour))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Empty(t, onDuty)

	// A malformed time is rejected.
	path := apisAlice.serverURL.JoinPath("/ims/api/events/", eventName, "/on_duty").String() + "?at=yesterday"
	_, resp = apisAlice.imsGet[[]imsjson.OnDutyRanger](ctx, path)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}

func (a ApiHelper) getOnDuty(ctx context.Context, eventName string) ([]imsjson.OnDutyRanger, *http.Response) {
	a.t.Helper()
	path := a.serverURL.JoinPath("/ims/api/events/", eventName, "/on_duty").String()
	return a.imsGet[[]imsjson.OnDutyRanger](ctx, path)
}

func (a ApiHelper) getOnDutyAt(ctx context.Context, eventName string, at time.Time) ([]imsjson.OnDutyRanger, *http.Response) {
	a.t.Helper()
	path := a.serverURL.JoinPath("/ims/api/events/", eventName, "/on_duty").String() + "?at=" + url.QueryEscape(at.Format(time.RFC3339))
	return a.imsGet[[]imsjson.OnDutyRanger](ctx, path)
}
