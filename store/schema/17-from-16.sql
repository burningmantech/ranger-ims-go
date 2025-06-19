alter table `INCIDENT_TYPE`
add column `DESCRIPTION` varchar(1024);

update `INCIDENT_TYPE`
set `DESCRIPTION` = 'Administrative information; not an actual incident in the field. Used for tracking informational calls such as "Tool is in a meeting until 1500" or other administrative details that are relevant to IMS users but not an actual incident. This also includes things like tech outages. These incidents are excluded from reporting.'
where `NAME` = 'Admin';

update `INCIDENT_TYPE`
set `DESCRIPTION` = 'This incident is garbage or is a duplicate of another incident. Ignore it. Because we do not delete incidents, this type is used to filter out incidents that one may otherwise would want to delete.'
where `NAME` = 'Junk';

update `SCHEMA_INFO`
set `VERSION` = 17
where true;
