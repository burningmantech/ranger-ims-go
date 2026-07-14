# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Ranger IMS is an Incident Management System for the Black Rock Rangers, used to track incidents at Black Rock City. This is a Go implementation that replaced a previous Python version.

The main domain entities, all scoped to an Event (e.g. a burn year): **Incidents**, **Field Reports** (attachable to an Incident), **Sanctuary Visits** (guest visits to the Sanctuary space, also attachable to an Incident), and **Places** (camps/art/etc. used for locations).

## Development Commands

### Initial Setup

Generated code and external build dependencies aren't checked in, so a fresh clone won't compile until you produce them:
```bash
make generate
# or
go run bin/build/build.go -generate-only
```

### Building

Build the server (runs sqlc, templ, and tsgo code generation, then compiles):
```bash
go run bin/build/build.go
# or
make build
```

The build outputs a `ranger-ims-go` binary in the project root.

**Always use the build script for code generation.** Do not run individual generators (templ, tsgo, sqlc) separately — use `go run bin/build/build.go` or `make build` instead, as the script runs all generators in the correct order.

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

Run with Docker Compose (includes auto-seeded databases — IMS data from
`store/fakeimsdb/seed.sql`, Clubhouse users from `directory/fakeclubhousedb/seed.sql`):
```bash
make compose/live
```
The compose make targets copy `.env.dev.example` / `.env.quickstart.example` to
`.env.dev` / `.env.quickstart` (both gitignored) on first run and pass them via
`--env-file`, so the stacks read their own env files instead of the `./.env`
used when running `ims serve` directly — the two configs can't collide.

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

Run TypeScript tests (Vitest, in `web/typescripttest/`; requires `npm install`):
```bash
npm test
# or
make test/ts
```
These run against templ-rendered HTML fixtures, which are regenerated automatically
by the `pretest` script (`bin/rendertestfixtures`) into the gitignored
`web/typescripttest/fixtures/` directory.

Run Playwright browser tests. The Playwright config's `webServer` reuses an
IMS stack already serving on :8080 (e.g. from `make compose/live`) or starts
the compose stack itself, tearing it down afterward:
```bash
make test/e2e
# or, with deps already installed:
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

Go linting runs via pre-commit (golangci-lint configured in `.golangci.yml`, plus `go vet`, `go fmt`, `go mod tidy`, govulncheck, and license-header checks):
```bash
uvx pre-commit run --all-files
```

JavaScript/TypeScript linting:
```bash
npx eslint
```

All source files require the Apache 2.0 license header (see any `.go` file). Add headers to new files automatically with:
```bash
go run bin/prependlicense/prependlicense.go
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
- **`cmd/`** - Cobra CLI commands (`serve`, `healthcheck`, `hash_password`, `add-user`)
- **`api/`** - HTTP API handlers and middleware for the REST/JSON API
- **`web/`** - Web UI handlers, templates (templ), TypeScript, and static assets
- **`store/`** - Database layer for the IMS database (incidents, field reports, etc.)
- **`directory/`** - User directory layer (Clubhouse DB or fake user store)
- **`lib/`** - Reusable utilities (auth, logging, caching, formatting, etc.)
- **`json/`** - JSON serialization types for the API

### Database Architecture

The system uses up to **two separate databases**:

1. **IMS Database** (`store/` package) - Stores incident data, field reports, etc., plus the
   IMS-native user directory tables (`DIRECTORY_*`), which are used when `IMS_DIRECTORY=ims`
2. **Directory Database** (`directory/` package) - User/personnel data from a Ranger Clubhouse
   MariaDB, used when `IMS_DIRECTORY=clubhousedb`. Not needed at all when `IMS_DIRECTORY=ims`.

Both use **sqlc** for type-safe SQL code generation:
- SQL schemas: `store/schema/current.sql` and `directory/schema/current.sql`
- SQL queries: `store/queries.sql` and `directory/queries.sql`
- Generated Go code: `store/imsdb/` and `directory/clubhousedb/`

The `directory.UserStore` caches user data fetched from a `directory.Source`, which has two
implementations: `ClubhouseSource` (Clubhouse DB) and `IMSSource` (the `DIRECTORY_*` tables in
the IMS DB). The IMS-native directory is administered via the `/ims/api/directory` endpoints
and the `/ims/app/admin/directory` web page; the `add-user` CLI command bootstraps the first
user on a fresh deployment.

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
- **Directory types**: `clubhousedb` (real Clubhouse MariaDB; the docker-compose dev setup seeds one
  from `directory/fakeclubhousedb/seed.sql`), `ims` (IMS-native directory tables in the IMS database),
  or `noop` (testing only)
- **DB store types**: `MariaDB` (persistent storage) or `noop` (no-op for testing only)
- **Attachments stores**: `local` (filesystem) or `s3` (AWS S3)

### API Structure

The API (`api/` package) uses a custom middleware adapter pattern:
- `AddToMux()` in `api/mux.go` registers all routes
- Handlers implement a specific interface and are wrapped with middleware adapters
- Middleware includes: authentication, logging, panic recovery, request size limits
- Handlers return `*herr.HTTPError` (`lib/herr/`), which carries both an internal error and a client-safe response message
- Real-time updates are pushed to clients via Server-Sent Events (`api/eventsource.go`); mutations publish events that the web UI listens for
- Incidents, Field Reports, and Visits use optimistic concurrency: each row carries a `VERSION` counter, surfaced to clients as a strong ETag; writes send `If-Match` and get a 412 on version mismatch (see `parseIfMatch`/`setETag` in `api/helpers.go`)
- Cross-event search (`api/search.go`) matches a substring or regex query against Incidents, Field Reports, and Visits across all events the requestor can read

### Web UI

The web UI uses:
- **templ** - Type-safe Go templates (`.templ` files generate `.go` files)
- **TypeScript** - Compiled to JavaScript via tsgo (in `web/typescript/`, output to `web/static/`)
- Custom mux pattern similar to the API

### Testing Strategy

- **Unit tests**: Throughout the codebase using `testify` assertions
- **Integration tests**: `store/integration/` and `api/integration/` use testcontainers to spin up real MariaDB
- **TypeScript tests**: Vitest tests in `web/typescripttest/` (run with `npm test`), exercising the real TypeScript against templ-rendered HTML fixtures in happy-dom, with fetch/EventSource mocked
- **Playwright tests**: Browser automation tests in `playwright/tests/`

### Code Generation Tools

The project uses several code generators (all invoked by the build script):

1. **sqlc** - Generates type-safe Go code from SQL
2. **templ** - Compiles `.templ` templates to Go code
3. **tsgo** - TypeScript compiler wrapper that transpiles to JavaScript

**Generated code is not checked in.** These paths are all gitignored and produced by the generators:

- `web/template/*_templ.go` — from the `.templ` files alongside them
- `store/imsdb/` and `directory/clubhousedb/` — from `*/queries.sql` + `*/schema/current.sql`
- `web/static/*.js` — from `web/typescript/*.ts`
- `web/static/ext/` — third-party client libs, fetched by `bin/fetchbuilddeps`

Never hand-edit them; edit the source (`.templ`, `.sql`, `.ts`) and regenerate. A fresh clone won't compile until you do — `go test ./...`, `go vet`, and gopls all need the generated code to exist. Run `make generate` (generators only) or `make build` (generators + `go build`) first. CI, the Dockerfile, and `make run/live` all run the generators themselves, so there is never anything to commit.

Because `tsgo` is the TypeScript type checker, a type error in `web/typescript/` fails `make generate` and fails CI.

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
- Per-event roles (`EventReporter`, `EventReader`, `EventWriter`, `EventVisitWriter`) map to bitmask permissions (`EventPermissionMask`) covering incidents, field reports, visits, and places
- Global (non-event) permissions (`GlobalPermissionMask`) cover listing events, reading personnel/incident types, and the admin pages
- Admins (defined in `IMS_ADMINS`) have unrestricted access

### Action Logging

All authenticated API requests are logged to an action log (`store/actionlog/`) for audit purposes.

## Key Differences from Python Version

- No SQLite support (MariaDB only for persistent storage)
- Uses `.env` file instead of `conf/imsd.conf`
- "File" directory type renamed to "TestUsers" and implemented as compiled Go code
- Heavy use of sqlc code generation instead of ORM
