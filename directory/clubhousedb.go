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
	clubhousequeries "github.com/burningmantech/ranger-ims-go/directory/clubhousedb"
	"github.com/go-sql-driver/mysql"
	"log/slog"
	"strings"
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

type DB struct {
	*sql.DB
	clubhousequeries.Querier
}

func (l DB) ExecContext(ctx context.Context, s string, i ...interface{}) (sql.Result, error) {
	start := time.Now()
	execContext, err := l.DB.ExecContext(ctx, s, i...)
	logQuery(s, start, err)
	return execContext, err
}

func (l DB) PrepareContext(ctx context.Context, s string) (*sql.Stmt, error) {
	start := time.Now()
	stmt, err := l.DB.PrepareContext(ctx, s)
	logQuery(s, start, err)
	return stmt, err
}

func (l DB) QueryContext(ctx context.Context, s string, i ...interface{}) (*sql.Rows, error) {
	start := time.Now()
	rows, err := l.DB.QueryContext(ctx, s, i...)
	logQuery(s, start, err)
	return rows, err
}

func (l DB) QueryRowContext(ctx context.Context, s string, i ...interface{}) *sql.Row {
	start := time.Now()
	row := l.DB.QueryRowContext(ctx, s, i...)
	logQuery(s, start, nil)
	return row
}

func logQuery(s string, start time.Time, err error) {
	queryName, _, _ := strings.Cut(s, "\n")
	queryName = strings.TrimPrefix(queryName, "-- name: ")
	slog.Debug("DoneCH", "query", queryName, "ms", time.Since(start).Milliseconds(), "err", err)
}
