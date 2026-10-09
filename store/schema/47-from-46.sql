/* Index the log tables' CREATED_AT, which the admin pages' time-range queries
   filter on and which otherwise means a scan of every logged request. */

alter table `ACTION_LOG`
    add key `CREATED_AT` (`CREATED_AT`);

alter table `ERROR_LOG`
    add key `CREATED_AT` (`CREATED_AT`);

update `SCHEMA_INFO`
set `VERSION` = 47
where true;
