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

-- name: PersonsOnDutyNow :many
select
    person_id,
    position_id
from
    timesheet
where
    on_duty > date_sub(now(), interval 60 day)
    and off_duty is null
-- Someone with more than one open shift (a missed sign-out) ends up in the
-- newest one, as in PersonsOnDutyThen.
order by
    on_duty
;

-- name: PersonsOnDutyThen :many
select
    person_id,
    position_id
from
    timesheet
where
    on_duty <= cast(sqlc.arg(at) as datetime)
    and (
        off_duty is null
        or off_duty > cast(sqlc.arg(at) as datetime)
    )
order by
    on_duty
;

-- name: Positions :many
select id, title, all_rangers from position;

-- name: Teams :many
select id, title from team where active;

-- name: PersonPositions :many
select person_id, position_id from person_position;

-- name: PersonTeams :many
select person_id, team_id from person_team;
