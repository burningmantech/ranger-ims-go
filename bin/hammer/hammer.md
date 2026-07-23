# hammer

`hammer` creates Incidents, Field Reports, or Visits concurrently against a
running IMS server, then checks that the server handed out record numbers
correctly under contention. It's a standalone tool under `bin/`, separate from
the `ims` server binary.

## What it tests

Each creation allocates its number by reading `MAX(NUMBER)+1` for the event and
then inserting. Concurrent creators in the same event can therefore pick the same
number; the `(EVENT, NUMBER)` primary key turns that collision into a
duplicate-key error, which the server is expected to absorb by retrying with a
fresh number.

`hammer` releases all its workers at once so the first burst of requests races
for the same number, then reports failure if:

- any creation didn't return `201 Created`,
- two creations came back with the **same** number,
- a created record can't be read back from the server afterward, or
- any creation got a `409 Conflict` — a graceful but real user-facing failure,
  meaning the server exhausted its number-allocation retry budget.

## Caveats

Every run writes **real records** to whatever server it's pointed at, and there
is **no cleanup**. Point it at a scratch event. Created records are labeled
`[hammer] test record N of M` so the debris is recognizable in the UI.

As a guard against accidents, `hammer` refuses to run against a non-loopback host
unless `-allow-remote` is given.

## Usage

```bash
go run ./bin/hammer -event <scratch-event> -identification <handle-or-email> [flags]
```

The login identity needs write permission on the target event. You'll be
prompted for the password, or pass `-password-stdin` to read it from stdin (for
non-interactive use):

```bash
echo "$IMS_PASSWORD" | go run ./bin/hammer \
  -event scratch \
  -identification hammer-user \
  -password-stdin
```

## Flags

| Flag | Default | Description |
| --- | --- | --- |
| `-server_url` | `http://localhost:8080` | URL and port of a running IMS server |
| `-event` | *(required)* | Event to create records in. Use a scratch event |
| `-identification` | *(required)* | Handle or email to log in as; needs write permission on the event |
| `-password-stdin` | `false` | Read the password from stdin rather than prompting |
| `-entity` | `incidents` | What to create: `incidents`, `field_reports`, or `visits` |
| `-concurrency` | `16` | How many creations to have in flight at once |
| `-count` | `200` | How many records to create in total |
| `-timeout` | `30s` | Per-request timeout |
| `-allow-remote` | `false` | Permit hammering a host other than localhost |

## Output

On success it prints a summary and exits `0`:

```
Creating 200 incidents in event "scratch", 16 at a time, against http://localhost:8080

  created:    200 of 200
  elapsed:    1.204s (166.1 creations/sec)
  latency:    p50 92ms, p95 210ms, max 480ms
  numbers:    200 distinct, 1 through 200
  read back:  200 of 200 found on the server

OK: every creation got a distinct number, and all of them read back.
```

If the server mishandled the contention, it prints the offending status codes,
the first few failures, and a list of problems, then exits non-zero.
