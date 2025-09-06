alter table `EVENT_ACCESS`
add column `EXPIRES` double after `VALIDITY`;

update `SCHEMA_INFO`
set `VERSION` = 20
where true;
