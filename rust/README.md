# IMS in Rust

This is an experimental port of the IMS server to Rust. It's meant to be a
drop-in replacement for `ranger-ims-go serve`. It reads the same environment
variables and `.env` file, and runs the same migrations on the same MariaDB
schema. It serves the same API and web UI, including the same TypeScript and
static assets. The Go server is still the real one, and nothing deploys this.

## Building and running

Generated code isn't checked in, and the Rust build embeds `web/static`. So run
the Go generators first, just as for the Go server:

```bash
make generate
make rust/build      # cargo build --manifest-path rust/Cargo.toml
make rust/run        # ./rust/target/debug/ims serve, configured from ./.env
```

The binary has the Go server's subcommands: `serve`, `healthcheck`,
`hash_password` and `add-user`.

`make rust/compose` runs the Rust server in Docker on :8081, with its own seeded
databases. That leaves it clear of the Go dev stack's :8080.

## Layout

`ims/src` follows the Go packages one-to-one:

| Rust            | Go                                                            |
|-----------------|---------------------------------------------------------------|
| `api/`          | `api/`                                                        |
| `authz/`        | `lib/authz`, `lib/authn`                                      |
| `cmd/`          | `cmd/`                                                        |
| `conf/`         | `conf/`, `cmd/serveconfig.go`                                 |
| `directory/`    | `directory/`                                                  |
| `json/`         | `json/`                                                       |
| `store/`        | `store/`, `store/actionlog`, `store/errorlog`                 |
| `util/`         | the rest of `lib/`                                            |
| `web/`          | `web/`                                                        |

The Go `lib/` becomes `util/` because Rust doesn't allow a module named `lib`
in a library crate.

The port shares these files with the Go tree rather than copying them:

- **SQL:** `store/queries.sql` and `directory/queries.sql` are parsed at
  startup (`store/sqlfile.rs`), so the port runs the same SQL as sqlc does.
  The `sqlc.arg`, `sqlc.narg`, `sqlc.slice` and `sqlc.embed` macros are
  expanded when a query runs. `store/queries.rs` wraps each named query.
- **Schemas and migrations:** `store/schema/` and `directory/schema/`.
- **Static assets:** `web/static/`.

**Templates:** `ims/templates/` holds Askama templates generated from
`web/template/*.templ` by `tools/templ2askama.py`. Don't edit them; after
changing a `.templ` file, rerun:

```bash
python3 rust/tools/templ2askama.py
```

## Testing

- `make rust/test` runs the Rust tests. The unit tests cover the ported `lib/`
  code, the middleware, JSON encoding, the templates and so on.
  - `tests/store_migrate.rs` and `tests/directory_sources.rs` need Docker.
  - `tests/html_parity.rs` needs the Go toolchain. It renders every page with
    the real templ components (`tools/rendergolden`) and checks that the
    Askama pages produce the same document, for each kind of deployment.
- `make rust/test/parity` runs the Go API integration suite
  (`api/integration`) against the Rust binary instead of the in-process Go
  server. It sets `IMS_TEST_TARGET=rust`, and each test's server becomes a
  Rust subprocess on the same test databases (see
  `api/integration/rusttarget_test.go`). The tests that intercept queries
  inside the Go server reach the Rust server through a test-only callback,
  `IMS_TEST_QUERY_HOOK_URL`. No test is skipped.
- The Playwright suite passes against the Rust server too. Its URLs are
  hard-coded to `localhost:8080`, so it's run against a server listening
  there.

## Deliberate differences

These are Go-runtime details that the Rust port can't or shouldn't copy:

- **Debug endpoints:** `/ims/api/debug/runtimemetrics` reports process and
  Tokio figures instead of Go runtime metrics. `/ims/api/debug/gc` is a no-op,
  since Rust has no garbage collector. `/ims/api/debug/buildinfo` reports the
  git revision and rustc version.
- **Error log:** stack traces are Rust backtraces.
- **Memory limit:** there's no `GOMEMLIMIT` tuning.
- **Times in the API:** they're always given in UTC.
- **Test-only settings:** a few extra env vars exist for the parity harness.
  They are `IMS_TEST_PANIC_ROUTE`, `IMS_TEST_SYNC_LOGS`,
  `IMS_TEST_QUERY_HOOK_URL`, `IMS_MAX_REQUEST_BYTES`,
  `IMS_DB_MAX_OPEN_CONNS` and `IMS_DMS_MAX_OPEN_CONNS`. Only a dev deployment
  accepts the `IMS_TEST_*` ones.

## Details worth knowing

Some Go library behaviors had to be reproduced by hand:

- **JSON output:** it matches Go's `encoding/json`. `<`, `>` and `&` are
  escaped, and map keys are sorted (`util/gojson.rs`).
- **JSON input:** a `null` member leaves the field at its zero value, as in Go
  (`api/helpers.rs`). The web client relies on that.
- **Argon2id:** hashes whose memory cost is below Argon2's minimum verify the
  way Go's `x/crypto/argon2` computes them (`util/argon2go.rs`). Go accepts
  those hashes and the argon2 crate doesn't. The API test seed users have
  them.
- **Unparseable requests:** the 400 response looks like Go's, with a
  `text/plain` body (`cmd/goio.rs`).
- **Path cleaning:** unclean paths are redirected, as Go's `ServeMux` does.
- **Durations:** duration env vars use Go's syntax (`8h`, `5m10s`).
