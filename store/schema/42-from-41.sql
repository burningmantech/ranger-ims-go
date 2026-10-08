/* Index the Ranger rosters by handle.

   Attaching or detaching a Ranger deletes that Ranger's row by
   (EVENT, NUMBER, RANGER_HANDLE). Indexed only by (EVENT, NUMBER), that delete
   locked every Ranger on the roster, and since MariaDB 11.6.2 turned on
   innodb_snapshot_isolation, any of those rows that a concurrent request had
   just written fails the whole transaction. With the handle in the index, it
   touches only the row it's after. */

alter table `INCIDENT__RANGER`
    add key `INCIDENT__RANGER_EVENT_INCIDENT_NUMBER_RANGER_HANDLE_index` (`EVENT`, `INCIDENT_NUMBER`, `RANGER_HANDLE`),
    drop key `INCIDENT__RANGER_EVENT_INCIDENT_NUMBER_index`;

alter table `VISIT__RANGER`
    add key `VISIT__RANGER_EVENT_VISIT_NUMBER_RANGER_HANDLE_index` (`EVENT`, `VISIT_NUMBER`, `RANGER_HANDLE`);

update `SCHEMA_INFO`
set `VERSION` = 42
where true;
