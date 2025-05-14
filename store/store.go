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
	"embed"
	"fmt"
	"github.com/burningmantech/ranger-ims-go/conf"
	_ "github.com/burningmantech/ranger-ims-go/lib/noopdb"
	"github.com/go-sql-driver/mysql"
	"log/slog"
)

const (
	MariaDBVersion     = "10.5.27"
	MariaDBDockerImage = "mariadb:" + MariaDBVersion
)

//go:embed schema/current.sql
var CurrentSchema string

//go:embed schema/*-from-*.sql
var Migrations embed.FS

func SqlDB(ctx context.Context, dbStoreCfg conf.DBStore, migrateDB bool) (*sql.DB, error) {
	if dbStoreCfg.Type == conf.DBStoreTypeNoOp {
		// This is a DB that does nothing and returns nothing on querying.
		// It's really only useful as a stand-in for testing.
		slog.Info("Using NoOp DB")
		return sql.Open("noop", "")
	}
	slog.Info("Setting up IMS DB connection")
	mariaCfg := dbStoreCfg.MariaDB

	// Capture connection properties.
	cfg := mysql.NewConfig()
	cfg.User = mariaCfg.Username
	cfg.Passwd = mariaCfg.Password
	cfg.Net = "tcp"
	cfg.Addr = fmt.Sprintf("%v:%v", mariaCfg.HostName, mariaCfg.HostPort)
	cfg.DBName = mariaCfg.Database

	// Get a database handle.
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("[sql.Open]: %ws", err)
	}
	// Some arbitrary value. We'll get errors from MariaDB if the server
	// hits the DB with too many parallel requests.
	db.SetMaxOpenConns(20)
	pingErr := db.PingContext(ctx)
	if pingErr != nil {
		return nil, fmt.Errorf("[db.PingContext]: %ws", pingErr)
	}

	if migrateDB {
		if err = MigrateDB(ctx, db); err != nil {
			return nil, fmt.Errorf("[MigrateDB]: %w", err)
		}
	} else {
		slog.Info("IMS DB migration not requested")
	}

	slog.Info("Connected to IMS MariaDB")
	return db, nil
}
