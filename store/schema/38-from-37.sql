/* Add a per-event switch for automatic Black Rock City address normalization.

   When true, addresses the client sends for an Incident's location or a
   Visit's guest camp are rewritten into canonical form ("7+e" becomes
   "7:00 & E") before being stored. It's a per-event column rather than a
   config setting so that it can be flipped from the admin UI without a
   server restart. */

alter table `EVENT` add column `NORMALIZE_ADDRESSES` boolean not null default false;

update `SCHEMA_INFO`
set `VERSION` = 38
where true;
