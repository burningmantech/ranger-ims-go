package noopdb_test

import (
	"database/sql"
	_ "github.com/burningmantech/ranger-ims-go/lib/noopdb"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNoOpDB(t *testing.T) {
	t.Parallel()
	db, err := sql.Open("noop", "")
	require.NoError(t, err)
	require.NoError(t, db.Close())
	// Driver
	conn, err := db.Driver().Open("")
	require.NoError(t, err)
	// Conn
	stmt, err := conn.Prepare("select 1")
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	tx, err := conn.Begin() //nolint:SA1019
	require.NoError(t, err)
	// Stmt
	require.NoError(t, stmt.Close())
	stmt.NumInput()
	result, err := stmt.Exec(nil)
	require.NoError(t, err)
	rows, err := stmt.Query(nil)
	require.NoError(t, err)
	// Result
	_, err = result.LastInsertId()
	require.NoError(t, err)
	_, err = result.RowsAffected()
	require.NoError(t, err)
	// Rows
	rows.Columns()
	require.NoError(t, rows.Close())
	require.NoError(t, rows.Next(nil))
	// Tx
	require.NoError(t, tx.Rollback())
	require.NoError(t, tx.Commit())
}
