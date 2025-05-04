/**
 *  This migration is designed to get the prod DB back
 *  in sync with the state of current.sql
 */

# This got out of sync because changing from latin1 to utf8mb4 encoding
# also changes TEXT columns into MEDIUMTEXT ones.
alter table `REPORT_ENTRY`
    modify column `TEXT` mediumtext not null
;

alter table `EVENT_ACCESS`
    drop foreign key if exists `event_access_ibfk_1`,
    add constraint `EVENT_ACCESS_TO_EVENT`
        foreign key (`EVENT`) references `EVENT` (`ID`)
;

alter table `FIELD_REPORT__REPORT_ENTRY`
    drop foreign key if exists `FIELD_REPORT__REPORT_ENTRY_ibfk_3`,
    drop key if exists `REPORT_ENTRY`,
    add constraint `FR_REPORT_ENTRY_TO_REPORT_ENTRY`
        foreign key (`REPORT_ENTRY`) references `REPORT_ENTRY` (`ID`)
;

/* Update schema version */
update `SCHEMA_INFO` set `VERSION` = 14;
