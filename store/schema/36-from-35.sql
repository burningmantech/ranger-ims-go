/* Optimistic-concurrency version counters, surfaced to clients as ETags.

   Bumped on every change to a record's representation other than report-entry
   changes. Report entries are append-only except for the STRICKEN flag, a
   reversible boolean toggle that is audited via generated entries — neither
   appends nor strikes can silently lose data, so neither moves the version. */

alter table `INCIDENT`     add column `VERSION` integer not null default 1;
alter table `FIELD_REPORT` add column `VERSION` integer not null default 1;
alter table `VISIT`        add column `VERSION` integer not null default 1;

update `SCHEMA_INFO`
set `VERSION` = 36
where true;
