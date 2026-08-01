# Ranger IMS

The Ranger Incident Management System is used by the Black Rock Rangers to track incidents
that occur in Black Rock City.

## Getting started with IMS development:

1. Clone the repo
2. Install Go and have `go` on your PATH. https://go.dev/dl
3. (Optional: if you want to run the integration tests) install Docker Desktop or Docker Engine. https://www.docker.com/
4. (Optional: if you want to run the Playwright tests) install Playwright: https://playwright.dev/docs/intro
5. Generate code and fetch external build dependencies into your repo, by running
   ```shell
   make generate
   ```
   None of the generated code (sqlc, templ, tsgo) is checked in, so a fresh clone
   won't compile until you've run this. Rerun it whenever you change a `.templ`,
   `.ts`, or `.sql` file — or just use `make build` / `make run/live`, which run the
   generators for you.

## Run IMS locally with docker compose

The fastest way to get a local IMS server up-and-running is to use Docker Compose. This
requires only that you install Docker in advance (you don't even need Go!). This approach
uses a live code-reloading mechanism, so any changes to the source code will cause the
server to rebuild and relaunch.

There's good documentation in `docker-compose.dev.yml` that's worth a read.

```shell
make compose/live
```

This copies `.env.dev.example` to `.env.dev` on first run and passes it via
`--env-file`, so the stack reads its own env file instead of the `./.env` used
when you run `ims serve` directly — the two configs can't collide. Edit `.env.dev`
(gitignored) to override defaults. The underlying command is
`docker compose --env-file .env.dev -f docker-compose.dev.yml up`.

## Run IMS locally with MariaDB

**This documentation is slightly wrong, because the TestUsers part is no longer a thing. Just use the docker compose way for now.**

1. Have a local MariaDB server running. An empty database is fine, as the IMS program will
   migrate your DB automatically on startup from nothing. e.g.
   ```shell
   password=$(openssl rand -hex 16)
   echo "Password is ${password}"
   docker run -it \
     -e MARIADB_RANDOM_ROOT_PASSWORD=true \
	 -e MARIADB_DATABASE=ims \
	 -e MARIADB_USER=rangers \
	 -e MARIADB_PASSWORD=${password} \
     -p 3306:3306 mariadb:10.5.29
   ```
2. Copy `.env.example` as `.env`, and set the various flags. Especially read the part in
   `.env.example` about `IMS_DIRECTORY` if you want to use TestUsers rather than a Clubhouse DB.
3. Run the following to build and launch the server. These *should* work on Windows as well as OSX
   and Linux, but Windows is so far untested.
   ```shell
   go run bin/build/build.go
   ./ranger-ims-go serve
   ```

## Run IMS without a Clubhouse database (IMS-native directory)

IMS normally reads its users, teams, and positions from a Ranger Clubhouse
database. Organizations that don't have a Clubhouse can instead use the
IMS-native directory, which stores users in the IMS database itself and is
managed through IMS's admin web UI.

The fastest way to try this is the quickstart compose stack, which runs the
IMS server and its MariaDB with no Clubhouse anything (see the comments at the
top of `docker-compose.quickstart.yml` for the full walkthrough):

```shell
make compose/quickstart
```

(This copies `.env.quickstart.example` to `.env.quickstart` on first run and runs
`docker compose --env-file .env.quickstart -f docker-compose.quickstart.yml up --build`.)

To set it up by hand instead:

1. Set `IMS_DIRECTORY=ims` in your `.env`. The `IMS_DMS_*` (Clubhouse DB)
   settings are then ignored and no Clubhouse database is needed.
2. Start the server once (`./ranger-ims-go serve`) so it creates the database
   tables, then bootstrap your first user:
   ```shell
   ./ranger-ims-go add-user --handle YourHandle --email you@example.org
   ```
3. Add that handle to `IMS_ADMINS` in `.env` and restart the server.
4. Log in to the web UI and manage users, teams, and positions at
   `/ims/app/admin/directory`.

Notes:

* Users log in with their handle (or email) and a password, which admins set
  in the admin UI. Passwords are stored as argon2id hashes in the IMS DB.
* Event access rules (`person:X`, `team:Y`, `position:Z`, `*`, and onsite
  validity) work the same as with a Clubhouse directory. `onduty:` rules never
  match, since the IMS-native directory has no shift/timesheet data.
* Prefer deactivating users over deleting them, so their handles remain
  attributable on old incidents.

## Run tests

To run all the tests (excluding Playwright), just do:

```shell
go test ./...
```

or to run all those tests and see a coverage report, do:

```shell
go test -coverprofile=coverage.out --coverpkg ./... ./... && go tool cover -html=coverage.out
```

## Build and run with Docker

```shell
docker build --tag ranger-ims-go .
docker run --env-file .env -it -p 80:8080 ranger-ims-go:latest
```

or use `docker compose up`

## Upgrade Go dependencies

Upgrade the Go toolchain simply by increasing the Go value in `go.mod`, e.g. https://github.com/burningmantech/ranger-ims-go/pull/64. Even Go major version upgrades (e.g. 1.23 to 1.24) are very unlikely to break anything, thanks to the Go 1.0 backward compatibility guarantee. If all the tests pass, you're all good.

This line in go.mod should be left as the only line in the repo that specifies the Go version. For example, the Dockerfile depends on Go, but it inherits the value in go.mod.

Upgrade all Go dependencies by running:

```shell
# Upgrade all normal and test dependencies
go get -t -u ./...

# Tidy up go.mod and go.sum
go mod tidy
```

## How IMS handles concurrent access

Several Rangers routinely work the same Incident at once: an Operator typing a summary
while a shift lead adds a Ranger to the roster and someone else attaches a Field Report.
This section records how the system deals with that, layer by layer, and — more
importantly — *why* each layer looks the way it does. Some of these choices have been
reversed once already; the reasoning is here so the next person doesn't have to
rediscover it.

### The governing idea

**Concurrent edits are resolved by making operations not conflict, rather than by
detecting conflicts and asking a human to sort them out.**

The alternative — optimistic concurrency exposed to the client, with `ETag`/`If-Match`
and a 412 on mismatch — was implemented and later removed. It's worth understanding why,
because it looks like the textbook answer:

* The failure it prevents is narrow. Edits are **sparse patches**, so two people editing
  different fields never clobber each other regardless of ordering. The only real hazard
  was two people overwriting *the same* field.
* Even that hazard is recoverable. Every field change produces a change-log line
  (`applyStringChange` in `api/helpers.go`) that `addChangeReportEntries` writes as a
  generated report entry, naming the new value, its author, and the time. A
  last-writer-wins overwrite isn't silent data loss — the previous value is one entry up
  in the record's permanent change log.
* The cost was high and fell on the common case. Because *any* change moved the version,
  a Ranger editing the location would get a 412 for a summary edit someone else made a
  second earlier — a conflict error for two edits that didn't actually conflict. The
  frontend needed a serializing mutation queue purely to keep a page's own edits from
  412ing each other.

So the guiding rule is: **if two Rangers can plausibly do these two things at the same
time, the operations must commute.** Reach for conflict detection only where they
genuinely can't.

### Layer 1: the database

Incidents, Field Reports, and Visits each carry a `VERSION` counter
(`store/schema/current.sql`). It is *not* part of the client contract — it's reported in
the record body but clients never send it back. It exists to make the server's own
read-merge-write safe:

1. Read the stored record.
2. Merge the request's fields over it.
3. `UPDATE ... WHERE VERSION = <the value just read>`.

If a competing writer committed in between, that `UPDATE` matches zero rows and the whole
attempt is retried against fresh state. The guarded `UPDATE` is also the *first* lock the
transaction takes, which makes the record's own row the single place competing writers
queue.

Two related rules:

* **Lock ordering.** Anything touching two records (linking Incidents) takes their row
  locks in a fixed global order — ascending by event, then number — regardless of which
  end the request arrived on. See `bumpIncidentPairVersions` in `api/incidentrelation.go`.
  Otherwise two requests from opposite ends of the same pair each hold the row the other
  is waiting for.
* **Bump before writing membership.** Writing to a child table (an Incident's types, its
  roster) takes a *shared* FK lock on the parent row, and the version bump needs that same
  row *exclusively*. Doing the child write first means two writers each hold a shared lock
  the other must upgrade past — which MariaDB resolves by killing one with a deadlock
  error, i.e. a 500 on an endpoint that promises to be safely concurrent.

Deadlocks are still possible under load, so transactions that can hit one are wrapped in
`retryOnDeadlock` (`maxDeadlockAttempts`, jittered exponential backoff). **Nothing may
escape the database before the commit** — no SSE notification, no response — because the
whole closure may run again.

Record numbers are allocated with a plain `SELECT`, so two concurrent creators in one
event can pick the same number. The `(EVENT, NUMBER)` primary key turns that into a
duplicate-key error and the insert retries with a fresh number
(`maxNumberAllocAttempts`).

### Layer 2: the API

**Sparse patches.** An edit body carries only the fields being changed. The server seeds
the update from the stored row and overrides just what was provided
(`buildIncidentUpdate`). This is what makes disjoint field edits inherently safe.

**One member at a time, never a whole set.** Every set-valued relation on an Incident is
mutated by naming the single member being changed. Types, links, and the Ranger roster
each have a per-item sub-resource endpoint:

```
POST   /ims/api/events/{event}/incidents/{number}/incident_types/{typeId}
DELETE /ims/api/events/{event}/incidents/{number}/incident_types/{typeId}
```

Attached Field Reports and Visits reach the same end from the other side: attachment is a
single-valued field on the Field Report or Visit itself, so there's no list to replace.

This exists specifically because the alternative doesn't commute. The API used to accept a
replacement list and diff it against stored state; a client whose list was built from a
stale read would silently *remove* another Ranger's addition while adding its own. A
per-item request says "attach this type" rather than "the set is now exactly this," so two
Rangers adding different types both win. They're idempotent too, which makes retries safe:
a request for membership the record already has is a true no-op that doesn't even move the
version.

Those list fields are now **response-only**. Sending one on an edit is a `400` naming the
endpoint to use instead (`rejectSetReplacement` in `api/incident.go`) — deliberately loud,
because silently ignoring the field would report success for a change that never happened.

**Report entries are exempt from all of this.** Appends can't lose data, and striking is a
reversible boolean that is itself audited. Neither moves the version. This matters:
appending a note must never fail because someone else edited a field, and must never
cause someone else's in-flight edit to retry.

**Retry, don't reject.** When the guarded `UPDATE` reports a conflict, the server re-reads
and retries (`maxCASAttempts`, currently 3) rather than surfacing the conflict. Only if it
loses repeatedly does the client see a `409`.

**Deliberate exception — event access.** Admin permission writes are last-write-wins with
no versioning at all, serialized by a process-level mutex (`eventAccessWriteMu` in
`api/eventaccess.go`). Two admins editing the same event can overwrite each other. That's
accepted because the writes are admin-only, rare, and immediately visible on the admin
page. Note this only serializes *within one process*; it is not a distributed lock.

### Layer 3: real-time propagation

After a transaction commits — never before — the handler publishes an event through
`EventSourcerer` (`api/eventsource.go`), and browsers viewing the affected record refetch
and redraw. When a change alters how a *different* record reads (linking two Incidents,
reassigning a Visit), that record's version is bumped and its own event published, so
every open page converges.

This is a large part of why loud conflict detection turned out to be unnecessary: the
losing side of a last-writer-wins race sees the winning value appear on their screen as
soon as the SSE update lands, rather than having to be told about the conflict.

### Layer 4: the web frontend

* **No preconditions.** Pages send no `If-Match` and track no ETag. Edits are fire and
  reload.
* **One mutation chain per page.** Every mutation a detail page makes goes through
  `newMutationChain` (`web/typescript/ims.ts`), which runs them one at a time. The reason
  is no longer ETags — it's that each mutation refetches and redraws when it completes, so
  concurrent mutations race their redraws and the page can settle on state the server has
  already moved past.
* **Redraws never clobber in-progress typing.** `setInputValue` skips a control the user
  is currently typing into. Overwriting it would discard the keystrokes *and* clear the
  browser's dirty flag, so the `change` event wouldn't fire on blur and the edit would be
  lost. Such a control may briefly show a stale value; it reconciles on the next redraw
  after blur.
* **Announce only other people's changes.** A detail page redraws for its own saves too
  (they come back over SSE). `newRemoteUpdateAnnouncer` debounces and suppresses those, so
  assistive tech announces only what the user couldn't otherwise know: someone else
  editing the record in front of them.

### Layer 5: the audit trail

The backstop under everything above. Every field change appends a generated report entry
naming the new value, its author, and the time; so do roster, type, link, and attachment
changes. Report entries are immutable — strikeable, never edited or deleted.

This is what makes last-writer-wins acceptable rather than reckless. There is no such
thing as a change nobody can see: the losing value is still in the record, visible under
"Show history and stricken."

### If you're adding a new mutating endpoint

1. Can two Rangers plausibly do this at the same time? If so, make the operation
   commutative — express one gesture, not a replacement of a whole set.
2. Route every write through a version-guarded `UPDATE` or a version bump, so racing
   edits retry instead of clobbering. Skip it only if the operation genuinely cannot lose
   data (appends, strikes).
3. Take multi-record locks in a globally fixed order, and take the parent row before its
   children.
4. Wrap anything that can deadlock in `retryOnDeadlock`, and let nothing escape the
   database before the commit.
5. Publish an SSE notification after the commit.
6. Write a generated report entry describing the change.

## Differences between the Go and Python IMS servers

1. We didn't bring over support for a SQLite IMS database, so MariaDB is the only option currently.
   It's kind of a pain supporting two different sets of SQLs statements and needing an abstraction layer
   in the middle. Also, sqlc doesn't support SQLite well yet, and this Go version of IMS makes heavy use
   of sqlc's glorious code generation. If we do end up wanting some lighter alternative to MariaDB for
   some reason, the easier thing would be to make a fake version of the Querier interface, i.e. creating
   an in-memory DB.
2. We use a `.env` file rather than `conf/imsd.conf` for local configuration. This ends up just being a
   lot simpler, since prod only uses env variables anyway, and this means each config setting just has
   one name.
3. We kept the "File" Directory type in spirit, but changed it to "TestUsers" and made it a compiled
   source file, `testusers.go`.
