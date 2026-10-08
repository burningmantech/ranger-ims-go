/* Let one event be marked active: the one people most likely care about,
   because it's happening soon, is happening now, or happened most recently. */

alter table `EVENT`
    add column `IS_ACTIVE` boolean not null default false after `NORMALIZE_ADDRESSES`;

update `SCHEMA_INFO`
set `VERSION` = 43
where true;
