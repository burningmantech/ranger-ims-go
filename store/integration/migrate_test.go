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
	"crypto/rand"
	"database/sql"
	_ "embed"
	"fmt"
	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/burningmantech/ranger-ims-go/lib/testctr"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"golang.org/x/sync/errgroup"
	"io"
	"slices"
	"testing"
)

//go:embed 06.sql
var schema06 string

// TestMigrateSameAsCurrentSchema checks the migration path for an
// old version of an IMS database.
//
// It brings up two MariaDB databases, one from IMS schema version 6
// (in this dir, as 06.sql), and one from the current version of the
// schema (in current.sql). It then migrates the version 06 database
// all the way up to current, using the same num-from-num.sql files
// that are used to migrate real IMS databases. At the end of those
// migrations, we expect both databases to have identical sets of
// tables, and for each table, we expect them to have the same
// "CREATE TABLE" SQL. If the "CREATE TABLE" statements are different
// from one side to the other, presumably a new migration should be
// created that gets them back in sync.
func TestMigrateSameAsCurrentSchema(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	database := rand.Text()
	username := rand.Text()
	password := rand.Text()

	// Bring up two DB containers in parallel
	var db1, db2 *sql.DB
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		_, db1 = newUnmigratedDB(t, groupCtx, database, username, password)
		return nil
	})
	group.Go(func() error {
		_, db2 = newUnmigratedDB(t, groupCtx, database, username, password)
		return nil
	})
	require.NoError(t, group.Wait())
	defer shut(db1)
	defer shut(db2)

	// DB 1 start with schema version 6, then gets migrated to the latest
	// DB 2 starts with no tables, then gets migrated to the latest

	// DB1: Run the version 6 migration
	err := runScript(ctx, db1, schema06)
	require.NoError(t, err)
	// DB1: Now migrate to current schema version
	err = store.MigrateDB(ctx, db1)
	require.NoError(t, err)

	// DB2: migrate straight up to current schema version
	err = store.MigrateDB(ctx, db2)
	require.NoError(t, err)
	// Run MigrateDB again, which is a no-op
	err = store.MigrateDB(ctx, db2)
	require.NoError(t, err)

	// The two databases should have the same set of tables
	var dbTables [2][]string

	for i, db := range []*sql.DB{db1, db2} {
		rows, err := db.QueryContext(ctx, `show tables`)
		require.NoError(t, err)
		for rows.Next() {
			var tableName string
			require.NoError(t, rows.Scan(&tableName))
			dbTables[i] = append(dbTables[i], tableName)
		}
		require.NoError(t, rows.Err())
		slices.Sort(dbTables[i])
		shut(rows)
	}
	require.Equal(t, dbTables[0], dbTables[1])

	// for each table, check that the two databases have the same
	// "CREATE TABLE" statement.
	for _, tableName := range dbTables[0] {
		row1 := db1.QueryRowContext(ctx, `show create table `+tableName)
		require.NoError(t, err)
		var tableName string
		var createTable1 string
		require.NoError(t, row1.Scan(&tableName, &createTable1))

		row2 := db2.QueryRowContext(ctx, `show create table `+tableName)
		require.NoError(t, err)
		var createTable2 string
		require.NoError(t, row2.Scan(&tableName, &createTable2))

		assert.Equal(t, createTable1, createTable2)
	}
}

func newUnmigratedDB(t *testing.T, ctx context.Context, database, username, password string) (testcontainers.Container, *sql.DB) {
	t.Helper()

	ctr, cleanup, dbHostPort, err := testctr.MariaDBContainer(ctx, database, username, password)
	t.Cleanup(cleanup)
	require.NoError(t, err)

	db, err := store.SqlDB(ctx,
		conf.DBStore{
			Type: conf.DBStoreTypeMaria,
			MariaDB: conf.DBStoreMaria{
				HostName: "",
				HostPort: dbHostPort,
				Database: database,
				Username: username,
				Password: password,
			},
		},
		false,
	)
	require.NoError(t, err)
	return ctr, db
}

func runScript(ctx context.Context, imsDBQ *sql.DB, script string) error {
	_, err := imsDBQ.ExecContext(ctx, script)
	if err != nil {
		return fmt.Errorf("[ExecContext]: %w", err)
	}
	return nil
}

func shut(c io.Closer) {
	_ = c.Close()
}
