/* Drop foreign keys to EVENT that a table's (EVENT, NUMBER) foreign key to
   its parent record already implies. FIELD_REPORT and VISIT keep theirs,
   since their INCIDENT_NUMBER is nullable. */

alter table `INCIDENT__RANGER`
    drop foreign key `INCIDENT__RANGER_ibfk_1`;

alter table `INCIDENT__INCIDENT_TYPE`
    drop foreign key `INCIDENT__INCIDENT_TYPE_ibfk_1`;

alter table `INCIDENT__REPORT_ENTRY`
    drop foreign key `INCIDENT__REPORT_ENTRY_ibfk_1`;

alter table `FIELD_REPORT__REPORT_ENTRY`
    drop foreign key `FIELD_REPORT__REPORT_ENTRY_ibfk_1`;

alter table `VISIT__REPORT_ENTRY`
    drop foreign key `VRE_TO_EVENT`;

alter table `VISIT__RANGER`
    drop foreign key `VISIT__RANGER_ibfk_1`;

update `SCHEMA_INFO`
set `VERSION` = 45
where true;
