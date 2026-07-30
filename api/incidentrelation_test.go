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
	"context"
	"database/sql"
	"net/http"
	"testing"

	_ "github.com/burningmantech/ranger-ims-go/lib/noopdb"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"github.com/stretchr/testify/require"
)

// The happy paths of these helpers, and of the four endpoints built on them,
// are covered by the integration tests, which have a real MariaDB to talk to.
// What's left here are the DB failures, which need a DB that fails.

// deadDBQ is a DBQ whose DB has been closed, so every query through it fails.
func deadDBQ(t *testing.T) *store.DBQ {
	t.Helper()
	db, err := sql.Open("noop", "")
	require.NoError(t, err)
	require.NoError(t, db.Close())
	return store.NewDBQ(db, imsdb.New())
}

// writableDBTX accepts writes and, by way of the DBTX it wraps, fails reads.
// bumpIncidentVersionAndRead takes its DBTX as an argument, so that it can run
// inside its caller's transaction; that seam is what makes the "the bump landed
// but the read-back didn't" case reachable.
type writableDBTX struct {
	imsdb.DBTX
}

func (writableDBTX) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return wroteNothing{}, nil
}

type wroteNothing struct{}

func (wroteNothing) LastInsertId() (int64, error) { return 0, nil }
func (wroteNothing) RowsAffected() (int64, error) { return 0, nil }

func TestBumpIncidentVersionAndReadErrors(t *testing.T) {
	t.Parallel()

	// The bump itself fails.
	dead := deadDBQ(t)
	_, errHTTP := bumpIncidentVersionAndRead(t.Context(), dead, dead, 1, 2)
	require.NotNil(t, errHTTP)
	require.Equal(t, http.StatusInternalServerError, errHTTP.Code)
	require.Contains(t, errHTTP.ResponseMessage, "Failed to update Incident")

	// The bump succeeds and the read-back of the new version fails.
	_, errHTTP = bumpIncidentVersionAndRead(t.Context(), dead, writableDBTX{dead}, 1, 2)
	require.NotNil(t, errHTTP)
	require.Equal(t, http.StatusInternalServerError, errHTTP.Code)
	require.Contains(t, errHTTP.ResponseMessage, "Failed to fetch Incident")
}

func TestCurrentIncidentVersionError(t *testing.T) {
	t.Parallel()

	dead := deadDBQ(t)
	_, errHTTP := currentIncidentVersion(t.Context(), dead, 1, 2)
	require.NotNil(t, errHTTP)
	require.Equal(t, http.StatusInternalServerError, errHTTP.Code)
	require.Contains(t, errHTTP.ResponseMessage, "Failed to fetch Incident")
}

func TestReadIncidentRowError(t *testing.T) {
	t.Parallel()

	// A DB failure that isn't "no such row" is a 500. Only the missing-row case
	// gets the 404 that the integration tests exercise, since a 404 tells the
	// client its request was wrong, and this one wasn't.
	dead := deadDBQ(t)
	relReq := incidentRelationRequest{event: imsdb.Event{ID: 1}, number: 2}
	_, errHTTP := readIncidentRow(t.Context(), dead, relReq)
	require.NotNil(t, errHTTP)
	require.Equal(t, http.StatusInternalServerError, errHTTP.Code)
	require.Contains(t, errHTTP.ResponseMessage, "Failed to fetch Incident")
}
