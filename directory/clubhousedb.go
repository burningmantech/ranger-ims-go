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

package directory

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/burningmantech/ranger-ims-go/conf"
	chqueries "github.com/burningmantech/ranger-ims-go/directory/clubhousedb"
	"github.com/burningmantech/ranger-ims-go/lib/cache"
	"github.com/go-sql-driver/mysql"
	"log/slog"
	"time"
)

func MariaDB(ctx context.Context, imsCfg conf.ClubhouseDB) (*sql.DB, error) {
	slog.Info("Setting up Clubhouse DB connection")

	// Capture connection properties.
	cfg := mysql.NewConfig()
	cfg.User = imsCfg.Username
	cfg.Passwd = imsCfg.Password
	cfg.Net = "tcp"
	cfg.Addr = imsCfg.Hostname
	cfg.DBName = imsCfg.Database

	// Get a database handle.
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("[sql.Open]: %w", err)
	}
	// Some arbitrary value. We'll get errors from MariaDB if the server
	// hits the DB with too many parallel requests.
	db.SetMaxOpenConns(20)
	pingErr := db.PingContext(ctx)
	if pingErr != nil {
		return nil, fmt.Errorf("[PingContext]: %w", pingErr)
	}
	slog.Info("Connected to Clubhouse MariaDB")
	return db, nil
}

// DBQ combines the SQL database and the Querier for the Clubhouse datastore.
type DBQ struct {
	*sql.DB
	q chqueries.Querier

	rangersByIdCache     *cache.InMemory[[]chqueries.RangersByIdRow]
	personPositionsCache *cache.InMemory[[]chqueries.PersonPosition]
	personTeamsCache     *cache.InMemory[[]chqueries.PersonTeamsRow]
	positionsCache       *cache.InMemory[[]chqueries.PositionsRow]
	teamsCache           *cache.InMemory[[]chqueries.TeamsRow]
}

var _ chqueries.Querier = (*DBQ)(nil)

// NewMariaDBQ creates a DBQ that uses a standard MariaDB Clubhouse database.
func NewMariaDBQ(db *sql.DB, cacheTTL time.Duration) *DBQ {
	return newDBQ(db, chqueries.New(), cacheTTL)
}

// NewTestUsersDBQ creates a DBQ that isn't actually connected to any database,
// but rather just returns values from a TestUsersStore. This actually sets the
// db to "nil", which is intentional, because it means we'll see a panic if
// something erroneously tries to talk to a DB rather than the TestUsersStore.
func NewTestUsersDBQ(testUsers []conf.TestUser, cacheTTL time.Duration) *DBQ {
	return newDBQ(nil, TestUsersStore{TestUsers: testUsers}, cacheTTL)
}

func newDBQ(db *sql.DB, querier chqueries.Querier, cacheTTL time.Duration) *DBQ {
	dbq := &DBQ{
		DB: db,
		q:  querier,
	}
	dbq.rangersByIdCache = cachemaker(dbq, cacheTTL, "RangersById", dbq.q.RangersById)
	dbq.personPositionsCache = cachemaker(dbq, cacheTTL, "PersonPositions", dbq.q.PersonPositions)
	dbq.personTeamsCache = cachemaker(dbq, cacheTTL, "PersonTeams", dbq.q.PersonTeams)
	dbq.positionsCache = cachemaker(dbq, cacheTTL, "Positions", dbq.q.Positions)
	dbq.teamsCache = cachemaker(dbq, cacheTTL, "Teams", dbq.q.Teams)
	return dbq
}

// cachemaker cachemaker makes me a cache.
func cachemaker[T any](
	dbtx chqueries.DBTX,
	valTTL time.Duration,
	queryName string,
	refresher func(context.Context, chqueries.DBTX) (T, error),
) *cache.InMemory[T] {
	return cache.New[T](
		valTTL,
		func(ctx context.Context) (T, error) {
			start := time.Now()
			t, err := refresher(ctx, dbtx)
			logQuery(queryName, start, err)
			return t, err
		},
	)
}

func logQuery(queryName string, start time.Time, err error) {
	durationMS := float64(time.Since(start).Microseconds()) / 1000.0
	slog.Debug("Ran Clubhouse SQL: "+queryName,
		"durationish", fmt.Sprintf("%.3fms", durationMS),
		"err", err,
	)
}

func (l DBQ) PersonPositions(ctx context.Context, db chqueries.DBTX) ([]chqueries.PersonPosition, error) {
	rows, err := l.personPositionsCache.Get(ctx)
	return orNil(rows), err
}

func (l DBQ) PersonTeams(ctx context.Context, db chqueries.DBTX) ([]chqueries.PersonTeamsRow, error) {
	rows, err := l.personTeamsCache.Get(ctx)
	return orNil(rows), err
}

func (l DBQ) Positions(ctx context.Context, db chqueries.DBTX) ([]chqueries.PositionsRow, error) {
	rows, err := l.positionsCache.Get(ctx)
	return orNil(rows), err
}

func (l DBQ) RangersById(ctx context.Context, db chqueries.DBTX) ([]chqueries.RangersByIdRow, error) {
	rows, err := l.rangersByIdCache.Get(ctx)
	return orNil(rows), err
}

func (l DBQ) Teams(ctx context.Context, db chqueries.DBTX) ([]chqueries.TeamsRow, error) {
	rows, err := l.teamsCache.Get(ctx)
	return orNil(rows), err
}

// orNil takes something like *[]string and returns it as either []string or nil.
func orNil[S ~*[]E, E any](sl S) []E {
	if sl == nil {
		return nil
	}
	return *sl
}
