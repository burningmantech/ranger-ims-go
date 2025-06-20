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

package directory_test

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/burningmantech/ranger-ims-go/directory"
	"github.com/burningmantech/ranger-ims-go/lib/testctr"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"io"
	"testing"
)

func TestMariaDB(t *testing.T) {
	t.Parallel()

	dbName := "rangers-test"
	username := "user"
	password := "password"

	ctx := t.Context()
	_, sqlDB := newEmptyDB(t, ctx, dbName, username, password)
	db := directory.DBQ{DB: sqlDB}
	_, err := db.ExecContext(ctx, "select 1")
	require.NoError(t, err)
	s, err := db.PrepareContext(ctx, "select 1")
	require.NoError(t, err)
	defer shut(t, s)
	//nolint:sqlclosecheck
	rows, err := db.QueryContext(ctx, "select 1")
	require.NoError(t, err)
	defer shut(t, rows)
	require.NoError(t, rows.Err())
	row := db.QueryRowContext(ctx, "select 1")
	require.NoError(t, row.Err())
}

func shut(t *testing.T, s io.Closer) {
	t.Helper()
	require.NoError(t, s.Close())
}

func newEmptyDB(t *testing.T, ctx context.Context, database, username, password string) (testcontainers.Container, *sql.DB) {
	t.Helper()

	ctr, cleanup, dbHostPort, err := testctr.MariaDBContainer(ctx, database, username, password)
	t.Cleanup(cleanup)
	require.NoError(t, err)

	db, err := directory.MariaDB(ctx,
		conf.Directory{
			ClubhouseDB: conf.ClubhouseDB{
				Hostname: fmt.Sprint(":", dbHostPort),
				Database: database,
				Username: username,
				Password: password,
			},
		},
	)
	require.NoError(t, err)
	return ctr, db
}
