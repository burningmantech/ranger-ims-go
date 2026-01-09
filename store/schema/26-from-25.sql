alter table `INCIDENT__RANGER`
add column `ROLE` varchar(128);

update `SCHEMA_INFO`
set `VERSION` = 26
where true;
