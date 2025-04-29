-- name: RangersById :many
select
    id,
    callsign,
    email,
    status,
    on_site,
    password
from person
where status in ('active', 'inactive', 'inactive extension', 'auditor');

-- name: Positions :many
select id, title from position where all_rangers = 0;

-- name: Teams :many
select id, title from team where active;

-- name: PersonPositions :many
select person_id, position_id from person_position;

-- name: PersonTeams :many
select person_id, team_id from person_team;
