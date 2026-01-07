# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Ranger IMS is an Incident Management System for the Black Rock Rangers, used to track incidents at Black Rock City. This is a Go implementation that replaced a previous Python version.

## Development Commands

### Initial Setup

First-time setup to fetch external build dependencies:
```bash
go run bin/fetchbuilddeps/fetchbuilddeps.go
```

### Building

Build the server (runs sqlc, templ, and tsgo code generation, then compiles):
```bash
go run bin/build/build.go
# or
make build
```

The build outputs a `ranger-ims-go` binary in the project root.

### Running the Server

Run with MariaDB (requires `.env` file configuration):
```bash
./ranger-ims-go serve
```

Run with live reloading using air:
```bash
make run/live
# or
go tool air
```

Run with Docker Compose (includes auto-seeded databases):
```bash
docker compose -f docker-compose.dev.yml up
```

### Testing

Run all Go tests:
```bash
go test ./...
# or
make test
```

Run tests with coverage report:
```bash
go test -coverprofile=coverage.out --coverpkg ./... ./... && go tool cover -html=coverage.out
# or
make cover
```

Run integration tests (requires Docker):
```bash
go test ./store/integration
go test ./api/integration
```

Run Playwright browser tests (requires Playwright installed):
```bash
cd playwright
npx playwright test
```

### Code Generation

The build script runs all code generators, but you can run them individually:

```bash
# Generate sqlc code from SQL schemas and queries
go tool sqlc generate

# Generate templ code (Go HTML templates)
go tool templ generate

# Generate JavaScript from TypeScript
go tool tsgo
```

### Linting

JavaScript/TypeScript linting:
```bash
npx eslint
```

### Running a Single Test

```bash
go test ./path/to/package -run TestName
```

### Upgrading Dependencies

Upgrade Go dependencies:
```bash
go get -t -u ./...
go mod tidy
# or
make upgrade-deps
```

## Architecture

### High-Level Structure

The codebase follows a layered architecture:

- **`main.go`** - Entry point that delegates to `cmd` package
- **`cmd/`** - Cobra CLI commands (`serve`, `healthcheck`, `hashpassword`)
- **`api/`** - HTTP API handlers and middleware for the REST/JSON API
- **`web/`** - Web UI handlers, templates (templ), TypeScript, and static assets
- **`store/`** - Database layer for the IMS database (incidents, field reports, etc.)
- **`directory/`** - User directory layer (Clubhouse DB or fake user store)
- **`lib/`** - Reusable utilities (auth, logging, caching, formatting, etc.)
- **`json/`** - JSON serialization types for the API

### Database Architecture

The system uses **two separate databases**:

1. **IMS Database** (`store/` package) - Stores incident data, field reports, etc.
2. **Directory Database** (`directory/` package) - User/personnel data (either ClubhouseDB or fake)

Both use **sqlc** for type-safe SQL code generation:
- SQL schemas: `store/schema/current.sql` and `directory/schema/current.sql`
- SQL queries: `store/queries.sql` and `directory/queries.sql`
- Generated Go code: `store/imsdb/` and `directory/clubhousedb/`

### Database Migrations

To modify the IMS database schema:

1. Create a new migration file in `store/schema/` following the pattern `XX-from-YY.sql`
2. Update the schema version in the migration:
   ```sql
   update `SCHEMA_INFO` set `VERSION` = XX;
   ```
3. Apply the same changes to `store/schema/current.sql` (update version there too)
4. Run the migration test: `go test ./store/integration`
5. Regenerate sqlc code: `go tool sqlc generate`
6. Update `store/queries.sql` if you modified existing tables/columns
7. Fix any broken Go code and run `go test ./...`

### Configuration

Configuration uses environment variables loaded from a `.env` file (copy from `.env.example`).

Key configuration concepts:
- **Directory types**: `fake` (test users from `directory/fakeclubhousedb/seed.sql`) or `ClubhouseDB` (real MariaDB)
- **DB store types**: `MariaDB` (persistent storage) or `noop` (no-op for testing only)
- **Attachments stores**: `local` (filesystem) or `s3` (AWS S3)

### API Structure

The API (`api/` package) uses a custom middleware adapter pattern:
- `AddToMux()` in `api/mux.go` registers all routes
- Handlers implement a specific interface and are wrapped with middleware adapters
- Middleware includes: authentication, logging, panic recovery, request size limits

### Web UI

The web UI uses:
- **templ** - Type-safe Go templates (`.templ` files generate `.go` files)
- **TypeScript** - Compiled to JavaScript via tsgo (in `web/typescript/`, output to `web/static/`)
- Custom mux pattern similar to the API

### Testing Strategy

- **Unit tests**: Throughout the codebase using `testify` assertions
- **Integration tests**: `store/integration/` and `api/integration/` use testcontainers to spin up real MariaDB
- **Playwright tests**: Browser automation tests in `playwright/tests/`

### Code Generation Tools

The project uses several code generators (all invoked by the build script):

1. **sqlc** - Generates type-safe Go code from SQL
2. **templ** - Compiles `.templ` templates to Go code
3. **tsgo** - TypeScript compiler wrapper that transpiles to JavaScript

## Development Patterns

### Directory/User Store Pattern

The `directory.UserStore` provides user lookups with caching. It abstracts over either:
- Real ClubhouseDB (production personnel database)
- Fake ClubhouseDB (seeded test data for local dev)

### Store Pattern

The `store.DBQ` wraps a `*sql.DB` and sqlc-generated `Querier` interface, providing database access throughout the application.

### Authentication

JWT-based authentication with separate access and refresh tokens:
- Access tokens: Short-lived (default 15 min)
- Refresh tokens: Long-lived (default 7 days)
- Tokens signed with `IMS_JWT_SECRET`

### Authorization

Event-based access control defined in `lib/authz/`:
- Users have specific access modes per event (read, write, report)
- Admins (defined in `IMS_ADMINS`) have unrestricted access

### Action Logging

All authenticated API requests are logged to an action log (`store/actionlog/`) for audit purposes.

## Key Differences from Python Version

- No SQLite support (MariaDB only for persistent storage)
- Uses `.env` file instead of `conf/imsd.conf`
- "File" directory type renamed to "TestUsers" and implemented as compiled Go code
- Heavy use of sqlc code generation instead of ORM
