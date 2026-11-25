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

-- name: PodsOnDutyThen :many
-- A pod with a mentor_pod_id is grouped into that mentor pod, which lends the
-- group its details. Whether the mentor pod itself is out doesn't matter.
select
    grp.id as pod_id,
    grp.type,
    grp.transport,
    grp.location,
    -- Someone with two open shifts is in here twice. sqlc can't parse
    -- json_arrayagg(distinct ...), so the Go code dedupes them.
    json_arrayagg(person_pod.person_id) as person_ids
from pod
    join pod as grp on grp.id = coalesce(pod.mentor_pod_id, pod.id)
    join slot       on slot.id = pod.slot_id
    join person_pod on person_pod.pod_id = pod.id
    join timesheet  on timesheet.person_id = person_pod.person_id
where slot.begins <= cast(sqlc.arg(at) as datetime)
    and slot.ends   >  cast(sqlc.arg(at) as datetime)
    and (
        pod.disbanded_at is null
        or pod.disbanded_at > cast(sqlc.arg(at) as datetime)
    )
    and person_pod.joined_at <= cast(sqlc.arg(at) as datetime)
    and (
        person_pod.left_at is null
        or person_pod.left_at > cast(sqlc.arg(at) as datetime)
    )
    and (
        person_pod.removed_at is null
        or person_pod.removed_at > cast(sqlc.arg(at) as datetime)
    )
    and timesheet.on_duty <= cast(sqlc.arg(at) as datetime)
    and (
        timesheet.off_duty is null
        or timesheet.off_duty > cast(sqlc.arg(at) as datetime)
    )
group by grp.id, grp.type, grp.transport, grp.location
-- Oldest first, by the newest pod in each group.
order by max(pod.id)
;

-- name: Positions :many
select id, title, all_rangers from position;

-- name: Teams :many
select id, title from team where active;

-- name: PersonPositions :many
select person_id, position_id from person_position;

-- name: PersonTeams :many
select person_id, team_id from person_team;
