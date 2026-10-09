/* Key the Ranger roster tables by (EVENT, NUMBER, RANGER_HANDLE) instead of an
   auto-increment ID, so that setting a Ranger's role can be one upsert on the
   primary key rather than a delete and reinsert through a non-unique index,
   which locked index gaps that neighboring records' roster writes needed.

   Nothing stopped duplicate rows before, and RANGER_HANDLE's collation is
   case-insensitive, so "Tool" and "tool" count as duplicates too. Rather than
   pick which to delete, fail before altering either table, so that the
   duplicates can be resolved by hand and the migration rerun. */

if exists (
    select 1 from `INCIDENT__RANGER`
    group by `EVENT`, INCIDENT_NUMBER, RANGER_HANDLE
    having count(*) > 1
) or exists (
    select 1 from `VISIT__RANGER`
    group by `EVENT`, VISIT_NUMBER, RANGER_HANDLE
    having count(*) > 1
) then
    signal sqlstate '45000' set message_text =
        'INCIDENT__RANGER or VISIT__RANGER has duplicate (EVENT, NUMBER, RANGER_HANDLE) rows. Remove them and rerun the migration.';
end if;

alter table `INCIDENT__RANGER`
    drop primary key,
    drop column ID,
    add primary key (`EVENT`, INCIDENT_NUMBER, RANGER_HANDLE),
    drop key `INCIDENT__RANGER_EVENT_INCIDENT_NUMBER_RANGER_HANDLE_index`;

alter table `VISIT__RANGER`
    drop primary key,
    drop column ID,
    add primary key (`EVENT`, VISIT_NUMBER, RANGER_HANDLE),
    drop key `VISIT__RANGER_EVENT_VISIT_NUMBER_RANGER_HANDLE_index`;

update `SCHEMA_INFO`
set `VERSION` = 46
where true;
