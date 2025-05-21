alter table `INCIDENT`
    add column `STARTED` double after `STATE`
;

update `INCIDENT`
    set `STARTED` = `CREATED`
;

alter table `INCIDENT`
    modify column `STARTED` double not null;

/* Update schema version */
update `SCHEMA_INFO` set `VERSION` = 15;
