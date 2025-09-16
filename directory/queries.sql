-- name: Persons :many
select
    id,
    callsign,
    email,
    status,
    on_site,
    password
from person
-- Filter persons to those with statuses that may be of interest to IMS.
-- These Persons results are used to determine who may log into IMS, and also
-- to determine who shows up in the Incident page's "Add Ranger" section.
--
-- This intentionally excludes statuses like bonked, echelon, deceased, resigned, and retired,
-- as those persons should not be able to log into IMS nor be granted any permissions in IMS.
where status in ('active', 'inactive', 'inactive extension', 'auditor', 'prospective', 'alpha');

-- name: PersonsOnDuty :many
select
    person_id,
    position_id
from
    timesheet
where
    on_duty > date_sub(now(), interval 60 day)
    and off_duty is null
;

-- name: Positions :many
select id, title from position where all_rangers = 0;

-- name: Teams :many
select id, title from team where active;

-- name: PersonPositions :many
select person_id, position_id from person_position;

-- name: PersonTeams :many
select person_id, team_id from person_team;
