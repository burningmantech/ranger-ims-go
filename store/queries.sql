-- name: QueryEventID :one
select sqlc.embed(e) from EVENT e where e.NAME = ?;

-- name: SchemaVersion :one
select VERSION from SCHEMA_INFO;

-- name: Event :one
select sqlc.embed(e) from EVENT e where ID = ?;

-- name: Events :many
select sqlc.embed(e) from EVENT e;

-- name: CreateEvent :execlastid
insert into EVENT (NAME, IS_GROUP, PARENT_GROUP) values (?, ?, ?);

-- name: UpdateEvent :exec
update `EVENT`
set
    NAME = ?,
    IS_GROUP = ?,
    PARENT_GROUP = ?
where ID = ?
;

-- This returns access for a target event, as well as for that event's
-- parent group, if any. If the target event *is* a group, this query
-- will return nothing. That's intentional, and it helps prevent people
-- from adding incidents or FRs to event groups as though those were events.
-- name: EventAndParentAccess :many
select sqlc.embed(ea)
from `EVENT` e
    join EVENT_ACCESS ea
        on e.ID = ea.EVENT
where e.ID = sqlc.arg(event_id)
    and not e.IS_GROUP
union all
select sqlc.embed(ea)
from `EVENT` e
    join EVENT_ACCESS ea
        on e.PARENT_GROUP = ea.EVENT
where e.ID = sqlc.arg(event_id)
    and e.PARENT_GROUP is not null
;


-- name: EventAccessAll :many
select sqlc.embed(ea)
from EVENT_ACCESS ea
;

-- name: ClearEventAccessForMode :exec
delete from EVENT_ACCESS
where EVENT = ? and MODE = ?;

-- name: ClearEventAccessForExpression :exec
delete from EVENT_ACCESS
where EVENT = ? and EXPRESSION = ?;

-- name: AddEventAccess :execlastid
insert into EVENT_ACCESS (EVENT, EXPRESSION, MODE, VALIDITY, EXPIRES)
values (?, ?, ?, ?, ?);

-- name: CreateIncident :execlastid
insert into INCIDENT (
    EVENT,
    NUMBER,
    CREATED,
    PRIORITY,
    STATE,
    STARTED
)
values (
   ?,?,?,?,?,?
);

-- name: UpdateIncident :exec
update INCIDENT set
    -- CREATED should be immutable, so it's not present in this UPDATE query
    PRIORITY = ?,
    STATE = ?,
    STARTED = ?,
    CLOSED = ?,
    SUMMARY = ?,
    LOCATION_NAME = ?,
    LOCATION_ADDRESS = ?,
    LOCATION_CONCENTRIC = ?,
    LOCATION_RADIAL_HOUR = ?,
    LOCATION_RADIAL_MINUTE = ?,
    LOCATION_DESCRIPTION = ?
where
    EVENT = ?
    and NUMBER = ?
;

-- name: Incident :one
select
    sqlc.embed(i),
    (
        select coalesce(json_arrayagg(iit.INCIDENT_TYPE), "[]")
        from INCIDENT__INCIDENT_TYPE iit
        where i.EVENT = iit.EVENT
          and i.NUMBER = iit.INCIDENT_NUMBER
    ) as INCIDENT_TYPE_IDS,
    (
        select coalesce(json_arrayagg(irep.NUMBER), "[]")
        from FIELD_REPORT irep
        where i.EVENT = irep.EVENT
          and i.NUMBER = irep.INCIDENT_NUMBER
    ) as FIELD_REPORT_NUMBERS
from INCIDENT i
where i.EVENT = ?
    and i.NUMBER = ?;

-- name: Incidents :many
select
    sqlc.embed(i),
    (
        select coalesce(json_arrayagg(iit.INCIDENT_TYPE), "[]")
        from INCIDENT__INCIDENT_TYPE iit
        where i.EVENT = iit.EVENT
            and i.NUMBER = iit.INCIDENT_NUMBER
    ) as INCIDENT_TYPE_IDS,
    (
        select coalesce(json_arrayagg(irep.NUMBER), "[]")
        from FIELD_REPORT irep
        where i.EVENT = irep.EVENT
            and i.NUMBER = irep.INCIDENT_NUMBER
    ) as FIELD_REPORT_NUMBERS
from
    INCIDENT i
where
    i.EVENT = ?
group by
    i.NUMBER;

-- name: Incidents_Rangers :many
select
    sqlc.embed(ir)
from
    INCIDENT__RANGER ir
where
    ir.EVENT = ?;

-- name: Incident_Rangers :many
select
    sqlc.embed(ir)
from
    INCIDENT__RANGER ir
where
    ir.EVENT = ?
    and ir.INCIDENT_NUMBER = ?;

-- name: Incident_LinkedIncidents :many
select
    ili.EVENT_2 as LINKED_EVENT,
    e.NAME as LINKED_EVENT_NAME,
    ili.INCIDENT_NUMBER_2 as LINKED_INCIDENT,
    i2.SUMMARY as LINKED_INCIDENT_SUMMARY
from
    INCIDENT__LINKED_INCIDENT ili
    join `EVENT` e
        on e.ID = ili.EVENT_2
    join INCIDENT i2
        on i2.EVENT = ili.EVENT_2
            and i2.NUMBER = ili.INCIDENT_NUMBER_2
where
    ili.EVENT_1 = ?
    and ili.INCIDENT_NUMBER_1 = ?
;

-- name: Incidents_ReportEntries :many
select
    ire.INCIDENT_NUMBER,
    sqlc.embed(re)
from
    INCIDENT__REPORT_ENTRY ire
        join REPORT_ENTRY re
             on re.ID = ire.REPORT_ENTRY
where
    ire.EVENT = ?
    and re.GENERATED <= ?
;

-- name: Incident_ReportEntries :many
select
    ire.INCIDENT_NUMBER,
    sqlc.embed(re)
from
    INCIDENT__REPORT_ENTRY ire
        join REPORT_ENTRY re
             on re.ID = ire.REPORT_ENTRY
where
    ire.EVENT = ?
    and ire.INCIDENT_NUMBER = ?
;

-- name: ConcentricStreets :many
select sqlc.embed(cs)
from CONCENTRIC_STREET cs
where cs.EVENT = ?;

-- name: IncidentTypes :many
select sqlc.embed(it)
from INCIDENT_TYPE it;

-- name: IncidentType :one
select sqlc.embed(it)
from INCIDENT_TYPE it
where it.ID = ?;

-- name: FieldReports :many
select sqlc.embed(fr)
from FIELD_REPORT fr
where fr.EVENT = ?;

-- name: FieldReport :one
select sqlc.embed(fr)
from FIELD_REPORT fr
where fr.EVENT = ?
    and fr.NUMBER = ?;

-- name: FieldReports_ReportEntries :many
select
    irre.FIELD_REPORT_NUMBER,
    sqlc.embed(re)
from
    FIELD_REPORT__REPORT_ENTRY irre
        join REPORT_ENTRY re
             on irre.REPORT_ENTRY = re.ID
where
    irre.EVENT = ?
    and re.GENERATED <= ?
;

-- name: FieldReport_ReportEntries :many
select
    sqlc.embed(re)
from
    FIELD_REPORT__REPORT_ENTRY irre
        join REPORT_ENTRY re
             on irre.REPORT_ENTRY = re.ID
where
    irre.EVENT = ?
    and irre.FIELD_REPORT_NUMBER = ?
;

-- name: AttachFieldReportToIncident :exec
update FIELD_REPORT
set INCIDENT_NUMBER = ?
where EVENT = ? and NUMBER = ?
;

-- This doesn't use "MAX" because sqlc can't figure out the type for aggregations :(.
-- name: NextFieldReportNumber :one
select NUMBER + 1 as NEXT_ID
from FIELD_REPORT
where EVENT = ?
union
select 1
order by 1 desc
limit 1;

-- This doesn't use "MAX" because sqlc can't figure out the type for aggregations :(.
-- name: NextIncidentNumber :one
select NUMBER + 1 as NEXT_ID
from INCIDENT
where EVENT = ?
union
select 1
order by 1 desc
limit 1;

-- name: CreateFieldReport :exec
insert into FIELD_REPORT (
    EVENT, NUMBER, CREATED, SUMMARY, INCIDENT_NUMBER
)
values (?, ?, ?, ?, ?);

-- name: UpdateFieldReport :exec
update FIELD_REPORT
set SUMMARY = ?, INCIDENT_NUMBER = ?
where EVENT = ? and NUMBER = ?;

-- name: CreateReportEntry :execlastid
insert into REPORT_ENTRY (
    AUTHOR, TEXT, CREATED, `GENERATED`, STRICKEN,
    ATTACHED_FILE, ATTACHED_FILE_ORIGINAL_NAME, ATTACHED_FILE_MEDIA_TYPE
) values (
   ?, ?, ?, ?, ?, ?, ?, ?
);

-- name: AttachReportEntryToFieldReport :exec
insert into FIELD_REPORT__REPORT_ENTRY (
    EVENT, FIELD_REPORT_NUMBER, REPORT_ENTRY
) values (
    ?, ?, ?
);

-- name: AttachReportEntryToIncident :exec
insert into INCIDENT__REPORT_ENTRY (
    EVENT, INCIDENT_NUMBER, REPORT_ENTRY
) values (
    ?, ?, ?
);

--
-- The "stricken" queries seem bloated at first blush, because the whole
-- "where ID in (..." could just be "where ID =". What it's doing though is
-- ensuring that the provided eventID and incidentNumber actually align with
-- the reportEntryID in question, and that's important for authorization purposes.
--

-- name: SetIncidentReportEntryStricken :exec
update REPORT_ENTRY
set STRICKEN = ?
where ID IN (
    select REPORT_ENTRY
    from INCIDENT__REPORT_ENTRY
    where EVENT = ?
        and INCIDENT_NUMBER = ?
        and REPORT_ENTRY = ?
);

-- name: SetFieldReportReportEntryStricken :exec
update REPORT_ENTRY
set STRICKEN = ?
where ID IN (
    select REPORT_ENTRY
    from FIELD_REPORT__REPORT_ENTRY
    where EVENT = ?
      and FIELD_REPORT_NUMBER = ?
      and REPORT_ENTRY = ?
);

-- name: AttachRangerHandleToIncident :exec
insert into INCIDENT__RANGER (EVENT, INCIDENT_NUMBER, RANGER_HANDLE, ROLE)
values (?, ?, ?, ?);

-- name: DetachRangerHandleFromIncident :exec
delete from INCIDENT__RANGER
where
    EVENT = ?
    and INCIDENT_NUMBER = ?
    and RANGER_HANDLE = ?
;

-- name: LinkIncidents :exec
insert into INCIDENT__LINKED_INCIDENT
    (EVENT_1, INCIDENT_NUMBER_1, EVENT_2, INCIDENT_NUMBER_2)
values
    (?, ?, ?, ?)
;

-- name: UnlinkIncidents :exec
delete from INCIDENT__LINKED_INCIDENT
where
    EVENT_1 = ?
    and INCIDENT_NUMBER_1 = ?
    and EVENT_2 = ?
    and INCIDENT_NUMBER_2 = ?
;

-- name: AttachIncidentTypeToIncident :exec
insert into INCIDENT__INCIDENT_TYPE (
    EVENT, INCIDENT_NUMBER, INCIDENT_TYPE
) values (
     ?, ?, ?
 );

-- name: DetachIncidentTypeFromIncident :exec
delete from INCIDENT__INCIDENT_TYPE
where
    EVENT = ?
    and INCIDENT_NUMBER = ?
    and INCIDENT_TYPE = ?
;


-- name: CreateIncidentType :execlastid
insert into INCIDENT_TYPE (NAME, HIDDEN)
values (?, ?)
;

-- name: UpdateIncidentType :exec
update INCIDENT_TYPE
set HIDDEN = ?,
    NAME = ?,
    DESCRIPTION = ?
where ID = ?;

-- name: CreateConcentricStreet :exec
insert into CONCENTRIC_STREET (EVENT, ID, NAME)
values (?, ?, ?);

-- name: AddActionLog :execlastid
insert into ACTION_LOG
    (CREATED_AT, ACTION_TYPE, METHOD, PATH, REFERRER, USER_ID, USER_NAME, POSITION_ID, POSITION_NAME, CLIENT_ADDRESS, HTTP_STATUS, DURATION_MICROS)
values
    (?,?,?,?,?,?,?,?,?,?,?,?)
;

-- name: ActionLogs :many
select
    sqlc.embed(al)
from
    ACTION_LOG al
where
    al.CREATED_AT > sqlc.arg(min_time)
    and al.CREATED_AT < sqlc.arg(max_time)
;

-- name: CreateDestination :exec
insert into DESTINATION
    (EVENT, NUMBER, TYPE, NAME, LOCATION_STRING, EXTERNAL_DATA)
values
    (?,?,?,?,?,?)
;

-- name: RemoveDestinations :exec
delete from
    DESTINATION
where EVENT = ?
    and TYPE = ?
;

-- name: Destinations :many
select
    EVENT,
    TYPE,
    NUMBER,
    NAME,
    LOCATION_STRING,
    if(sqlc.arg(exclude_external_data), '', EXTERNAL_DATA) as EXTERNAL_DATA
from
    DESTINATION d
where
    EVENT = ?
;
