# Plan: Decoupling the user directory from Ranger Clubhouse

## Implementation status (2026-07-04)

**Implemented.** All phases below are done, with these deviations from the
original plan:

- The Clubhouse and IMS sources live in the existing `directory` package
  (`directory.ClubhouseSource` in `clubhousedb.go`, `directory.IMSSource` in
  `imssource.go`) rather than in `chsource/`/`imssource/` subpackages. The
  sqlc config, schema embed, and docker-compose mounts all point at
  `directory/`, so a package move was pure churn.
- Password hashing uses `argon2id.SecondRecommendedParams` (64 MiB, t=3)
  rather than `FirstRecommendedParams`, which allocates 2 GiB per hash —
  unreasonable for a server-side login/admin path.
- Persons created via the admin API get a placeholder password that is an
  argon2id hash of a random string, so login cleanly fails with 401 (rather
  than a 500 from an unparseable placeholder) until an admin sets a password.
- The `add-user` CLI takes `--handle/--email/--onsite` only (no
  `--team`/`--position` flags); memberships are managed in the admin UI.
- The admin-root nav link to `/ims/app/admin/directory` is always shown (the
  admin root page is static); on Clubhouse deployments the page itself shows
  the 403 error from the API.
- The Clubhouse-free compose stack is `docker-compose.quickstart.yml`
  (`make compose/quickstart`), which runs the production image plus a MariaDB
  and documents the add-user bootstrap flow in its header comment.
- Remaining (not done): `.env.example` documentation of `IMS_DIRECTORY=ims`
  (tooling could not edit that file; suggested text was provided separately).

## Background

IMS currently requires a Ranger Clubhouse MariaDB (or at least its `person`,
`position`, `team`, `person_position`, `person_team`, and `timesheet` tables)
as its user directory. Non-Burning Man Ranger organizations want to run IMS,
and requiring them to stand up a Clubhouse-shaped database is an awkward fit.

This plan adds an **IMS-native user directory**: a set of simple directory
tables living in the IMS database itself, managed through a new admin web UI,
selectable via configuration. ClubhouseDB remains fully supported as an
alternative backend.

### Decisions already made

| Question | Decision |
|---|---|
| Backend type | IMS-native tables in the IMS MariaDB (not a separate DB, not OIDC, not file-based) |
| Authentication | Local argon2id passwords, same login flow and JWT issuance as today |
| Authz model | Users, teams, and positions. No on-duty concept; `onduty:` access expressions simply never match on the native backend |
| Management | Admin web UI for user/team/position CRUD, plus a small CLI command for bootstrapping the first user |

### Goals

- A non-Clubhouse org can run IMS with only the IMS MariaDB — no second database.
- Existing access-control expressions (`person:X`, `position:Y`, `team:Z`, `*`,
  onsite validity) work identically on the native backend.
- Zero behavior change for ClubhouseDB deployments.
- Admins manage directory contents in the web UI without server access.

### Non-goals (for this effort)

- OIDC/SSO login (the design should not preclude layering it on later).
- On-duty/shift tracking in the native store (`onduty:` expressions are
  Clubhouse-only).
- Migrating BM Ranger production off ClubhouseDB.
- Replacing `fakeclubhousedb` for local dev (though the native store may
  eventually make a nicer dev default; see Open Questions).
- Syncing/importing users from Clubhouse into the native store.

## Current state

The seam is already mostly in place. Consumers (`api/*.go`,
`lib/authz/permission.go`) only use three methods on
`directory.UserStore` (`directory/directory.go`):

- `GetAllUsers(ctx)` — login, token refresh, per-request user lookups
- `GetRangers(ctx)` — the personnel API
- `GetPositionsAndTeams(ctx)` — authz expression evaluation

The Clubhouse coupling is all *behind* that seam:

- `UserStore` holds a concrete `*directory.DBQ` wrapping sqlc-generated
  queries (`directory/clubhousedb/`) against the Clubhouse schema.
- `directory/queries.sql` hardcodes Clubhouse person statuses
  (`active`, `inactive`, `auditor`, …) and derives on-duty positions from the
  Clubhouse `timesheet` table.
- `conf.DirectoryType` only knows `clubhousedb` and `noop`.
- `cmd/serve.go` unconditionally opens a Clubhouse DB connection.

## Design

### 1. Pluggable directory source

Keep `directory.UserStore` as the single cached facade that all consumers use
(no changes to `api/` or `lib/authz/` call sites). Extract its data-fetching
into an interface, and make `UserStore` wrap a source:

```go
// directory/directory.go
type Source interface {
    FetchUsers(ctx context.Context) (map[int64]*User, error)     // fully populated, incl. team/position memberships
    FetchPositions(ctx context.Context) (map[int64]string, error)
    FetchTeams(ctx context.Context) (map[int64]string, error)
}

func NewUserStore(source Source, cacheTTL time.Duration) *UserStore
```

Implementations:

- **`directory/chsource/`** — the existing Clubhouse logic, moved verbatim:
  `DBQ`, the sqlc-generated `clubhousedb` package, and the cache-refresh code
  currently in `UserStore.refreshUserCache` et al. On-duty position handling
  stays here, since only this source has the data.
- **`directory/imssource/`** — new; reads the native tables (below) via sqlc
  queries added to `store/queries.sql`. `OnDutyPositionID`/`OnDutyPositionName`
  are always nil. `Status` is `"active"` for every user it returns (it only
  returns active users).

The existing three-cache structure (`userCache`, `positionCache`, `teamCache`)
stays in `UserStore`, shared by both sources.

`UserStore` gains one new method, `Flush()`, which invalidates all three
caches. The admin write path (below) calls it so directory edits are visible
immediately instead of after `IMS_DIRECTORY_CACHE_TTL`. This requires adding
an invalidation method to `lib/cache.InMemory`.

### 2. Native directory schema (IMS database)

New tables in `store/schema/current.sql`, following existing naming
conventions (`34-from-33.sql` migration; bump `SCHEMA_INFO.VERSION` to 34):

```sql
create table DIRECTORY_PERSON (
    ID       bigint       not null auto_increment,
    HANDLE   varchar(128) not null,
    EMAIL    varchar(256),          -- optional; login accepts handle or email
    PASSWORD varchar(256) not null, -- argon2id PHC string; may be a placeholder that can never verify
    ACTIVE   boolean      not null default true,
    ONSITE   boolean      not null default false,

    primary key (ID),
    unique key UNIQUE_HANDLE (HANDLE),
    unique key UNIQUE_EMAIL (EMAIL)
);

create table DIRECTORY_TEAM (
    ID     bigint       not null auto_increment,
    TITLE  varchar(128) not null,
    ACTIVE boolean      not null default true,
    primary key (ID),
    unique key UNIQUE_TITLE (TITLE)
);

create table DIRECTORY_POSITION (
    ID     bigint       not null auto_increment,
    TITLE  varchar(128) not null,
    ACTIVE boolean      not null default true,
    primary key (ID),
    unique key UNIQUE_TITLE (TITLE)
);

create table DIRECTORY_PERSON__TEAM (
    PERSON_ID bigint not null,
    TEAM_ID   bigint not null,
    primary key (PERSON_ID, TEAM_ID),
    foreign key (PERSON_ID) references DIRECTORY_PERSON(ID) on delete cascade,
    foreign key (TEAM_ID)   references DIRECTORY_TEAM(ID)   on delete cascade
);

create table DIRECTORY_PERSON__POSITION (
    PERSON_ID   bigint not null,
    POSITION_ID bigint not null,
    primary key (PERSON_ID, POSITION_ID),
    foreign key (PERSON_ID)   references DIRECTORY_PERSON(ID)   on delete cascade,
    foreign key (POSITION_ID) references DIRECTORY_POSITION(ID) on delete cascade
);
```

Notes:

- **Status model**: Clubhouse's rich status taxonomy collapses to a single
  `ACTIVE` flag. Inactive users cannot log in and don't appear in personnel
  lists — equivalent to Clubhouse's excluded statuses. Deactivation, not
  deletion, is the normal way to offboard (handles referenced from incidents
  should keep resolving).
- **`ONSITE`** feeds the existing `validity=onsite` access rules and is a
  plain admin-editable flag. Orgs that don't care leave it false and use
  `validity=always` rules (the default).
- These tables exist in every IMS database regardless of directory type
  (schema is uniform; they're simply unused under `clubhousedb`).
- Uniqueness of `HANDLE` is required because handles are the identity keys in
  JWTs, `IMS_ADMINS`, `person:` expressions, and incident attribution — same
  as Clubhouse callsigns today.

### 3. Configuration

- New `conf.DirectoryType` value: `ims` (alongside `clubhousedb`, `noop`),
  set via the existing `IMS_DIRECTORY` env var. Update
  `DirectoryType.Validate()`, `.env.example`, and README.
- When `IMS_DIRECTORY=ims`:
  - `cmd/serve.go` does **not** open a Clubhouse DB connection; the
    `IMS_DMS_*` vars are ignored (and blanked in `Validate()`/`PrintRedacted`,
    like the existing non-clubhousedb handling in `conf/imsconfig.go`).
  - The directory source is built on the already-open IMS `store.DBQ`.
- `Directory.InMemoryCacheTTL` applies to both source types unchanged.

### 4. Authentication

No changes to the login flow, JWT contents, or refresh logic in `api/auth.go`:
the native source returns `directory.User` values with argon2id `Password`
strings and `lib/authn.Verify` works as-is. `OnDutyPositionID` nil is already
handled (JWT claim is a pointer).

Password hashing for admin-set passwords uses
`lib/argon2id.CreateHash` with `FirstRecommendedParams` (not
`DevelopmentParams`, which is tuned for the fake seed data).

### 5. Admin API and web UI

New admin surface, gated on the `Administrator` role **and** on
`IMS_DIRECTORY=ims` (under `clubhousedb` the endpoints return 4xx and the nav
link is hidden — Clubhouse remains the source of truth there).

API endpoints (in `api/`, following existing handler + `AddToMux` patterns,
with action-logging like other mutations):

- `GET  /ims/api/directory` — persons (without password hashes), teams,
  positions, memberships
- `POST /ims/api/directory/persons` — create/update person
  (handle, email, active, onsite, team IDs, position IDs)
- `POST /ims/api/directory/persons/{id}/password` — set password
  (server-side hashing; plaintext accepted over HTTPS exactly like login)
- `DELETE /ims/api/directory/persons/{id}` — hard delete (UI steers admins
  toward deactivation instead)
- `POST /ims/api/directory/teams`, `POST /ims/api/directory/positions` —
  create/update/rename; `DELETE` for both
- All mutations call `UserStore.Flush()` on success.

Renaming a team/position changes what `team:`/`position:` access expressions
match, since `EVENT_ACCESS` expressions store names. Same behavior as a rename
in Clubhouse today; the UI should warn when renaming an entity that appears in
any `EVENT_ACCESS` expression.

Web UI: `web/template/admindirectory.templ` + `web/typescript/admindirectory.ts`,
modeled on `adminevents.templ`/`admintypes.templ`: tables of persons, teams,
and positions with inline edit, an add-person form, a set-password control, and
checkbox pickers for memberships. Linked from `adminroot.templ`.

### 6. Bootstrap CLI

A fresh `ims` deployment has an empty directory, and the admin UI requires
logging in as a user listed in `IMS_ADMINS`. Chicken-and-egg. Add a cobra
command:

```
ranger-ims-go add-user --handle Alice --email alice@example.org [--position ...] [--team ...]
```

It prompts for a password (or reads `--password-stdin`), hashes it, connects
to the IMS DB using the normal `.env` config, and upserts the person. Ops then
adds the handle to `IMS_ADMINS`. This complements, not replaces, the admin UI,
and reuses machinery from the existing `hashpassword` command.

### 7. Personnel API

`GetPersonnel` (`api/personnel.go`) and `GetRangers` need no changes: native
users surface with `Status: "active"`, and email/password are already
stripped. The debug/directory admin pages that display Clubhouse cache info
(`api/debug.go`) should be checked for Clubhouse-type assumptions and made
source-agnostic.

## Implementation phases

Each phase lands independently green (`go test ./...`, integration tests,
pre-commit).

### Phase 1 — Extract the seam (no behavior change)

1. Define `directory.Source`; move Clubhouse fetch logic from
   `UserStore.refresh*` into `directory/chsource/` (wrapping the existing
   `DBQ`/sqlc code).
2. `NewUserStore(source, ttl)`; update `cmd/serve.go` and all test
   constructors.
3. Add cache invalidation to `lib/cache.InMemory` and `UserStore.Flush()`.
4. Verify ClubhouseDB and noop modes behave identically (existing unit +
   `api/integration` tests).

### Phase 2 — Native store, read path

1. Migration `store/schema/34-from-33.sql` + `current.sql` (version 34);
   run `go test ./store/integration`.
2. Directory queries in `store/queries.sql`; regenerate sqlc via the build
   script.
3. Implement `directory/imssource/` and the `ims` config type; wire up
   `cmd/serve.go` (skip Clubhouse connection).
4. Tests: login/refresh/personnel/authz against a seeded native directory in
   `api/integration` (reuses the existing IMS MariaDB container — the
   Clubhouse container isn't needed for this variant).

### Phase 3 — Bootstrap CLI

1. `add-user` cobra command with password prompt and argon2id hashing.
2. This makes `ims` mode genuinely usable end-to-end (CLI-managed), even
   before the UI lands.

### Phase 4 — Admin API + web UI

1. API endpoints with authz gating, action logging, cache flush, and unit
   tests.
2. `admindirectory.templ` + TypeScript, nav wiring, Vitest coverage in
   `web/typescripttest/`.
3. Playwright happy-path test: create user in UI → log in as that user.

### Phase 5 — Docs and dev ergonomics

1. README + `.env.example` documentation of `IMS_DIRECTORY=ims` for external
   orgs.
2. Optional: a `docker-compose` variant (or profile) that runs IMS with the
   native directory and no Clubhouse container, as the quick-start for
   external orgs.

## Open questions

- **Config naming**: `ims` vs `internal` vs `builtin` for the new
  `DirectoryType` value. Plan assumes `ims`.
- **Should `noop` remain?** Once the native store exists it may subsume the
  noop directory in tests. Out of scope, revisit later.
- **Local dev default**: `docker-compose.dev.yml` could eventually seed the
  native directory instead of the fake Clubhouse DB, removing a container
  from the dev stack. Deferred to keep this change smaller.
- **Password self-service**: users changing their own passwords (not just
  admin resets) would need a small authenticated endpoint + settings-page UI.
  Worth doing soon after Phase 4, but not required for launch.
- **Future OIDC**: the `Source` interface deliberately keeps identity
  (handle/ID) separate from authn (password verification happens in
  `api/auth.go`), so an OIDC flow could later mint the same JWTs for users
  from either store without reworking this design.
