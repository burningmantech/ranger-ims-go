# AGENTS.md

Ranger IMS is the Incident Management System for the Black Rock Rangers, used to track incidents at
Black Rock City. It's a Go server (replacing an older Python version) with a templ + TypeScript web UI
and a MariaDB backend.

The main domain entities, all scoped to an Event (e.g. a burn year): **Incidents**, **Field Reports**
(attachable to an Incident), **Sanctuary Visits** (guest visits to the Sanctuary space, also attachable
to an Incident), and **Places** (camps, art, etc., used for locations).

## Setup

Generated code and third-party client libs aren't checked in, so a fresh clone won't compile (and
`go test`, `go vet`, and gopls won't work) until you generate them:

```bash
make generate   # generators only
make build      # generators + go build, producing ./ranger-ims-go
```

Always generate through the build script (`make generate`/`make build`, or
`go run bin/build/build.go [-generate-only]`). Don't run sqlc, templ, or tsc on their own; the script
runs them in the right order.

Configuration is environment variables from `.env` (copy `.env.example`).

## Running

```bash
make compose/live   # Docker Compose with seeded IMS and Clubhouse DBs, on :8080
make run/live       # live reload with air, against your .env
./ranger-ims-go serve
```

The compose targets create and use `.env.dev` / `.env.quickstart` (gitignored, copied from the
`.example` files), so they never collide with the `.env` used by `serve`. The seed data is
`store/fakeimsdb/seed.sql` and `directory/fakeclubhousedb/seed.sql`.

## Testing

```bash
make test                                 # all Go tests (go test ./...)
go test ./path/to/package -run TestName   # a single test
go test ./store/integration ./api/integration   # integration tests; need Docker
make cover                                # Go coverage report
make test/ts                              # Vitest (npm test)
make test/e2e                             # Playwright
```

- **Go unit tests** use `testify`. Prefer explicit, unrolled cases over table-driven tests.
- **Integration tests** (`store/integration/`, `api/integration/`) run against a real MariaDB via
  testcontainers.
- **TypeScript tests** (`web/typescripttest/`) run the real TypeScript in happy-dom against
  templ-rendered HTML fixtures, with fetch/EventSource mocked. The `pretest` script regenerates the
  fixtures into the gitignored `web/typescripttest/fixtures/`.
- **Playwright tests** (`playwright/tests/`) reuse a stack already serving on :8080, or start the
  compose stack and tear it down afterward.
- Vitest and Playwright run the browser in Europe/Berlin to catch time zone leaks (see Web UI below).

## Linting and code style

```bash
uvx pre-commit run --all-files   # golangci-lint, go vet/fmt, go mod tidy, govulncheck, license headers
npx eslint                        # TypeScript
```

- Every source file needs the Apache 2.0 license header. Add it to new files with
  `go run bin/prependlicense/prependlicense.go`.
- `tsc` is the TypeScript type checker, so a type error in `web/typescript/` fails `make generate` and CI.

## Generated code

Never hand-edit these gitignored outputs; edit the source and regenerate. CI, the Dockerfile, and
`make run/live` all generate their own, so there's never anything to commit.

| Output | Source |
| --- | --- |
| `store/imsdb/`, `directory/clubhousedb/` (sqlc) | `*/queries.sql` + `*/schema/current.sql` |
| `web/template/*_templ.go` (templ) | the `.templ` files beside them |
| `web/static/*.js` (tsc) | `web/typescript/*.ts` |
| `web/static/ext/` | third-party libs fetched by `bin/fetchbuilddeps` |

## Project layout

- `main.go` → `cmd/`: Cobra commands (`serve`, `healthcheck`, `hash_password`, `add-user`)
- `api/`: REST/JSON API handlers and middleware; routes registered in `AddToMux()` in `api/mux.go`
- `web/`: web UI handlers, templ templates, TypeScript, static assets
- `store/`: IMS database layer (`store.DBQ` wraps the `*sql.DB` and the sqlc `Querier`)
- `directory/`: user directory layer
- `json/`: API serialization types
- `lib/`: shared utilities (auth, logging, caching, formatting, `herr` errors, etc.)

## Databases and migrations

There are up to two MariaDB databases, both accessed through sqlc:

1. **IMS database** (`store/`): incidents, field reports, etc., plus the IMS-native user directory
   tables (`DIRECTORY_*`).
2. **Clubhouse database** (`directory/`): Ranger personnel, used only when `IMS_DIRECTORY=clubhousedb`.

To change the IMS schema:

1. Add a migration `store/schema/XX-from-YY.sql` that ends with
   ``update `SCHEMA_INFO` set `VERSION` = XX;``
2. Make the same changes, and version bump, in `store/schema/current.sql`.
3. Run `go test ./store/integration`, which checks that the migrations match `current.sql`.
4. Update `store/queries.sql` as needed, `make generate`, and fix the Go code until `go test ./...` passes.

## Key configuration

- `IMS_DIRECTORY`: `clubhousedb`, `ims` (the `DIRECTORY_*` tables), or `noop` (tests only)
- DB store: `MariaDB`, or `noop` (tests only). Attachments: `local` or `s3`.
- `IMS_TIME_ZONE` (default `America/Los_Angeles`): the zone the UI shows and takes every date and time
  in, whatever the viewer's device says
- `IMS_BM_API_KEY` / `IMS_BM_API_URL`: optional; enable importing Places from the Burning Man API
- `IMS_ADMINS`: handles with unrestricted access

## Server-side conventions

**Handlers.** Handlers return `*herr.HTTPError` (`lib/herr/`), which carries both an internal error and
a client-safe message. Mutations publish Server-Sent Events (`api/eventsource.go`) that the web UI
listens for.

**Concurrent edits.** Incidents, Field Reports, and Visits have a `VERSION` counter that serves as an
internal compare-and-swap gate: an edit reads the record, applies a version-guarded `UPDATE`, and
retries if another writer got there first (`maxCASAttempts` in `api/incident.go`). Clients send no
precondition. It covers only the record's own columns. Report entries, rosters, incident types, links,
and field-report/visit assignment live in other tables, and set-valued fields change one member at a time through their
own endpoints, so concurrent writers can't undo each other.

**Junk, not deletion.** Nothing is ever deleted. A record made by mistake gets `"junk": true` through
the normal edit endpoint, which logs a system entry. Event-wide lists still return junk items, flagged,
and the list pages hide them client-side (`initJunkFilter` in `ims.ts`). Cross-event search skips them
unless `junk=true`.

**Lists don't carry content.** The event-wide lists (`GET .../incidents`, `.../field_reports`,
`.../visits`) return slimmed list items (`IncidentListItem` etc.) with no report entries, so that
reading a record's content takes a single-record request, which the action log records. The lists take
`number=N` (to refresh one row on an SSE update) and, for Incidents and Field Reports, `q=...`
(`api/listsearch.go`: words or `"phrases"` must all match, `!word` excludes, `/.../` is a regex; an
Incident search includes its attached Field Reports and Visits). Cross-event search is `api/search.go`.

**Action log.** Each route declares an `ActionLogMode` in `api/mux.go`: `LogNothing` (chatty reads),
`LogMetadata` (who did what, including every single-record read), `LogMutation` (also stores the JSON
body in `ACTION_LOG.REQUEST_BODY`, passwords redacted and truncated past 16KiB, `api/actionlogbody.go`),
or `LogSearch` (logs only requests with `q`, storing their query parameters). Logins, attachment uploads,
and bulk Places updates stay at `LogMetadata`.

**Embargoed places.** Camp and art placement is embargoed until the Event's release times. Until then
`api/embargo.go` strips camp/art locations from `GetPlaces` and the `MapURL` from `GetEvents` for
non-admins. The release times themselves are public, so the Places page can say what's embargoed.

**Places import.** `ImportPlaces` (`api/place.go`) fetches one place type for a given year from the
Burning Man API (`lib/bmapi`) and replaces the event's places of that type. The key stays on the server.
An empty upstream response is rejected so a mistyped year can't wipe an event's places.

**Day names.** Events can name days (`EVENT_DAY_NAME`, `api/dayname.go`, admin page
`/ims/app/admin/daynames`). The record pages show the name before timestamps and in date pickers
(`loadDayNames`/`dayName` in `ims.ts`).

## Authentication and authorization

- A single JWT (signed with `IMS_JWT_SECRET`) lasts `IMS_TOKEN_LIFETIME` (default 8h), with no refresh;
  handles in `IMS_LONG_TOKEN_USERS` get `IMS_LONG_TOKEN_LIFETIME` instead (e.g. a dashboard account).
  Tokens can't be revoked, so `authenticate` in `api/mux.go` also requires the user to still be in the
  directory with the same handle.
- Browsers get the token in an `HttpOnly`, `SameSite=Strict` `__Host-ims_access_token` cookie
  (`lib/authz/cookie.go`) and their JavaScript never sees it. `IMS_INSECURE_COOKIES=true` (dev only, set
  by the compose stacks) uses a plain `ims_access_token` over HTTP instead.
- Other clients send `"token_in_body": true` to `POST /ims/api/auth` and use `Authorization: Bearer`,
  which wins over the cookie. Other `Authorization` schemes are ignored.
- Every `/ims/api` route goes through `RejectCrossOrigin`, and the cookie is refused on requests whose
  `Sec-Fetch-Site` is another origin. Responses default to `Cache-Control: no-store`
  (`NoStoreByDefault`).
- The user's positions, teams, and on-site/on-duty status come from the cached `directory.UserStore`,
  not the JWT, so directory changes apply on the next cache refresh.
- Permissions live in `lib/authz/`: per-event roles (`EventReporter`, `EventReader`, `EventWriter`,
  `EventVisitWriter`) map to `EventPermissionMask` bits, global permissions to `GlobalPermissionMask`.

## Web UI conventions

- **Time zones.** Every date and time is shown and edited in `IMS_TIME_ZONE`, which pages read from
  `<meta name="ims-time-zone">` (`imsTimeZone()` in `ims.ts`). Format with the `ims.ts` helpers
  (`zonedParts`, `dateISO`, `longFormatDate`, etc.) and parse with `parseDateInput`/`zonedDate`. Never use
  `Date`'s local getters/setters, or an `Intl.DateTimeFormat` without that `timeZone`. flatpickr only
  knows the browser's zone, so `newFlatpickr` feeds it stand-in Dates (`toPickerDate`/`fromPickerDate`)
  and gives `onChange` real instants.
- **Content-Security-Policy.** The CSP is strict (`contentSecurityPolicy` in `web/mux.go`), so templates
  have no inline `on*=` handlers or `<script>` blocks. Page modules attach listeners with
  `addEventListener`, and parameterized menu items use `data-value` with `ims.onMenuPick`.
  `TestTemplEndpoints` and the Playwright fixtures fail on violations.
