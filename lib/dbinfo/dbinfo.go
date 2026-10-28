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

// Package dbinfo describes a MariaDB connection pool and the server at the
// other end of it, for the debugging page.
package dbinfo

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// sessionVariables are the server and session variables Summary reports, in
// the order it reports them.
var sessionVariables = []string{
	"version_comment",
	"version_compile_os",
	"version_compile_machine",
	"hostname",
	"port",
	"read_only",
	"time_zone",
	"system_time_zone",
	"sql_mode",
	"autocommit",
	"transaction_isolation",
	"character_set_client",
	"character_set_connection",
	"character_set_results",
	"collation_connection",
	"character_set_server",
	"collation_server",
	"max_allowed_packet",
	"max_statement_time",
	"wait_timeout",
	"net_read_timeout",
	"net_write_timeout",
	"lock_wait_timeout",
	"innodb_lock_wait_timeout",
	"innodb_buffer_pool_size",
	"have_ssl",
	"require_secure_transport",
}

var sessionStatus = []string{
	"Ssl_version",
	"Ssl_cipher",
}

var globalStatus = []string{
	"Uptime",
	"Threads_connected",
	"Threads_running",
	"Max_used_connections",
	"Max_used_connections_time",
	"max_connections",
	"Connections",
	"Aborted_connects",
	"Aborted_clients",
	"Questions",
	"Slow_queries",
}

type printer struct {
	strings.Builder
}

func (p *printer) printf(format string, a ...any) {
	_, _ = fmt.Fprintf(p, format, a...)
}

func (p *printer) field(name string, value any) {
	p.printf("    %-28s %v\n", name+":", value)
}

// Summary describes db: the driver config it was opened with (cfg, whose
// password is never printed), its pool settings and stats, and a selection of
// what the server reports about itself and one of the pool's connections.
func Summary(ctx context.Context, db *sql.DB, cfg *mysql.Config, connMaxLifetime time.Duration) string {
	var p printer

	p.printf("Client driver config\n")
	p.field("Net", cfg.Net)
	p.field("Addr", cfg.Addr)
	p.field("DBName", cfg.DBName)
	p.field("User", cfg.User)
	p.field("TLS", orDefault(cfg.TLSConfig, "none"))
	p.field("Collation", orDefault(cfg.Collation, "(driver default)"))
	p.field("Loc", cfg.Loc)
	p.field("ParseTime", cfg.ParseTime)
	p.field("MultiStatements", cfg.MultiStatements)
	p.field("InterpolateParams", cfg.InterpolateParams)
	p.field("CheckConnLiveness", cfg.CheckConnLiveness)
	p.field("MaxAllowedPacket", cfg.MaxAllowedPacket)
	p.field("Timeout", cfg.Timeout)
	p.field("ReadTimeout", cfg.ReadTimeout)
	p.field("WriteTimeout", cfg.WriteTimeout)

	stats := db.Stats()
	p.printf("\nClient connection pool\n")
	p.field("MaxOpenConnections", stats.MaxOpenConnections)
	p.field("ConnMaxLifetime", connMaxLifetime)
	p.field("OpenConnections", stats.OpenConnections)
	p.field("InUse", stats.InUse)
	p.field("Idle", stats.Idle)
	p.field("WaitCount", stats.WaitCount)
	p.field("WaitDuration", stats.WaitDuration)
	p.field("MaxIdleClosed", stats.MaxIdleClosed)
	p.field("MaxIdleTimeClosed", stats.MaxIdleTimeClosed)
	p.field("MaxLifetimeClosed", stats.MaxLifetimeClosed)

	// Pin one connection, so that the session values all describe the same one.
	conn, err := db.Conn(ctx)
	if err != nil {
		p.printf("\nFailed to get a connection to the server: %v\n", err)
		return p.String()
	}
	defer func() { _ = conn.Close() }()

	p.printf("\nServer identity\n")
	var version, currentUser, user string
	var database sql.NullString
	var connectionID int64
	err = conn.QueryRowContext(ctx, "SELECT VERSION(), CURRENT_USER(), USER(), DATABASE(), CONNECTION_ID()").
		Scan(&version, &currentUser, &user, &database, &connectionID)
	if err != nil {
		p.printf("    Failed to query server identity: %v\n", err)
	} else {
		p.field("VERSION()", version)
		p.field("CURRENT_USER()", currentUser)
		// USER() includes the client's host as the server sees it, which can
		// differ from ours behind a proxy or NAT.
		p.field("USER()", user)
		p.field("DATABASE()", orDefault(database.String, "(none)"))
		p.field("CONNECTION_ID()", connectionID)
	}

	p.printf("\nServer and session variables (for this connection)\n")
	vars, err := nameValues(ctx, conn, "SHOW SESSION VARIABLES")
	p.selected("SHOW SESSION VARIABLES", vars, err, sessionVariables, nil)

	p.printf("\nConnection encryption (for this connection)\n")
	status, err := nameValues(ctx, conn, "SHOW SESSION STATUS")
	p.selected("SHOW SESSION STATUS", status, err, sessionStatus, func(name, value string) string {
		if value == "" {
			return "(not encrypted)"
		}
		return value
	})

	p.printf("\nServer load\n")
	status, err = nameValues(ctx, conn, "SHOW GLOBAL STATUS")
	// max_connections is a variable rather than a status value, but it's what
	// the connection counts need to be read against.
	if maxConns, ok := vars["max_connections"]; ok && err == nil {
		status["max_connections"] = maxConns
	}
	p.selected("SHOW GLOBAL STATUS", status, err, globalStatus, func(name, value string) string {
		if name == "Uptime" {
			secs, err := strconv.ParseInt(value, 10, 64)
			if err == nil {
				return fmt.Sprintf("%v (%v s)", time.Duration(secs)*time.Second, value)
			}
		}
		return value
	})
	return p.String()
}

// selected prints the named values from the result of a SHOW statement, in
// the order given. format, when non-nil, rewrites each value before it's printed.
func (p *printer) selected(show string, values map[string]string, err error, names []string, format func(name, value string) string) {
	if err != nil {
		p.printf("    Failed to run %v: %v\n", show, err)
		return
	}
	for _, name := range names {
		value, ok := values[name]
		if !ok {
			p.field(name, "(not reported)")
			continue
		}
		if format != nil {
			value = format(name, value)
		}
		p.field(name, value)
	}
}

// All lists every session variable and global status value the server reports.
func All(ctx context.Context, db *sql.DB) string {
	var p printer
	conn, err := db.Conn(ctx)
	if err != nil {
		p.printf("Failed to get a connection to the server: %v\n", err)
		return p.String()
	}
	defer func() { _ = conn.Close() }()

	for _, show := range []string{"SHOW SESSION VARIABLES", "SHOW GLOBAL STATUS"} {
		p.printf("%v\n", show)
		rows, err := nameValueRows(ctx, conn, show)
		if err != nil {
			p.printf("    Failed: %v\n", err)
		}
		for _, row := range rows {
			p.printf("    %v = %v\n", row[0], row[1])
		}
		p.printf("\n")
	}
	return p.String()
}

func nameValues(ctx context.Context, conn *sql.Conn, show string) (map[string]string, error) {
	rows, err := nameValueRows(ctx, conn, show)
	if err != nil {
		return nil, err
	}
	values := make(map[string]string, len(rows))
	for _, row := range rows {
		values[row[0]] = row[1]
	}
	return values, nil
}

func nameValueRows(ctx context.Context, conn *sql.Conn, show string) ([][2]string, error) {
	rows, err := conn.QueryContext(ctx, show)
	if err != nil {
		return nil, fmt.Errorf("[QueryContext]: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var result [][2]string
	for rows.Next() {
		var name string
		var value sql.NullString
		err = rows.Scan(&name, &value)
		if err != nil {
			return nil, fmt.Errorf("[Scan]: %w", err)
		}
		result = append(result, [2]string{name, value.String})
	}
	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("[rows.Err]: %w", err)
	}
	return result, nil
}

func orDefault(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
