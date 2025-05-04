package integration_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	_ "embed"
	"fmt"
	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mariadb"
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

	// DB 1 start with schema version 6, then gets migrated
	_, db1 := newUnmigratedDB(t, ctx, database, username, password)
	defer func() {
		_ = db1.Close()
	}()
	// Run the 6 migration
	err := runScript(ctx, db1, schema06)
	require.NoError(t, err)
	// now migrate to current schema version
	err = store.MigrateDB(ctx, db1)
	require.NoError(t, err)

	// DB and container 1 starts with no tables, then gets migrated
	_, db2 := newUnmigratedDB(t, ctx, database, username, password)
	defer func() {
		_ = db2.Close()
	}()
	// migrate to current schema version
	err = store.MigrateDB(ctx, db2)
	require.NoError(t, err)

	// the two databases should have the same set of tables
	var tableNamesDB1 []string
	rows1, err := db1.QueryContext(ctx, `show tables`)
	require.NoError(t, err)
	defer func() {
		_ = rows1.Close()
	}()
	for rows1.Next() {
		var tableName string
		require.NoError(t, rows1.Scan(&tableName))
		tableNamesDB1 = append(tableNamesDB1, tableName)
	}
	require.NoError(t, rows1.Err())
	var tableNamesDB2 []string
	rows2, err := db2.QueryContext(ctx, `show tables`)
	require.NoError(t, err)
	defer func() {
		_ = rows2.Close()
	}()
	for rows2.Next() {
		var tableName string
		require.NoError(t, rows2.Scan(&tableName))
		tableNamesDB2 = append(tableNamesDB2, tableName)
	}
	require.NoError(t, rows2.Err())
	slices.Sort(tableNamesDB1)
	slices.Sort(tableNamesDB2)
	require.Equal(t, tableNamesDB1, tableNamesDB2)

	// for each table, check that the two databases have the same
	// "CREATE TABLE" statement.
	for _, tableName := range tableNamesDB1 {
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

	ctr, err := mariadb.Run(ctx, store.MariaDBDockerImage,
		mariadb.WithDatabase(database),
		mariadb.WithUsername(username),
		mariadb.WithPassword(password),
	)
	testcontainers.CleanupContainer(t, ctr)
	require.NoError(t, err)
	port, err := ctr.MappedPort(ctx, "3306/tcp")
	require.NoError(t, err)

	require.NoError(t, err)
	dbHostPort := int32(port.Int())
	db, err := store.MariaDB(ctx, conf.StoreMariaDB{
		HostName: "",
		HostPort: dbHostPort,
		Database: database,
		Username: username,
		Password: password,
	}, false)
	require.NoError(t, err)
	return ctr, db
}

func runScript(ctx context.Context, imsDB *sql.DB, sql string) error {
	script := "BEGIN NOT ATOMIC\n" + sql + "\nEND"
	_, err := imsDB.ExecContext(ctx, script)
	if err != nil {
		return fmt.Errorf("[ExecContext]: %w", err)
	}
	return nil
}
