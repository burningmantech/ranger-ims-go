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

package store

import (
	"context"
	"database/sql"

	"github.com/burningmantech/ranger-ims-go/store/imsdb"
)

// DBQ combines the SQL database and the Querier for the IMS datastore.
type DBQ struct {
	*sql.DB
	imsdb.Querier
}

func NewDBQ(sqlDB *sql.DB, querier imsdb.Querier) *DBQ {
	return &DBQ{
		DB:      sqlDB,
		Querier: querier,
	}
}

func (l DBQ) SchemaVersion(ctx context.Context, db imsdb.DBTX) (int16, error) {
	// This function is a demonstration of how calls into the Querier can be intercepted,
	// e.g. for logging/instrumenting purposes.
	return l.Querier.SchemaVersion(ctx, db)
}
