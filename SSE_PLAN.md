# Plan: authenticated, payload-carrying SSE

Status: proposed (2026-07-04). Not yet started.

## Background

Today's Server-Sent Events design is notify-only and unauthenticated:

- `GET /ims/api/eventsource` is registered unauthenticated (`api/mux.go`), with a
  single global channel. Events carry only `{event_id, incident_number |
  field_report_number | visit_number}` (`api/eventsource.go`).
- One leader browser tab (Web Locks) owns the EventSource and re-broadcasts over
  BroadcastChannel; each page then GETs the updated entity via the authenticated
  REST API (`web/typescript/ims.ts`, `subscribeToUpdates`).
- Missed-event recovery: `ReplayAll` + an `InitialEvent` carrying the latest SSE
  ID, compared against `localStorage["last_sse_id"]`; a mismatch triggers an
  `update_all` full refetch.
- API auth is a Bearer access token (15 min, from localStorage). The browser
  `EventSource` API can't set an `Authorization` header, which is why the SSE
  endpoint is unauthenticated.
- Authorization is per-connection-varying: per-event permission masks, and for
  field reports even per-record ("own" = user authored a report entry).

### Why change

- **Thundering herd**: every update makes every connected browser on that event
  GET the same entity simultaneously; update latency is two hops (notify →
  fetch).
- **Metadata leak**: anyone, logged in or not, can watch the stream and learn
  incident/FR/visit numbers, event IDs, and operational tempo. It's also an
  unauthenticated long-lived connection anyone can open in bulk.
- Users receive notifications for events they have no access to; filtering is
  client-side only.

### Costs accepted with the new design

- Authorization moves into the push path: a second enforcement point that must
  mirror REST authz (per event, per entity type, own-vs-all field reports) and
  can drift from it.
- Connection auth needs solving (EventSource can't send headers), plus
  mid-stream token expiry.
- The client still needs the fetch path for initial load and missed-event
  recovery, so push becomes a cache-update protocol.
- `launchdarkly/eventsource`'s channel model can't do per-connection filtering
  or multi-channel subscription, so we build a custom SSE hub.

### Decisions made

1. **Connection auth**: fetch-based streaming client sending the normal
   `Authorization: Bearer` header (not cookies, not query-param tokens).
2. **Payload**: full entity JSON, identical in shape to the GET response.
3. **Replay**: none. Keep the `InitialEvent` / `last_sse_id` / `update_all`
   refetch fallback for gaps; no server-side payload buffer.
4. **Rollout**: two PRs — authenticate + filter first (still ID-only), then add
   payloads.

## PR 1 — Authenticate the stream and filter per connection (notifications stay ID-only)

This lands the risky part — connection auth and per-connection authorization —
while the events are still just numbers, so a filtering bug can't leak content.

### Server

1. **Replace `launchdarkly/eventsource` with a small custom hub** in
   `api/eventsource.go`. The hub keeps: the atomic SSE ID counter, a
   mutex-guarded set of connections, and a per-connection buffered channel. Each
   connection stores its subscriber identity: ranger handle plus a per-event
   permission mask snapshot, computed at connect time via the existing
   `authz.ManyEventPermissions` machinery (`lib/authz/permission.go`).
2. **Register the endpoint as authenticated**: change the `unauthed` route in
   `api/mux.go` to `authed`. `RequireAuthN` already puts claims in the request
   context; the handler reads them from there.
3. **Connection lifetime = access-token lifetime.** The server closes the stream
   when the connecting token's `exp` passes. That bounds how stale a
   connection's permission snapshot can get (≤15 min) without any mid-stream
   re-checking, and the client's existing reconnect loop picks it back up with a
   fresh token. Add a keepalive comment frame every ~20s (the library did this
   for us; proxies kill silent connections).
4. **Filter at publish time.** `notifyIncidentUpdate` → only connections with
   `EventReadIncidents` for that event ID; `notifyVisitUpdate` →
   `EventReadVisits`; `notifyFieldReportUpdate` → `EventReadAllFieldReports`, or
   `EventReadOwnFieldReports` when the recipient is an author. The FR notify
   call needs the author set passed in — the mutation handlers
   (`api/fieldreport.go`) already have the report entries in hand.
5. **Backpressure**: if a connection's buffer is full, close it. The client
   reconnects, sees an SSE-ID gap, and does the `update_all` refetch — the
   existing recovery path doubles as the slow-consumer policy.
6. **Keep the `InitialEvent`** carrying the current counter ID on connect
   (reimplemented without the library's `Replay` interface).

### Client (`web/typescript/ims.ts`)

7. **Replace `new EventSource(...)` in `subscribeToUpdates` with a fetch-based
   reader**: `fetch(url_eventSource, {headers: {Authorization: ...}})` + a small
   `text/event-stream` parser over the body ReadableStream (~50 lines:
   accumulate lines, split frames on blank line, honor `event:`/`data:`/`id:`).
   Dispatch into the exact same listener functions. On 401, run the existing
   token-refresh flow, then reconnect. The Web-Locks leader loop already owns
   reconnection, so we lose nothing by giving up EventSource's auto-reconnect.
8. Everything downstream — BroadcastChannel fan-out, `last_sse_id` in
   localStorage, `update_all` fallback — is untouched.

### Tests

- Go unit tests on the hub's filtering (per-event masks, own-FR authorship,
  admin).
- `api/integration` test: an unauthenticated connect gets 401; a user with
  access to event A but not event B only receives A's notifications.
- Vitest coverage of the stream parser with a mocked `fetch`.

### Suggested first commit

The custom hub with the endpoint still unauthenticated — a pure library
replacement, behavior-identical — then auth + client in the same PR.

## PR 2 — Push the full entity JSON

1. **Factor entity assembly out of the GET handlers.** `GetIncident`,
   `GetFieldReport`, `GetVisit` each build an `imsjson` response from the DB;
   extract that into shared functions so the mutation path can produce the
   identical shape. This is the most invasive server change — it's also the
   guarantee that pushed and fetched payloads never drift.
2. **Publish after commit**: mutation handlers load the fresh entity and hand it
   to the hub, which JSON-serializes once and writes the frame to every
   authorized connection. Add the payload as a new field on `IMSEventData`
   (e.g. `incident: {...}`) alongside the existing number fields — a client that
   gets an event without a payload just falls back to refetching, which gives
   free graceful degradation.
3. **Client applies payloads directly**: the leader broadcasts the entity over
   BroadcastChannel; `incidents.ts` updates the DataTables row from the payload
   instead of GETing, and `incident.ts` / `field_report.ts` render from it. The
   REST fetch path stays for initial load, `update_all` recovery, and
   payload-less events. Ordering is safe by construction — events are published
   post-commit and delivered in order per connection — but pages should ignore a
   pushed entity older (by SSE ID) than one they've already applied, as cheap
   insurance.
4. **Verify payloads are requester-independent.** The GET response shape must
   not vary by who asks (it doesn't today; `attachmentsEnabled` is global
   config). If anything requester-specific ever creeps in, it can't be pushed.

### Tests

- Integration test: an own-FR-only user never receives another author's FR
  payload; a pushed incident deep-equals the GET response.
- Vitest tests for the direct-apply path.

## Risks

- **The authz mirror.** The hub's filtering is a second enforcement point that
  must track `lib/authz` semantics (position/team/onsite grants, validity
  windows). The 15-minute snapshot lifetime bounds staleness, but a revoked
  user keeps receiving pushes until their token expires — the same window as
  REST access today, so no regression, but worth stating.
- **Fan-out bandwidth.** An incident with a long report-entry history gets
  pushed in full to every reader on every edit. Total bytes are no worse than
  today's N simultaneous GETs of the same entity — but it's now the server's
  concern. If it ever matters, a "slim row + refetch detail" variant is the
  fallback lever.
- **Proxy behavior.** SSE runs behind Cloudflare (`CF-Connecting-IP` handling in
  `api/mux.go`); the keepalive interval needs to beat its idle timeout. The
  current setup already lives with this — just don't lose the heartbeat when
  the library goes.
