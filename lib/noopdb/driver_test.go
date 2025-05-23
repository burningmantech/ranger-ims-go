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
	tx, err := conn.Begin()
	require.NoError(t, err)
	// Stmt
	require.NoError(t, stmt.Close())
	require.Equal(t, 0, stmt.NumInput())
	result, err := stmt.Exec(nil)
	require.NoError(t, err)
	rows, err := stmt.Query(nil)
	require.NoError(t, err)
	// Result
	lastInsert, err := result.LastInsertId()
	require.Equal(t, int64(0), lastInsert)
	require.NoError(t, err)
	aff, err := result.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(0), aff)
	// Rows
	require.Nil(t, rows.Columns())
	require.NoError(t, rows.Close())
	require.NoError(t, rows.Next(nil))
	// Tx
	require.NoError(t, tx.Rollback())
	require.NoError(t, tx.Commit())
}
