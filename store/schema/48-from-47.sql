/* Mark erroneous or duplicate Incidents, Field Reports, and Visits as junk with
   a flag on the record itself, which the list pages hide by default.

   Incidents used to be marked with a "Junk" Incident Type instead, which hid
   nothing and had no counterpart for Field Reports or Visits. Carry those
   Incidents over to the flag, then drop the type. */

alter table `INCIDENT` add column `JUNK` boolean not null default false;
alter table `FIELD_REPORT` add column `JUNK` boolean not null default false;
alter table `VISIT` add column `JUNK` boolean not null default false;

update `INCIDENT` i
set i.`JUNK` = true
where exists (
    select 1
    from `INCIDENT__INCIDENT_TYPE` iit
        join `INCIDENT_TYPE` it
            on it.ID = iit.INCIDENT_TYPE
    where iit.`EVENT` = i.`EVENT`
        and iit.INCIDENT_NUMBER = i.NUMBER
        and it.NAME = 'Junk'
);

delete iit
from `INCIDENT__INCIDENT_TYPE` iit
    join `INCIDENT_TYPE` it
        on it.ID = iit.INCIDENT_TYPE
where it.NAME = 'Junk';

delete from `INCIDENT_TYPE` where NAME = 'Junk';

-- Matches a fresh install's counter, where Junk was never inserted. InnoDB
-- raises this to one past the highest remaining ID.
alter table `INCIDENT_TYPE` auto_increment = 1;

update `SCHEMA_INFO`
set `VERSION` = 48
where true;
