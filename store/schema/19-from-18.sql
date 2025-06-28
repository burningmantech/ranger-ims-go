alter table `ACTION_LOG`
drop column `CREATED_AT`,
add column `CREATED_AT` double not null after `ID`;

-- do this to remove any "CREATED_AT" values of zero.
-- we haven't pushed ACTION_LOG to prod yet anyway, so
-- there's no loss of data there (just in staging).
-- there's no loss of data there (just in staging).
truncate table `ACTION_LOG`;

update `SCHEMA_INFO`
set `VERSION` = 19
where true;
