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
	"sync/atomic"
	"testing"

	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests cover the race window between a creation's number allocation
// (a plain SELECT of MAX+1) and its INSERT: a concurrent creator can claim
// the same number first, and the server must retry with a fresh number
// rather than surface the duplicate-key error to the client.

func (q casInterceptor) CreateIncident(ctx context.Context, db imsdb.DBTX, arg imsdb.CreateIncidentParams) (int64, error) {
	if q.beforeCreateIncident != nil {
		q.beforeCreateIncident(ctx, arg)
	}
	return q.Querier.CreateIncident(ctx, db, arg)
}

func (q casInterceptor) CreateFieldReport(ctx context.Context, db imsdb.DBTX, arg imsdb.CreateFieldReportParams) error {
	if q.beforeCreateFieldReport != nil {
		q.beforeCreateFieldReport(ctx, arg)
	}
	return q.Querier.CreateFieldReport(ctx, db, arg)
}

func (q casInterceptor) CreateVisit(ctx context.Context, db imsdb.DBTX, arg imsdb.CreateVisitParams) (int64, error) {
	if q.beforeCreateVisit != nil {
		q.beforeCreateVisit(ctx, arg)
	}
	return q.Querier.CreateVisit(ctx, db, arg)
}

func TestNewIncidentRetriesWhenNumberIsTaken(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)

	// A concurrent creator claims the freshly allocated number before this
	// creation's INSERT runs, so the first attempt hits a duplicate key.
	// The hook asserts rather than requires, since it runs on the server's
	// request goroutine.
	var createAttempts atomic.Int32
	hookedURL := interceptedServer(t, casInterceptor{
		beforeCreateIncident: func(ctx context.Context, arg imsdb.CreateIncidentParams) {
			if createAttempts.Add(1) == 1 {
				_, err := shared.imsDBQ.CreateIncident(ctx, shared.imsDBQ, arg)
				assert.NoError(t, err)
			}
		},
	})
	hookedApis := ApiHelper{t: t, serverURL: hookedURL, jwt: apis.jwt}

	num := hookedApis.newIncidentSuccess(ctx, sampleIncident1(eventName))
	// The competitor took number 1 in this fresh event; the retry landed on 2.
	require.Equal(t, int32(2), num)
	require.Equal(t, int32(2), createAttempts.Load())

	retrieved, resp := apis.getIncident(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, num, retrieved.Number)
}

func TestNewIncidentGivesUpWhenNumbersKeepBeingTaken(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)

	// A competing creator wins the number race on every attempt.
	var createAttempts atomic.Int32
	hookedURL := interceptedServer(t, casInterceptor{
		beforeCreateIncident: func(ctx context.Context, arg imsdb.CreateIncidentParams) {
			createAttempts.Add(1)
			_, err := shared.imsDBQ.CreateIncident(ctx, shared.imsDBQ, arg)
			assert.NoError(t, err)
		},
	})
	hookedApis := ApiHelper{t: t, serverURL: hookedURL, jwt: apis.jwt}

	resp := hookedApis.newIncident(ctx, sampleIncident1(eventName))
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	require.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))
	require.NoError(t, resp.Body.Close())
	// The server exhausts its allocation retry budget, then gives up.
	require.Equal(t, int32(3), createAttempts.Load())
}

func TestNewFieldReportRetriesWhenNumberIsTaken(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)

	var createAttempts atomic.Int32
	hookedURL := interceptedServer(t, casInterceptor{
		beforeCreateFieldReport: func(ctx context.Context, arg imsdb.CreateFieldReportParams) {
			if createAttempts.Add(1) == 1 {
				err := shared.imsDBQ.CreateFieldReport(ctx, shared.imsDBQ, arg)
				assert.NoError(t, err)
			}
		},
	})
	hookedApis := ApiHelper{t: t, serverURL: hookedURL, jwt: apis.jwt}

	num := hookedApis.newFieldReportSuccess(ctx, sampleFieldReport1(eventName))
	require.Equal(t, int32(2), num)
	require.Equal(t, int32(2), createAttempts.Load())

	retrieved, resp := apis.getFieldReport(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, num, retrieved.Number)
}

func TestNewVisitRetriesWhenNumberIsTaken(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)
	resp := apisAdmin.addVisitWriter(ctx, eventName, userAliceHandle)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	var createAttempts atomic.Int32
	hookedURL := interceptedServer(t, casInterceptor{
		beforeCreateVisit: func(ctx context.Context, arg imsdb.CreateVisitParams) {
			if createAttempts.Add(1) == 1 {
				_, err := shared.imsDBQ.CreateVisit(ctx, shared.imsDBQ, arg)
				assert.NoError(t, err)
			}
		},
	})
	hookedApis := ApiHelper{t: t, serverURL: hookedURL, jwt: apis.jwt}

	num := hookedApis.newVisitSuccess(ctx, imsjson.Visit{
		Event:              eventName,
		GuestPreferredName: new("A. Guest"),
	})
	require.Equal(t, int32(2), num)
	require.Equal(t, int32(2), createAttempts.Load())

	retrieved, resp := apis.getVisit(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, num, retrieved.Number)
}
