package directory_test

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/burningmantech/ranger-ims-go/directory"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"testing"
)

func TestMariaDB(t *testing.T) {
	t.Parallel()

	dbName := "rangers-test"
	username := "user"
	password := "password"

	ctx := t.Context()
	_, sqlDB := newEmptyDB(t, ctx, dbName, username, password)
	db := directory.DB{DB: sqlDB}
	_, err := db.ExecContext(ctx, "select 1")
	require.NoError(t, err)
	s, err := db.PrepareContext(ctx, "select 1")
	require.NoError(t, err)
	require.NoError(t, s.Close())
	rows, err := db.QueryContext(ctx, "select 1")
	require.NoError(t, err)
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	row := db.QueryRowContext(ctx, "select 1")
	require.NoError(t, row.Err())
}

func newEmptyDB(t *testing.T, ctx context.Context, database, username, password string) (testcontainers.Container, *sql.DB) {
	t.Helper()

	ctr, err := testcontainers.GenericContainer(ctx,
		testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image:        store.MariaDBDockerImage,
				ExposedPorts: []string{"3306/tcp"},
				WaitingFor:   wait.ForLog("port: 3306  mariadb.org binary distribution"),
				Env: map[string]string{
					"MARIADB_RANDOM_ROOT_PASSWORD": "true",
					"MARIADB_DATABASE":             database,
					"MARIADB_USER":                 username,
					"MARIADB_PASSWORD":             password,
				},
			},
			Started: true,
		},
	)
	testcontainers.CleanupContainer(t, ctr)
	require.NoError(t, err)
	port, err := ctr.MappedPort(ctx, "3306/tcp")
	require.NoError(t, err)
	dbHostPort := int32(port.Int())
	db, err := directory.MariaDB(ctx, conf.ClubhouseDB{
		Hostname: fmt.Sprint(":", dbHostPort),
		Database: database,
		Username: username,
		Password: password,
	})
	require.NoError(t, err)
	return ctr, db
}
