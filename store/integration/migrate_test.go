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
	"github.com/testcontainers/testcontainers-go/wait"
	"slices"
	"strconv"
	"strings"
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
// created that gets them back in sync
func TestMigrateSameAsCurrentSchema(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	database := rand.Text()
	username := rand.Text()
	password := rand.Text()

	// DB 1 start with schema version 6, then gets migrated
	container1, db1 := newDB(t, ctx, database, username, password)
	defer func() {
		_ = container1.Terminate(ctx)
		_ = db1.Close()
	}()
	// Run the 6 migration
	err := runScript(ctx, db1, schema06)
	require.NoError(t, err)
	// now migrate to current schema version
	err = store.MigrateDB(ctx, db1)
	assert.NoError(t, err)

	// DB and container 1 starts with no tables, then gets migrated
	container2, db2 := newDB(t, ctx, database, username, password)
	defer func() {
		_ = container2.Terminate(ctx)
		_ = db2.Close()
	}()
	// migrate to current schema version
	err = store.MigrateDB(ctx, db2)
	assert.NoError(t, err)

	// the two databases should have the same set of tables
	var tableNamesDB1 []string
	rows, err := db1.QueryContext(ctx, `show tables`)
	require.NoError(t, err)
	for rows.Next() {
		var tableName string
		require.NoError(t, rows.Scan(&tableName))
		tableNamesDB1 = append(tableNamesDB1, tableName)
	}
	var tableNamesDB2 []string
	rows, err = db2.QueryContext(ctx, `show tables`)
	require.NoError(t, err)
	for rows.Next() {
		var tableName string
		require.NoError(t, rows.Scan(&tableName))
		tableNamesDB2 = append(tableNamesDB2, tableName)
	}
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

func newDB(t *testing.T, ctx context.Context, database, username, password string) (testcontainers.Container, *sql.DB) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        store.MariaDBDockerImage,
		ExposedPorts: []string{"3306/tcp"},
		WaitingFor:   wait.ForListeningPort("3306/tcp"),
		Env: map[string]string{
			"MARIADB_RANDOM_ROOT_PASSWORD": "true",
			"MARIADB_DATABASE":             database,
			"MARIADB_USER":                 username,
			"MARIADB_PASSWORD":             password,
		},
	}
	container, err := testcontainers.GenericContainer(ctx,
		testcontainers.GenericContainerRequest{
			ContainerRequest: req,
			Started:          true,
		},
	)
	require.NoError(t, err)
	endpoint, err := container.Endpoint(ctx, "")
	require.NoError(t, err)
	port, err := strconv.ParseInt(strings.TrimPrefix(endpoint, "localhost:"), 0, 32)
	require.NoError(t, err)
	dbHostPort := int32(port)
	db := store.MariaDB(ctx, conf.StoreMariaDB{
		HostName: "",
		HostPort: dbHostPort,
		Database: database,
		Username: username,
		Password: password,
	}, false)
	return container, db
}

func runScript(ctx context.Context, imsDB *sql.DB, sql string) error {
	script := "BEGIN NOT ATOMIC\n" + sql + "\nEND"
	_, err := imsDB.ExecContext(ctx, script)
	if err != nil {
		return fmt.Errorf("[ExecContext]: %w", err)
	}
	return nil
}
