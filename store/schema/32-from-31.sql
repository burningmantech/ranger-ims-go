alter table `EVENT_ACCESS`
    change column `EXPIRES` `NOT_AFTER` double,
    add column `NOT_BEFORE` double after `NOT_AFTER`;

update `SCHEMA_INFO`
set `VERSION` = 32
where true;
