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
	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMigrateFakeDB(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	db, err := store.SqlDB(ctx,
		conf.DBStore{
			Type: conf.DBStoreTypeFake,
			Fake: conf.DefaultIMS().Store.Fake,
		},
		true,
	)
	require.NoError(t, err)
	defer shut(db)

	// Check that the schema version has been bumped high enough (15 as of 2025-05-31)
	r := db.QueryRowContext(ctx, "select VERSION from SCHEMA_INFO")
	var version int64
	require.NoError(t, r.Scan(&version))
	require.GreaterOrEqual(t, version, int64(15))

	// Check that the data seeding worked (from seed.sql)
	r = db.QueryRowContext(ctx, "select NAME from INCIDENT_TYPE where ID = 3")
	var name string
	require.NoError(t, r.Scan(&name))
	require.Equal(t, "Sound Complaint", name)
}
