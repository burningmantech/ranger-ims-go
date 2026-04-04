alter table `DESTINATION`
modify column `TYPE` enum('camp', 'art', 'other', 'mv') not null;

update `SCHEMA_INFO`
set `VERSION` = 28
where true;
