# Changelog

This is the changelog for ranger-ims-go/ranger-ims-server. This is intended to summarize changes over time. It's
probably too verbose for consumption by general users of IMS, but it might be useful for anyone
trying to follow along with IMS's progression as a system.

This file must use the [Common Changelog format](https://common-changelog.org/), with the variation
that we use months rather than version numbers. We don't include dependency version upgrades in the
changelog, as those would pollute this too much.

<!--
Each month below should look like the following, using the same ordering for the four categories:
## YYYY-MM
### Changed
### Added
### Removed
### Fixed

This page accounts for changes up until:
https://github.com/burningmantech/ranger-ims-go/pull/528
-->

## 2026-02

## 2026-01

### Added

- Added "Roles" for Rangers within an incident. These are short textual representations of a Ranger's role within an incident, e.g. "Incident Commander" or "First on scene". https://github.com/burningmantech/ranger-ims-go/issues/451

## 2025-11

### Added

- Added "event groups", which allow for much easier administration of event permissions. https://github.com/burningmantech/ranger-ims-go/pull/493

### Changed

- Did a bunch of internal frontend cleanups, moved a lot of templatizing into HTML template elements.

## 2025-10

### Changed

- Got rid of the location address dropdowns (concentric, radial hour, radial minute), and replaced them with a string field. https://github.com/burningmantech/ranger-ims-go/pull/463

### Added

- Added a "Destinations" functionality to IMS, so that users can see all the art and camps for an event. Users can also quickly set the location of an Incident to match one of those camps or art pieces. https://github.com/burningmantech/ranger-ims-go/pull/461
- Started allowing setting the IMS number for an FR from the FR page. https://github.com/burningmantech/ranger-ims-go/pull/452

## 2025-09

### Changed

- Started including inactive Rangers in "add Rangers" options. This makes it possible to add a Ranger to an incident while they're on their first shift in years. https://github.com/burningmantech/ranger-ims-go/pull/382
- Began populating summary values from the first report entry. Previously we just populated the placeholder. This makes it easier for a user to modify that value, rather than just start from scratch. https://github.com/burningmantech/ranger-ims-go/pull/397
- For local development, switched from an in-process MySQL clone into using Docker Compose with full MariaDB containers. https://github.com/burningmantech/ranger-ims-go/pull/428 https://github.com/burningmantech/ranger-ims-go/pull/431
- Started using an external library to detect file attachment file types. This gives the right type much more reliably, allowing for better previewing. https://github.com/burningmantech/ranger-ims-go/pull/415
- Switched to RFC 9457-style errors in all APIs. This allows for cleaner error handling and better message display to users. https://github.com/burningmantech/ranger-ims-go/pull/408 https://github.com/burningmantech/ranger-ims-go/pull/409
- Improved suggested filename when printing to PDF. https://github.com/burningmantech/ranger-ims-go/issues/416

### Added

- Introduced Incident "linking". This is a way to create bidirectional connections between Incidents from the Incident page. https://github.com/burningmantech/ranger-ims-go/issues/435
- Created access expirations, which allow permissions to expire at a provided time. https://github.com/burningmantech/ranger-ims-go/pull/389
- Added a show/hide password button on the login screen. https://github.com/burningmantech/ranger-ims-go/pull/388
- Started saving and restoring Incidents table state, so that navigating to an Incident and back doesn't reset the table filters/pagination. https://github.com/burningmantech/ranger-ims-go/pull/424 https://github.com/burningmantech/ranger-ims-go/pull/440
- Added support for previewing Quicktime file attachments, with special casing for Chromium browsers. https://github.com/burningmantech/ranger-ims-go/pull/417
- Began indicating on Admin Events page when a permission references an unknown person, position, or team. https://github.com/burningmantech/ranger-ims-go/pull/411
- Added an Incident "closed" time to the data model and API, and backfilled those in the database. https://github.com/burningmantech/ranger-ims-go/pull/401
- Added a way to pick a preferred Incidents table state filter, such that the preference persists through the IMS session. https://github.com/burningmantech/ranger-ims-go/pull/445 https://github.com/burningmantech/ranger-ims-go/pull/450

### Fixed

- Stopped resetting pagination (to page 1) anytime an incident/field report SSE arrives. https://github.com/burningmantech/ranger-ims-go/pull/387
- Disabled autocomplete on fields like "summary" for which autocomplete is annoying rather than useful. https://github.com/burningmantech/ranger-ims-go/pull/386
- Resolved a bug in which "override start time" and "permission expiration times" would fail to set in Safari desktop and mobile. https://github.com/burningmantech/ranger-ims-go/pull/431
- Made it so that ctrl/cmd+clicking any Incidents table row always opens in a new tab. This is basically what we were doing before, but it was glitchy sometimes. https://github.com/burningmantech/ranger-ims-go/pull/420
- Fixed an issue where clearing a search box wouldn't always reset the view. https://github.com/burningmantech/ranger-ims-go/pull/414
- Resolved a race condition that sometimes led to each Incident being shown twice in the Incidents table. https://github.com/burningmantech/ranger-ims-go/pull/400

## 2025-08

### Changed

- Upgraded to Go 1.25 and started using some new features (e.g. of os.Root).

### Added

- Created "multisearch", which makes it easier to query multiple events at once. https://github.com/burningmantech/ranger-ims-go/pull/351 https://github.com/burningmantech/ranger-ims-go/pull/352 https://github.com/burningmantech/ranger-ims-go/pull/354 https://github.com/burningmantech/ranger-ims-go/pull/356

### Fixed

- Started properly detecting HEIC image file attachments. https://github.com/burningmantech/ranger-ims-go/pull/367
- Fixed handling of text/plain file attachments. https://github.com/burningmantech/ranger-ims-go/pull/364

## 2025-07

### Changed

- Limited concurrent access to password validation code, since Argon2id (by design) uses a lot of memory, and we don't want IMS to grow too fast and get killed by Fargate. https://github.com/burningmantech/ranger-ims-go/pull/300 https://github.com/burningmantech/ranger-ims-go/pull/301

### Added

- Added a "password reset" link, which just redirects to Clubhouse prod/staging. https://github.com/burningmantech/ranger-ims-go/pull/337 https://github.com/burningmantech/ranger-ims-go/pull/344
- Made modals on the event access page that show which Rangers have access via which rules. https://github.com/burningmantech/ranger-ims-go/pull/320 https://github.com/burningmantech/ranger-ims-go/pull/314
- Started doing some tuning of the Go memory limit when IMS is running on AWS Fargate, to help ensure IMS stays below its allowed cgroup limit. https://github.com/burningmantech/ranger-ims-go/pull/310 https://github.com/burningmantech/ranger-ims-go/pull/309 https://github.com/burningmantech/ranger-ims-go/pull/308 https://github.com/burningmantech/ranger-ims-go/pull/307 https://github.com/burningmantech/ranger-ims-go/pull/306 https://github.com/burningmantech/ranger-ims-go/pull/305 https://github.com/burningmantech/ranger-ims-go/pull/304
- Added a ROC chat Slack link to the Incidents page in production. https://github.com/burningmantech/ranger-ims-go/pull/296

### Fixed

- Improved print display a bit, by forcing the site into light mode for printing. https://github.com/burningmantech/ranger-ims-go/pull/322
- Made regexp searches involving spaces work properly. https://github.com/burningmantech/ranger-ims-go/pull/289
- Made changes to Cloudflare CDN caching instructions, mostly so that Cloudflare doesn't cache our JS excessively long.

## 2025-06

### Changed

- Made the access token JWT much shorter, by switching from position/team names to IDs, and then representing those IDs as bitsets. https://github.com/burningmantech/ranger-ims-go/pull/187 https://github.com/burningmantech/ranger-ims-go/pull/189
- Limited Cloudflare caching of IMS's JavaScript files. This was annoying before, because Cloudflare would override our Cache-Control header and tell clients to cache for 4 hours. That was a problem when we were pushing out new code and wanted clients to get the new code soon after the server code was updated.
- Switched over to talking about Incident Types by ID rather than name in the APIs. https://github.com/burningmantech/ranger-ims-go/pull/239 https://github.com/burningmantech/ranger-ims-go/pull/240
- Improved linkification of rows on Incidents and Field Reports tables.

### Added

- Implemented "preview" and "download" functionality for file attachments. https://github.com/burningmantech/ranger-ims-go/pull/185 https://github.com/burningmantech/ranger-ims-go/pull/184
- Added action logging to IMS. This lets admins see who did what in IMS. https://github.com/burningmantech/ranger-ims-go/pull/267 https://github.com/burningmantech/ranger-ims-go/pull/269 https://github.com/burningmantech/ranger-ims-go/pull/271
- Added "on-duty" link to Clubhouse on Incident page. https://github.com/burningmantech/ranger-ims-go/pull/261
- Allowed permissions based on the current position for which a person is on-duty. https://github.com/burningmantech/ranger-ims-go/pull/258
- Added Incident Type descriptions as a concept. Admins can set the descriptions, and users can view the descriptions via popup on the Incident page. https://github.com/burningmantech/ranger-ims-go/pull/241
- Added admin debugging pages to the server.
- Added link to #ranger-operations-center Slack channel from Incidents page in prod. https://github.com/burningmantech/ranger-ims-go/pull/296

### Removed

- Dropped the "show attached entries" checkbox on the Incident page. We now just always show attached entries. It was overly complex to allow toggling that. https://github.com/burningmantech/ranger-ims-go/pull/256

### Fixed

- Resolved a bunch of page load flickering problems. https://github.com/burningmantech/ranger-ims-go/pull/175 https://github.com/burningmantech/ranger-ims-go/pull/176 https://github.com/burningmantech/ranger-ims-go/pull/177
- Resolved DB contention problem with development in-process MySQL. https://github.com/burningmantech/ranger-ims-go/pull/282
- Fixed remote address logging. https://github.com/burningmantech/ranger-ims-go/pull/276
- Made keyboard shortcuts work in more cases. https://github.com/burningmantech/ranger-ims-go/pull/264
- Stopped padding radial hour numbers, allowing for simpler data entry (e.g. highlight field, press "5" to select 5, rather than needing to press "0" then "5"). https://github.com/burningmantech/ranger-ims-go/pull/255
- Restricted concurrent access to Argon2id code to stop AWS from killing the server. https://github.com/burningmantech/ranger-ims-go/issues/294 https://github.com/burningmantech/ranger-ims-go/pull/300 https://github.com/burningmantech/ranger-ims-go/pull/301
- Fixed regular expression searching handling of literal " ". https://github.com/burningmantech/ranger-ims-go/pull/289
- Corrected how we figure out a remote caller's IP address. https://github.com/burningmantech/ranger-ims-go/pull/276

## 2025-05

Abraham rewrote the IMS server in Go, with that new server living in https://github.com/burningmantech/ranger-ims-go.
This CHANGELOG now talks mainly about that replacement server.

### Changed

- Started opening incidents in the same tab by default. A user can still open incidents in new tabs though, using standard approaches (e.g. middle click, or right-click on incident number and click "open in new tab"). https://github.com/burningmantech/ranger-ims-go/issues/91 https://github.com/burningmantech/ranger-ims-go/pull/92
- Made a helpful and clean HTTPError struct/package to manage errors in the API layer. https://github.com/burningmantech/ranger-ims-go/pull/93
- Improved Incidents page load times with more concurrency. https://github.com/burningmantech/ranger-ims-go/pull/101

### Added

- Moved the prototype Go server into burningmantech: https://github.com/burningmantech/ranger-ims-go/pull/1
- Made a bunch of integration tests, which thoroughly test the whole round trip of interacting with IMS,
  from the API layer all the way down to the database. https://github.com/burningmantech/ranger-ims-go/tree/master/api/integration
- Started issuing refresh tokens to clients, and made access tokens have a much shorter validity. https://github.com/burningmantech/ranger-ims-go/pull/19 https://github.com/burningmantech/ranger-ims-go/pull/18
- Created a Dockerfile for the new server. The image with the new server is only about 30 MiB!
- Added database migration functionality, with tests that ensure that a migrated DB's schema matches the current schema, as it's specified in the repo.
- Created a threadsafe cache mechanism for storing Clubhouse data. https://github.com/burningmantech/ranger-ims-go/pull/58
- Added authorization testing for all current API endpoints. https://github.com/burningmantech/ranger-ims-go/pull/63
- Built incident/field report attachments functionality in the new server, which allows either local or S3 storage. It's fast and it works well! https://github.com/burningmantech/ranger-ims-go/issues/68
- Added a Makefile with some neat targets, like one to hot-reload a local IMS server on any code changes. https://github.com/burningmantech/ranger-ims-go/pull/102
- Added an in-process MySQL implementation, so that developers can run IMS locally without needing to run MariaDB. https://github.com/burningmantech/ranger-ims-go/pull/159 https://github.com/burningmantech/ranger-ims-go/pull/168

### Fixed

- Started doing an all-in-one statement to insert an incident/FR with a new number. Previously we did an
  unfortunate two-step process. https://github.com/burningmantech/ranger-ims-go/pull/100

## 2025-04

### Changed

- Dropped all dependence on the old TWISTED_SESSION cookie, which previously is how clients authenticated with the
  server. Now we're using an Authorization JWT instead. The main benefit to users is that they won't be "logged out"
  when an IMS server restarts; they'll just be able to carry on as soon as it's back on-line (this is how Clubhouse
  already
  works). https://github.com/burningmantech/ranger-ims-server/issues/1683 https://github.com/burningmantech/ranger-ims-server/issues/1682 https://github.com/burningmantech/ranger-ims-server/issues/1681 https://github.com/burningmantech/ranger-ims-server/issues/1678 https://github.com/burningmantech/ranger-ims-server/issues/1670
- Got rid of the two-stage templating of Incident(s)/Field Report(s) pages. Now there's just a single template each.
  This is an internal cleanup that makes things much
  simpler. https://github.com/burningmantech/ranger-ims-server/issues/1666
- Upgraded to modern JavaScript ES modules, which have a bunch of
  benefits. https://github.com/burningmantech/ranger-ims-server/issues/1660

### Fixed

- Changed some table keys to reduce chance of transaction deadlock (e.g. when multiple users are modifying any two
  incidents' Rangers lists at the same
  time). https://github.com/burningmantech/ranger-ims-server/issues/1689 https://github.com/burningmantech/ranger-ims-server/issues/1665 https://github.com/burningmantech/ranger-ims-server/issues/1658

## 2025-03

### Changed

- Upgraded all of our JavaScript to TypeScript. This makes for much safer frontend code, as well as much more pleasant
  development. https://github.com/burningmantech/ranger-ims-server/issues/1628
- Moved away from JQuery, toward modern browser JavaScript. https://github.com/burningmantech/ranger-ims-server/issues/1628 https://github.com/burningmantech/ranger-ims-server/pull/1636 https://github.com/burningmantech/ranger-ims-server/pull/1639 https://github.com/burningmantech/ranger-ims-server/pull/1642 https://github.com/burningmantech/ranger-ims-server/pull/1644

### Added

- Allowed creation of Streets via the Admin Streets
  page. https://github.com/burningmantech/ranger-ims-server/issues/1643
- Added some Playwright testing, which we've since been
  improving. https://github.com/burningmantech/ranger-ims-server/issues/1648
- Started adding support for file attachments on Incidents and Field
  Reports. https://github.com/burningmantech/ranger-ims-server/issues/7 https://github.com/burningmantech/ranger-ims-server/pull/1607

## 2025-02

<!-- TODO: document keyboard shortcut updates, once they've settled down a bit -->

### Changed

- Reordered columns on Incidents page for improved readability and mobile experience. The summary field is now much
  farther left. We also added a "last modified" column, which may or may not prove useful for
  people. https://github.com/burningmantech/ranger-ims-server/pull/1589 https://github.com/burningmantech/ranger-ims-server/pull/1595

### Added

- Started allowing searches using regular expressions on the Incidents and Field Reports pages, mostly to support "OR"
  -based queries. https://github.com/burningmantech/ranger-ims-server/issues/1570
- Created the ability to have a search query as part of an Incidents or Field Reports page URL. This allows
  bookmarking. https://github.com/burningmantech/ranger-ims-server/issues/1570
- Put all the table filters (state, type, rows, days-ago) into the URLs, making all of those bookmarkable, in addition
  to search. https://github.com/burningmantech/ranger-ims-server/issues/1570
- Converted the "show incident type" dropdown into a multiselect, allowing filtering the Incidents page to any number of
  Incident Types. https://github.com/burningmantech/ranger-ims-server/issues/1581
- Added "created" timestamp to Incident page, where "priority" used to
  be. https://github.com/burningmantech/ranger-ims-server/pull/1599
- Added team-based access control, e.g. to allow all members of Council to have year-round
  access. https://github.com/burningmantech/ranger-ims-server/pull/1587
- Created links in the navbar to the current event's Incidents and Field Reports
  pages. https://github.com/burningmantech/ranger-ims-server/pull/1580
- Added popup alert when user tries to close an incident that doesn't have any incident
  type. https://github.com/burningmantech/ranger-ims-server/pull/1600

### Removed

- Dropped Incident "priority" from the UI, since almost no one was using
  it. https://github.com/burningmantech/ranger-ims-server/issues/1574
- Removed the "show all" keyboard shortcut for Incidents and Field Reports pages, since the new bookmarkable filtered
  views make such a shortcut unnecessary. https://github.com/burningmantech/ranger-ims-server/issues/1570

### Fixed

- Prevented duplicate report entries from getting saved during periods of network
  latency. https://github.com/burningmantech/ranger-ims-server/pull/1593

## 2025-01

### Changed

- Sped up the incidents page background refresh by only refreshing the incident that changed. Previously we reloaded all
  the data from the server for the whole table on any update to any
  incident. https://github.com/burningmantech/ranger-ims-server/pull/1493
- Improved background refresh resiliency by having clients track their last received server-side
  event. https://github.com/burningmantech/ranger-ims-server/pull/1504
- Created a slight pause after search field input prior to actually running the search. This will reduce perceived
  latency in typing/deleting in the search
  field. https://github.com/burningmantech/ranger-ims-server/issues/1481 https://github.com/burningmantech/ranger-ims-server/pull/1483
- Started Field Report numbering from 1 each event, as we already did for
  Incidents. https://github.com/burningmantech/ranger-ims-server/pull/1506
- Enhanced the "permission denied" error page, to make it more descriptive for when we block access for an authenticated
  Ranger. https://github.com/burningmantech/ranger-ims-server/pull/1530
- Stopped showing Ranger legal names in IMS; started linking from Ranger handles into Clubhouse person pages
  instead. https://github.com/burningmantech/ranger-ims-server/issues/1536
- Switched from LocalStorage caching of Incident Types and Personnel to HTTP
  caching. https://github.com/burningmantech/ranger-ims-server/pull/1561

### Added

- Introduced "striking" of report entries. This allows a user to hide an outdated/inaccurate entry, such that it doesn't
  appear by default on the Incident or Field Report page. https://github.com/burningmantech/ranger-ims-server/issues/249
- Added help modals, toggled by pressing "?", which show keyboard shortcuts for the current
  page. https://github.com/burningmantech/ranger-ims-server/issues/1482
- Started publishing Field Report entity updates to the web clients (via server-sent events), and started automatically
  background-updating the Field Reports (table) and Field Report pages on
  updates. https://github.com/burningmantech/ranger-ims-server/issues/1498
- Also started background-updating the Incident page based on Field Report updates, so that the user shouldn't ever need
  to reload an Incident page to pick up updates from the
  server. https://github.com/burningmantech/ranger-ims-server/pull/1555
- Added a help link from the Incident page to documentation about the meaning of the many Incident
  Types. https://github.com/burningmantech/ranger-ims-server/pull/1512
- Added Subresource Integrity checks to our JavaScript dependencies, improving our security against supply chain
  attacks. https://github.com/burningmantech/ranger-ims-server/issues/1517
- Got rid of the "requireActive" global setting, and changed it into a "validity" property of each event permission
  instead. This allows us to specify which permissions should be granted all year, and which should only be available to
  Rangers while they are on-playa. https://github.com/burningmantech/ranger-ims-server/issues/1540

### Removed

- Dropped "\*\*"-style ACLs, which we didn't use and didn't actually work at
  all. https://github.com/burningmantech/ranger-ims-server/pull/1553
- Dropped lscache frontend
  dependency. https://github.com/burningmantech/ranger-ims-server/pull/1558 https://github.com/burningmantech/ranger-ims-server/pull/1561

### Fixed

- Removed confusing messaging from login screen when a user was already logged
  in. https://github.com/burningmantech/ranger-ims-server/pull/1511 https://github.com/burningmantech/ranger-ims-server/issues/1508
- Made the session cookie use more secure by adding a SameSite value and setting
  HttpOnly. https://github.com/burningmantech/ranger-ims-server/pull/1563

## 2024-12

### Changed

- Upgraded to Bootstrap 5 (from 3). This unlocks a bunch of neat new features. It required many minor UI
  changes https://github.com/burningmantech/ranger-ims-server/pull/1445
- Started collapsing the Instructions on the Field Report page by
  default https://github.com/burningmantech/ranger-ims-server/pull/1445
- Modernized the login screen a bit, by using form-floating input
  fields https://github.com/burningmantech/ranger-ims-server/pull/1445

### Added

- Added dark mode and a light/dark mode
  toggler https://github.com/burningmantech/ranger-ims-server/pull/1445 https://github.com/burningmantech/ranger-ims-server/issues/290
- Started showing the (currently read-only) IMS number on the Field Report
  page https://github.com/burningmantech/ranger-ims-server/pull/1429
- Added a button on the Field Report page that allows instant creation of a new Incident based on that Field Report.
  This button will only appear for users with writeIncident permission (e.g. Operators and Shift
  Command) https://github.com/burningmantech/ranger-ims-server/pull/1429

### Fixed

- Resolved a longstanding bug in which a user would be forced to log in twice in a short period of
  time. https://github.com/burningmantech/ranger-ims-server/pull/1456
- Fixed a glitch in which the placeholder text for the "Summary" field was never showing up on the Incident and Field
  Report pages. We simultaneously altered the Field Report summary placeholder to suggest the user include an IMS number
  in that field. https://github.com/burningmantech/ranger-ims-server/pull/1443

## 2024-11

### Changed

- Switched to text-fields with datalists for "Add Ranger" and "Add Incident Types" on the incident page. Previously we
  used select dropdowns, which were long and
  cumbersome https://github.com/burningmantech/ranger-ims-server/pull/1292, https://github.com/burningmantech/ranger-ims-server/pull/1365
- Stopped showing empty locations in the UI as "(?:?)@?", but rather as just an empty
  string https://github.com/burningmantech/ranger-ims-server/pull/1362
- Tightened security on the personnel endpoint, by restricting it to those with at least readIncident permission, and by
  removing Ranger email addresses from the
  response https://github.com/burningmantech/ranger-ims-server/pull/1355, https://github.com/burningmantech/ranger-ims-server/pull/1317
- Made the incidents table load another 2x faster, by not retrieving system-generated report entries as part of that
  call. This should make the table more responsive too https://github.com/burningmantech/ranger-ims-server/pull/1396
- Tweaked the navbar's formatting to align better with
  Clubhouse https://github.com/burningmantech/ranger-ims-server/pull/1394
- On incident page, for a brand-new incident, stopped scrolling down to focus on the "add entry" box. Instead the page
  will focus on the summary field in that case. https://github.com/burningmantech/ranger-ims-server/pull/1419
- Started hiding system-generated incident history by default on the incident page. This can still be toggled by the "
  show history" checkbox, but the default is now that this is
  unchecked https://github.com/burningmantech/ranger-ims-server/pull/1421

### Added

- Added an Incident Type filter to the Incidents page https://github.com/burningmantech/ranger-ims-server/pull/1401
- Added full Unicode support to IMS. All text fields now accept previously unsupported characters, like those from
  Cyrillic, Chinese, emoji, and much more https://github.com/burningmantech/ranger-ims-server/issues/1353
- Started doing client-side retries on any EventSource connection failures. This should mean that an IMS web session
  will be better kept in sync with incident updates, in particular in the off-season, when IMS is running on
  AWS https://github.com/burningmantech/ranger-ims-server/pull/1389
- Added a warning banner to non-production instances of the web UI, to make sure people don't accidentally put prod data
  into non-production IMS instances. https://github.com/burningmantech/ranger-ims-server/issues/1366
- Started showing full datetimes, including time zone, when a user hovers over a time on the incidents page. All times
  have always been in the user's locale, but this wasn't indicated
  anywhere https://github.com/burningmantech/ranger-ims-server/pull/1412

### Removed

- Got rid of Moment.js dependency, as it's deprecated and we're able to use the newer Intl JavaScript browser construct
  instead https://github.com/burningmantech/ranger-ims-server/pull/1412

### Fixed

- Made incident printouts look much
  better https://github.com/burningmantech/ranger-ims-server/pull/1382, https://github.com/burningmantech/ranger-ims-server/pull/1405
- Fixed bug that caused the incident page to reload data multiple times for each incident
  update https://github.com/burningmantech/ranger-ims-server/issues/1369
- Uncovered and resolved some subtle XSS vulnerabilities https://github.com/burningmantech/ranger-ims-server/pull/1402

## 2024-10

### Changed

- Resolved IMS's longstanding 6-open-tab limitation, by using a BroadcastChannel to share one EventSource connection
  between
  tabs https://github.com/burningmantech/ranger-ims-server/issues/1320, https://github.com/burningmantech/ranger-ims-server/pull/1322
- Changed login screen to encourage users to log in by email address, rather than by Ranger
  handle https://github.com/burningmantech/ranger-ims-server/pull/1293
- Optimized the API calls that back the incidents endpoint. This speeds up the web UI incidents table load by around
  3x https://github.com/burningmantech/ranger-ims-server/pull/1349, https://github.com/burningmantech/ranger-ims-server/issues/1324

### Added

- Added groupings to the "add Field Report" dropdown, which emphasize which Field Reports are or are not attached to any
  other incident. This also simplified the sort order for that
  list https://github.com/burningmantech/ranger-ims-server/pull/1321

### Fixed

- Stopped using hardcoded 1-hour duration limit on IMS sessions, allowing us to make sessions that will last for a whole
  shift on playa. https://github.com/burningmantech/ranger-ims-server/pull/1301
- Got rid of the browser popup alerts that occurred frequently on JavaScript errors. Instead, error messages will now be
  written to a text field near the top of each page https://github.com/burningmantech/ranger-ims-server/pull/1335

## 2024-01

### Added

- Added "Changes you made may not be saved" browser popup when a user might otherwise lose data on incident entries and
  Field Report entries https://github.com/burningmantech/ranger-ims-server/pull/1088

### Fixed

- Do case-insensitive sorting of Ranger handles on incident
  page https://github.com/burningmantech/ranger-ims-server/pull/1089
