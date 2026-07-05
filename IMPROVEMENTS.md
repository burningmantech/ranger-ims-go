# Code Cleanup Opportunities

An assessment of the most impactful code cleanups for this repo, ranked by
impact. The repo is in solid shape overall (clean layering, generated code
well-separated, strong test coverage push recently). The cleanup opportunities
are concentrated in two places: the incident/field-report/visit triplication in
`api/`, and the parallel page scripts in `web/typescript/`.

## 1. De-duplicate the incident/visit/field-report handler triplet in `api/` — DONE

Implemented (−437/+181 lines across `api/`):

- The four Ranger attach/detach handlers now share one flow in `api/ranger.go`,
  parameterized by a `rangerRoster` descriptor (permission bit, path key, noun,
  and the entity-specific sqlc calls).
- Report-entry creation is shared: `createReportEntry` in `api/reportentry.go`
  does the insert, and the thin `addIncidentReportEntry`/`addFRReportEntry`/
  `addVisitReportEntry` wrappers do the entity-specific attach. The
  `true, "", "", ""` positional tail was replaced by a `newReportEntry` struct.
- Not done: `fetchIncident`/`fetchVisit`/`fetchFieldReport` and the `*ToJSON`
  functions were left as-is — their remaining "duplication" is mostly distinct
  sqlc types and entity fields, and Go generics can't reach common struct
  fields without more abstraction than the ~30 lines each would save.

## 2. Break up `updateIncident` (~375 lines) and `updateVisit` (~215 lines) — DONE

Implemented:

- `updateIncident` is now a ~100-line orchestrator. The field diffing lives in
  `buildIncidentUpdate`, and each reconciliation section is its own function:
  `applyIncidentTypeChanges`, `applyIncidentFieldReportChanges`,
  `applyIncidentVisitChanges`, `applyLinkedIncidentChanges`.
- `updateVisit` shrank similarly, with field diffing in `buildVisitUpdate`.
- The repeated `if newX != nil { update.X = ...; logs = append(...) }` blocks
  (7 in incident, 21 in visit) collapsed into `applyStringChange` /
  `applyInt16Change` helpers in `api/helpers.go`, preserving the exact log
  text.
- The shared "write change log + user report entries" tail became
  `addChangeReportEntries` in `api/reportentry.go`, also reused by the
  field-report edit/create handlers.
- Bonus: removed a dead `Events` query in `updateVisit` (it built an
  `eventNameById` map that was never read — copied from `updateIncident`).

## 3. Collapse the middleware boilerplate in `api/mux.go`

All 46 routes repeat the same 4–5 line `Adapt(handler, RecoverFromPanic(),
RequireAuthN(jwter), LogRequest(true, actionLogger, userStore),
LimitRequestBytes(...))` stack. A local helper like `std(h Handler)
http.Handler` (with an explicit variant for the few unauthenticated routes like
`POST /ims/api/auth`) would cut ~300 lines and — more importantly — make
"forgot `RequireAuthN`" impossible to write silently.

Similarly, every handler struct carries the same `imsDBQ, userStore, es,
imsAdmins` fields, and every action has an identical 8-line `ServeHTTP`
wrapper; a shared base struct plus a generic
`func jsonHandler[T](fn func(*http.Request) (T, *herr.HTTPError)) http.Handler`
adapter would remove ~40 copies of each.

## 4. Same triplication on the TypeScript side

The table pages `incidents.ts`, `field_reports.ts`, and `sanctuary_visits.ts`
share roughly half their content — after mechanically renaming
"incident"→"field report", about 800 of 1459 lines between just the first two
are identical (DataTables setup, SSE subscription, filtering, rendering glue).
A shared "entity table page" module parameterized by columns/endpoints would
shrink those three files dramatically. The detail pages (`incident.ts`,
`field_report.ts`, `sanctuary_visit.ts`) share less (~a third) but the
report-entry and page-init plumbing is common.

## 5. Split `ims.ts` (2184 lines)

It's the single highest-churn file in the repo and it's a grab-bag: fetch/auth/
refresh logic, date/timezone formatting, DataTables render functions,
report-entry DOM construction, page-init, and misc utilities all in one module.
Splitting it along those seams (e.g. `fetch.ts`, `datetime.ts`, `render.ts`,
`reportentry.ts`, `page.ts`) would make both reading and testing easier — and
the growing Vitest suite means smaller modules pay off immediately.

## Smaller items, aligned with existing TODOs

- `api/fieldreport.go:298` — the TODO to kill the field-report "action"
  query-param framework in favor of a plain POST (like visits already do)
  would remove a special case.
- `api/event.go:120` — the TODO to split the combined create-or-update
  endpoint into RESTful create/update.
- 25 call sites hand-roll `conv.ParseInt32(req.PathValue(...))` + a BadRequest
  wrap; a tiny `pathInt32(req, "incidentNumber")` helper in `api/helpers.go`
  would tidy every handler.

## Suggested starting point

Item #1: it's the largest raw reduction (~1000+ lines), it targets the code
that changes most, and #2 and the fieldreport TODO become much easier once
incident/visit/field-report share machinery.
