package noopdb

import (
	"database/sql"
	"database/sql/driver"
)

var d *Driver

func init() {
	d = &Driver{}
	sql.Register("noop", d)
}

type Driver struct{}

type Conn struct{}

type Stmt struct{}

type Result struct{}

type Rows struct{}

type Tx struct{}

func (d Driver) Open(_ string) (driver.Conn, error) {
	return Conn{}, nil
}

func (c Conn) Prepare(query string) (driver.Stmt, error) {
	return Stmt{}, nil
}

func (c Conn) Close() error {
	return nil
}

func (c Conn) Begin() (driver.Tx, error) {
	return Tx{}, nil
}

func (s Stmt) Close() error {
	return nil
}

func (s Stmt) NumInput() int {
	return 0
}

func (s Stmt) Exec(args []driver.Value) (driver.Result, error) {
	return Result{}, nil
}

func (s Stmt) Query(args []driver.Value) (driver.Rows, error) {
	return Rows{}, nil
}

func (r Result) LastInsertId() (int64, error) {
	return 0, nil
}

func (r Result) RowsAffected() (int64, error) {
	return 0, nil
}

func (r Rows) Columns() []string {
	return nil
}

func (r Rows) Close() error {
	return nil
}

func (r Rows) Next(dest []driver.Value) error {
	return nil
}

func (t Tx) Commit() error {
	return nil
}

func (t Tx) Rollback() error {
	return nil
}
