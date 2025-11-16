alter table `EVENT`
add column `IS_GROUP` boolean not null default false,
add column `PARENT_GROUP` integer,
add foreign key `PARENT_GROUP_TO_PARENT`(PARENT_GROUP) references `EVENT`(ID);

update `SCHEMA_INFO`
set `VERSION` = 25
where true;
