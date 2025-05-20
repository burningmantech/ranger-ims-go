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
	"fmt"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"log/slog"
	"strings"
	"time"
)

// DBQ combines the SQL database and the Querier for the IMS datastore. It's convenient having those
// two types embedded in one struct, because it allows great flexibility in custom method overrides.
type DBQ struct {
	*sql.DB
	imsdb.Querier
}

func New(sqlDB *sql.DB, querier imsdb.Querier) *DBQ {
	db := &DBQ{
		DB:      sqlDB,
		Querier: querier,
	}
	return db
}

func (l DBQ) ExecContext(ctx context.Context, s string, i ...interface{}) (sql.Result, error) {
	start := time.Now()
	result, err := l.DB.ExecContext(ctx, s, i...)
	logQuery(s, start, err)
	return result, err
}

func (l DBQ) PrepareContext(ctx context.Context, s string) (*sql.Stmt, error) {
	start := time.Now()
	stmt, err := l.DB.PrepareContext(ctx, s)
	logQuery(s, start, err)
	return stmt, err
}

func (l DBQ) QueryContext(ctx context.Context, s string, i ...interface{}) (*sql.Rows, error) {
	start := time.Now()
	rows, err := l.DB.QueryContext(ctx, s, i...)
	logQuery(s, start, err)
	return rows, err
}

func (l DBQ) QueryRowContext(ctx context.Context, s string, i ...interface{}) *sql.Row {
	start := time.Now()
	row := l.DB.QueryRowContext(ctx, s, i...)
	logQuery(s, start, nil)
	return row
}

func logQuery(s string, start time.Time, err error) {
	queryName, _, _ := strings.Cut(s, "\n")
	queryName = strings.TrimPrefix(queryName, "-- name: ")
	queryName = strings.Fields(queryName)[0]
	durationMS := float64(time.Since(start).Microseconds()) / 1000.0

	// Note that the duration(ish) is very misleading. It'll always be less than
	// the actual query time, often significantly. That's because most of the IO
	// takes place after we're able to log in this file, e.g. in the "for rows.Next()"
	// part of reading the results, and unfortunately that code is in the generated
	// sqlc package. It's a TODO for later to log actual query times.
	slog.Debug("Ran IMS SQL: "+queryName,
		"durationish", fmt.Sprintf("%.3fms", durationMS),
		"err", err,
	)
}

// TODO: possibly do this sort of caching
//  where DBQ has a "eventsCache *cache.InMemory[[]imsdb.EventsRow]" field
// func (l DBQ) Events(ctx context.Context, db imsdb.DBTX) ([]imsdb.EventsRow, error) {
//	rows, err := l.eventsCache.Get(ctx)
//	return orNil(rows), err
//}
//
// func orNil[S ~*[]E, E any](sl S) []E {
//	if sl == nil {
//		return nil
//	}
//	return *sl
//}
