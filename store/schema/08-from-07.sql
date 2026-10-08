/* this migration is about renaming INCIDENT_REPORT to FIELD_REPORT */

alter table `INCIDENT_REPORT__REPORT_ENTRY` rename column `INCIDENT_REPORT_NUMBER` to `FIELD_REPORT_NUMBER`, rename to `FIELD_REPORT__REPORT_ENTRY`;
alter table `INCIDENT_REPORT` rename to `FIELD_REPORT`;

/* Before MariaDB 12.1, renaming a table also renamed its auto-named foreign keys
   (INCIDENT_REPORT_ibfk_1 became FIELD_REPORT_ibfk_1). Since then it doesn't, so
   do that by hand. Both steps are no-ops on a database migrated by an older MariaDB.
   INCIDENT_REPORT__REPORT_ENTRY_ibfk_2 is left alone, since 10-from-09 drops it. */

alter table `FIELD_REPORT`
    drop foreign key if exists `INCIDENT_REPORT_ibfk_1`,
    drop foreign key if exists `INCIDENT_REPORT_ibfk_2`;
alter table `FIELD_REPORT`
    add constraint `FIELD_REPORT_ibfk_1`
        foreign key if not exists (`EVENT`) references `EVENT` (`ID`),
    add constraint `FIELD_REPORT_ibfk_2`
        foreign key if not exists (`EVENT`, `INCIDENT_NUMBER`) references `INCIDENT` (`EVENT`, `NUMBER`);

alter table `FIELD_REPORT__REPORT_ENTRY`
    drop foreign key if exists `INCIDENT_REPORT__REPORT_ENTRY_ibfk_1`,
    drop foreign key if exists `INCIDENT_REPORT__REPORT_ENTRY_ibfk_3`;
alter table `FIELD_REPORT__REPORT_ENTRY`
    add constraint `FIELD_REPORT__REPORT_ENTRY_ibfk_1`
        foreign key if not exists (`EVENT`) references `EVENT` (`ID`),
    add constraint `FIELD_REPORT__REPORT_ENTRY_ibfk_3`
        foreign key if not exists (`REPORT_ENTRY`) references `REPORT_ENTRY` (`ID`);

/* Update schema version */

update `SCHEMA_INFO` set `VERSION` = 8;
