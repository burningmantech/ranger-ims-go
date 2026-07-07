/* Remove the legacy structured location fields (concentric street, radial hour/minute).

   Free-text LOCATION_ADDRESS replaced these in migration 24. */

alter table `INCIDENT` drop constraint `INCIDENT_ibfk_2`;

/* Drop the index that was auto-created for INCIDENT_ibfk_2. The INCIDENT_ibfk_1
   foreign key on EVENT is still covered by the (EVENT, NUMBER) primary key. */
alter table `INCIDENT` drop index `EVENT`;

alter table `INCIDENT`
    drop column `LOCATION_CONCENTRIC`,
    drop column `LOCATION_RADIAL_HOUR`,
    drop column `LOCATION_RADIAL_MINUTE`
;

drop table CONCENTRIC_STREET;

update `SCHEMA_INFO`
set `VERSION` = 35
where true;
