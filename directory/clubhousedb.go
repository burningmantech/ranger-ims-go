package directory

import (
	"context"
	"database/sql"
	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/go-sql-driver/mysql"
	"log/slog"
	"os"
	"strings"
	"time"
)

func MariaDB(imsCfg *conf.IMSConfig) *DB {
	slog.Info("Setting up Clubhouse DB connection")

	// Capture connection properties.
	cfg := mysql.NewConfig()
	cfg.User = imsCfg.Directory.ClubhouseDB.Username
	cfg.Passwd = imsCfg.Directory.ClubhouseDB.Password
	cfg.Net = "tcp"
	cfg.Addr = imsCfg.Directory.ClubhouseDB.Hostname
	cfg.DBName = imsCfg.Directory.ClubhouseDB.Database

	// Get a database handle.
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		slog.Error("Failed to open Clubhouse DB connection", "error", err)
		os.Exit(1)
	}
	// Some arbitrary value. We'll get errors from MariaDB if the server
	// hits the DB with too many parallel requests.
	db.SetMaxOpenConns(20)
	pingErr := db.Ping()
	if pingErr != nil {
		slog.Error("Failed ping attempt to Clubhouse DB", "error", pingErr)
		os.Exit(1)
	}
	slog.Info("Connected to Clubhouse MariaDB")
	return &DB{DB: db}
}

type DB struct {
	*sql.DB
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
