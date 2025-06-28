alter table `ACTION_LOG`
drop column `CREATED_AT`,
add column `CREATED_AT` double not null after `ID`;

update `SCHEMA_INFO`
set `VERSION` = 19
where true;
