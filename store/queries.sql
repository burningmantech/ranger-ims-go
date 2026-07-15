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
    PARENT_GROUP = ?,
    MAP_URL = ?
where ID = ?
;

-- The DeleteEvent* queries below support full deletion of an Event and all
-- rows associated with it. They must run in the order used by the DeleteEvent
-- API handler, so that no foreign key constraint is violated along the way.

-- name: EventReportEntryIDs :many
select ire.REPORT_ENTRY from INCIDENT__REPORT_ENTRY ire where ire.EVENT = sqlc.arg(event_id)
union
select fre.REPORT_ENTRY from FIELD_REPORT__REPORT_ENTRY fre where fre.EVENT = sqlc.arg(event_id)
union
select vre.REPORT_ENTRY from VISIT__REPORT_ENTRY vre where vre.EVENT = sqlc.arg(event_id)
;

-- name: DeleteEventIncidentReportEntries :exec
delete from INCIDENT__REPORT_ENTRY where EVENT = ?;

-- name: DeleteEventFieldReportReportEntries :exec
delete from FIELD_REPORT__REPORT_ENTRY where EVENT = ?;

-- name: DeleteEventVisitReportEntries :exec
delete from VISIT__REPORT_ENTRY where EVENT = ?;

-- name: DeleteReportEntries :exec
delete from REPORT_ENTRY where ID in (sqlc.slice(ids));

-- name: DeleteEventIncidentRangers :exec
delete from INCIDENT__RANGER where EVENT = ?;

-- name: DeleteEventIncidentIncidentTypes :exec
delete from INCIDENT__INCIDENT_TYPE where EVENT = ?;

-- name: DeleteEventLinkedIncidents :exec
delete from INCIDENT__LINKED_INCIDENT
where EVENT_1 = sqlc.arg(event_id) or EVENT_2 = sqlc.arg(event_id);

-- name: DeleteEventVisitRangers :exec
delete from VISIT__RANGER where EVENT = ?;

-- name: DeleteEventVisits :exec
delete from VISIT where EVENT = ?;

-- name: DeleteEventFieldReports :exec
delete from FIELD_REPORT where EVENT = ?;

-- name: DeleteEventIncidents :exec
delete from INCIDENT where EVENT = ?;

-- name: DeleteEventAccessAll :exec
delete from EVENT_ACCESS where EVENT = ?;

-- name: DeleteEventPlaces :exec
delete from PLACE where EVENT = ?;

-- name: DetachChildrenFromEventGroup :exec
update `EVENT` set PARENT_GROUP = null where PARENT_GROUP = ?;

-- name: DeleteEvent :exec
delete from `EVENT` where ID = ?;

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
insert into EVENT_ACCESS (EVENT, EXPRESSION, MODE, VALIDITY, NOT_AFTER, NOT_BEFORE, DESCRIPTION)
values (?, ?, ?, ?, ?, ?, ?);

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

-- The VERSION bump must stay in the SET clause: the MySQL driver reports rows
-- *changed* (not matched), so the bump guarantees that zero rows affected means
-- the WHERE clause didn't match (stale version or missing row), never that the
-- row matched but was already identical.
-- name: UpdateIncident :execrows
update INCIDENT set
    VERSION = VERSION + 1,
    -- CREATED should be immutable, so it's not present in this UPDATE query
    PRIORITY = ?,
    STATE = ?,
    STARTED = ?,
    CLOSED = ?,
    SUMMARY = ?,
    LOCATION_NAME = ?,
    LOCATION_ADDRESS = ?,
    LOCATION_DESCRIPTION = ?
where
    EVENT = ?
    and NUMBER = ?
    and VERSION = ?
;

-- BumpIncidentVersion is for mutations that change an incident's representation
-- without going through the version-guarded UpdateIncident query (Ranger
-- assignments, the peer of a link/unlink, field-report/visit reassignment).
-- name: BumpIncidentVersion :exec
update INCIDENT
set VERSION = VERSION + 1
where EVENT = ? and NUMBER = ?;

-- name: IncidentVersion :one
select VERSION
from INCIDENT
where EVENT = ? and NUMBER = ?;

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
    ) as FIELD_REPORT_NUMBERS,
    (
        select coalesce(json_arrayagg(visit.NUMBER), "[]")
        from VISIT visit
        where i.EVENT = visit.EVENT
          and i.NUMBER = visit.INCIDENT_NUMBER
    ) as VISIT_NUMBERS
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
    ) as FIELD_REPORT_NUMBERS,
    (
        select coalesce(json_arrayagg(visit.NUMBER), "[]")
        from VISIT visit
        where i.EVENT = visit.EVENT
          and i.NUMBER = visit.INCIDENT_NUMBER
    ) as VISIT_NUMBERS
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
set INCIDENT_NUMBER = ?, VERSION = VERSION + 1
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

-- name: FieldReportVersion :one
select VERSION
from FIELD_REPORT
where EVENT = ? and NUMBER = ?;

-- The VERSION bump must stay in the SET clause; see UpdateIncident.
-- name: UpdateFieldReport :execrows
update FIELD_REPORT
set VERSION = VERSION + 1, SUMMARY = ?, INCIDENT_NUMBER = ?
where EVENT = ? and NUMBER = ? and VERSION = ?;

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

-- name: AttachReportEntryToVisit :exec
insert into VISIT__REPORT_ENTRY (
    EVENT, VISIT_NUMBER, REPORT_ENTRY
) values (
    ?, ?, ?
);

-- name: AttachVisitToIncident :exec
update VISIT
set INCIDENT_NUMBER = ?, VERSION = VERSION + 1
where EVENT = ? and NUMBER = ?
;

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

-- name: SetVisitReportEntryStricken :exec
update REPORT_ENTRY
set STRICKEN = ?
where ID IN (
    select REPORT_ENTRY
    from VISIT__REPORT_ENTRY
    where EVENT = ?
      and VISIT_NUMBER = ?
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

-- name: CreatePlace :exec
insert into PLACE
    (EVENT, NUMBER, TYPE, NAME, LOCATION_STRING, EXTERNAL_DATA)
values
    (?,?,?,?,?,?)
;

-- name: RemovePlaces :exec
delete from
    PLACE
where EVENT = ?
    and TYPE = ?
;

-- name: Places :many
select
    EVENT,
    TYPE,
    NUMBER,
    NAME,
    LOCATION_STRING,
    if(sqlc.arg(exclude_external_data), '', EXTERNAL_DATA) as EXTERNAL_DATA
from
    PLACE d
where
    EVENT = ?
;

-- name: CreateVisit :execlastid
insert into VISIT (`EVENT`, NUMBER, CREATED) values (?, ?, ?);

-- The VERSION bump must stay in the SET clause; see UpdateIncident.
-- name: UpdateVisit :execrows
update VISIT set
    VERSION = VERSION + 1,
    -- CREATED should be immutable, so it's not present in this UPDATE query
    INCIDENT_NUMBER = ?,
    GUEST_PREFERRED_NAME = ?,
    GUEST_LEGAL_NAME = ?,
    GUEST_DESCRIPTION = ?,
    GUEST_ACTION_PLAN = ?,
    GUEST_CAMP_NAME = ?,
    GUEST_CAMP_ADDRESS = ?,
    GUEST_CAMP_DESCRIPTION = ?,
    GUEST_CAMP_CONTACTS = ?,

    ARRIVAL_TIME = ?,
    ARRIVAL_METHOD = ?,
    ARRIVAL_STATE = ?,
    ARRIVAL_REASON = ?,
    ARRIVAL_BELONGINGS = ?,

    DEPARTURE_TIME = ?,
    DEPARTURE_METHOD = ?,
    DEPARTURE_STATE = ?,

    RESOURCE_SITTER = ?,
    RESOURCE_BED_ID = ?,
    RESOURCE_REST = ?,
    RESOURCE_CLOTHES = ?,
    RESOURCE_POGS = ?,
    RESOURCE_FOOD_BEV = ?,
    RESOURCE_OTHER = ?
where
    EVENT = ?
    and NUMBER = ?
    and VERSION = ?
;

-- BumpVisitVersion is for mutations that change a visit's representation
-- without going through the version-guarded UpdateVisit query (Ranger
-- assignments).
-- name: BumpVisitVersion :exec
update VISIT
set VERSION = VERSION + 1
where EVENT = ? and NUMBER = ?;

-- name: VisitVersion :one
select VERSION
from VISIT
where EVENT = ? and NUMBER = ?;

-- name: Visit :one
select
    sqlc.embed(s)
from
    VISIT s
where
    s.EVENT = ?
    and s.NUMBER = ?;

-- name: Visits :many
select
    sqlc.embed(s)
from
    VISIT s
where
    s.EVENT = ?
group by
    s.NUMBER;

-- name: Visits_Rangers :many
select
    sqlc.embed(sr)
from
    VISIT__RANGER sr
where
    sr.EVENT = ?;

-- name: Visit_Rangers :many
select
    sqlc.embed(sr)
from
    VISIT__RANGER sr
where
    sr.EVENT = ?
    and sr.VISIT_NUMBER = ?;

-- name: AttachRangerToVisit :exec
insert into VISIT__RANGER (EVENT, VISIT_NUMBER, RANGER_HANDLE, ROLE)
values (?, ?, ?, ?);

-- name: DetachRangerFromVisit :exec
delete from VISIT__RANGER
where
    EVENT = ?
    and VISIT_NUMBER = ?
    and RANGER_HANDLE = ?
;

-- name: Visit_ReportEntries :many
select
    sre.VISIT_NUMBER,
    sqlc.embed(re)
from
    VISIT__REPORT_ENTRY sre
        join REPORT_ENTRY re
             on re.ID = sre.REPORT_ENTRY
where
    sre.EVENT = ?
    and sre.VISIT_NUMBER = ?
;

-- name: Visits_ReportEntries :many
select
    sre.VISIT_NUMBER,
    sqlc.embed(re)
from
    VISIT__REPORT_ENTRY sre
        join REPORT_ENTRY re
             on re.ID = sre.REPORT_ENTRY
where
    sre.EVENT = ?
    and re.GENERATED <= ?
;

-- This doesn't use "MAX" because sqlc can't figure out the type for aggregations :(.
-- name: NextVisitNumber :one
select NUMBER + 1 as NEXT_ID
from VISIT
where EVENT = ?
union
select 1
order by 1 desc
limit 1;

-- name: DirectoryActivePersons :many
select ID, HANDLE, EMAIL, PASSWORD, ONSITE
from DIRECTORY_PERSON
where ACTIVE;

-- name: DirectoryActivePositions :many
select ID, TITLE from DIRECTORY_POSITION where ACTIVE;

-- name: DirectoryActiveTeams :many
select ID, TITLE from DIRECTORY_TEAM where ACTIVE;

-- name: DirectoryPersonPositions :many
select PERSON_ID, POSITION_ID from DIRECTORY_PERSON__POSITION;

-- name: DirectoryPersonTeams :many
select PERSON_ID, TEAM_ID from DIRECTORY_PERSON__TEAM;

-- name: DirectoryAllPersons :many
select ID, HANDLE, EMAIL, ACTIVE, ONSITE
from DIRECTORY_PERSON;

-- name: DirectoryAllPositions :many
select ID, TITLE, ACTIVE from DIRECTORY_POSITION;

-- name: DirectoryAllTeams :many
select ID, TITLE, ACTIVE from DIRECTORY_TEAM;

-- name: DirectoryPersonByHandle :one
select ID, HANDLE, EMAIL, ACTIVE, ONSITE
from DIRECTORY_PERSON
where HANDLE = ?;

-- name: DirectoryCreatePerson :execlastid
insert into DIRECTORY_PERSON (HANDLE, EMAIL, PASSWORD, ACTIVE, ONSITE)
values (?, ?, ?, ?, ?);

-- name: DirectoryUpdatePerson :exec
update DIRECTORY_PERSON
set
    HANDLE = ?,
    EMAIL = ?,
    ACTIVE = ?,
    ONSITE = ?
where ID = ?;

-- name: DirectorySetPersonPassword :exec
update DIRECTORY_PERSON
set PASSWORD = ?
where ID = ?;

-- name: DirectoryDeletePerson :exec
delete from DIRECTORY_PERSON where ID = ?;

-- name: DirectoryCreateTeam :execlastid
insert into DIRECTORY_TEAM (TITLE, ACTIVE) values (?, ?);

-- name: DirectoryUpdateTeam :exec
update DIRECTORY_TEAM
set TITLE = ?, ACTIVE = ?
where ID = ?;

-- name: DirectoryDeleteTeam :exec
delete from DIRECTORY_TEAM where ID = ?;

-- name: DirectoryCreatePosition :execlastid
insert into DIRECTORY_POSITION (TITLE, ACTIVE) values (?, ?);

-- name: DirectoryUpdatePosition :exec
update DIRECTORY_POSITION
set TITLE = ?, ACTIVE = ?
where ID = ?;

-- name: DirectoryDeletePosition :exec
delete from DIRECTORY_POSITION where ID = ?;

-- name: DirectoryClearPersonTeams :exec
delete from DIRECTORY_PERSON__TEAM where PERSON_ID = ?;

-- name: DirectoryAddPersonTeam :exec
insert into DIRECTORY_PERSON__TEAM (PERSON_ID, TEAM_ID) values (?, ?);

-- name: DirectoryClearPersonPositions :exec
delete from DIRECTORY_PERSON__POSITION where PERSON_ID = ?;

-- name: DirectoryAddPersonPosition :exec
insert into DIRECTORY_PERSON__POSITION (PERSON_ID, POSITION_ID) values (?, ?);

-- name: DirectoryPersonByID :one
select ID, HANDLE, EMAIL, ACTIVE, ONSITE
from DIRECTORY_PERSON
where ID = ?;

-- The Search* queries below power the cross-event search API. Each matches
-- either a case-insensitive LIKE pattern (the handler escapes user input and
-- wraps it in "%") or a REGEXP pattern, scoped to the events the requestor may
-- read. Exactly one of text_like and text_regexp must be non-null: comparing
-- against a null pattern yields null, so the unused branch of each
-- "like ... or ... regexp ..." pair drops out. The MATCHED_ENTRY_TEXT column
-- carries the text of one matching report entry, for display as a
-- search-result snippet.

-- name: SearchIncidents :many
select
    i.EVENT,
    e.NAME as EVENT_NAME,
    i.NUMBER,
    i.CREATED,
    i.PRIORITY,
    i.SUMMARY,
    i.LOCATION_NAME,
    coalesce((
        select re.TEXT
        from INCIDENT__REPORT_ENTRY ire
            join REPORT_ENTRY re
                on re.ID = ire.REPORT_ENTRY
        where ire.EVENT = i.EVENT
            and ire.INCIDENT_NUMBER = i.NUMBER
            and re.GENERATED = false
            and re.STRICKEN = false
            and (re.TEXT like sqlc.narg(text_like) or regexp_instr(re.TEXT, sqlc.narg(text_regexp)) > 0)
        order by re.CREATED
        limit 1
    ), '') as MATCHED_ENTRY_TEXT
from INCIDENT i
    join EVENT e
        on e.ID = i.EVENT
where i.EVENT in (sqlc.slice(event_ids))
    and (
        (i.SUMMARY like sqlc.narg(text_like) or regexp_instr(i.SUMMARY, sqlc.narg(text_regexp)) > 0)
        or (i.LOCATION_NAME like sqlc.narg(text_like) or regexp_instr(i.LOCATION_NAME, sqlc.narg(text_regexp)) > 0)
        or (i.LOCATION_ADDRESS like sqlc.narg(text_like) or regexp_instr(i.LOCATION_ADDRESS, sqlc.narg(text_regexp)) > 0)
        or (i.LOCATION_DESCRIPTION like sqlc.narg(text_like) or regexp_instr(i.LOCATION_DESCRIPTION, sqlc.narg(text_regexp)) > 0)
        or exists (
            select 1
            from INCIDENT__RANGER ir
            where ir.EVENT = i.EVENT
                and ir.INCIDENT_NUMBER = i.NUMBER
                and (ir.RANGER_HANDLE like sqlc.narg(text_like) or regexp_instr(ir.RANGER_HANDLE, sqlc.narg(text_regexp)) > 0)
        )
        or exists (
            select 1
            from INCIDENT__INCIDENT_TYPE iit
                join INCIDENT_TYPE it
                    on it.ID = iit.INCIDENT_TYPE
            where iit.EVENT = i.EVENT
                and iit.INCIDENT_NUMBER = i.NUMBER
                and (it.NAME like sqlc.narg(text_like) or regexp_instr(it.NAME, sqlc.narg(text_regexp)) > 0)
        )
        or exists (
            select 1
            from INCIDENT__REPORT_ENTRY ire
                join REPORT_ENTRY re
                    on re.ID = ire.REPORT_ENTRY
            where ire.EVENT = i.EVENT
                and ire.INCIDENT_NUMBER = i.NUMBER
                and re.GENERATED = false
                and re.STRICKEN = false
                and (re.TEXT like sqlc.narg(text_like) or regexp_instr(re.TEXT, sqlc.narg(text_regexp)) > 0)
        )
    )
order by i.CREATED desc
limit ?
;

-- name: SearchFieldReports :many
select
    fr.EVENT,
    e.NAME as EVENT_NAME,
    fr.NUMBER,
    fr.CREATED,
    fr.SUMMARY,
    fr.INCIDENT_NUMBER,
    coalesce((
        select re.TEXT
        from FIELD_REPORT__REPORT_ENTRY frre
            join REPORT_ENTRY re
                on re.ID = frre.REPORT_ENTRY
        where frre.EVENT = fr.EVENT
            and frre.FIELD_REPORT_NUMBER = fr.NUMBER
            and re.GENERATED = false
            and re.STRICKEN = false
            and (re.TEXT like sqlc.narg(text_like) or regexp_instr(re.TEXT, sqlc.narg(text_regexp)) > 0)
        order by re.CREATED
        limit 1
    ), '') as MATCHED_ENTRY_TEXT
from FIELD_REPORT fr
    join EVENT e
        on e.ID = fr.EVENT
where fr.EVENT in (sqlc.slice(event_ids))
    and (
        (fr.SUMMARY like sqlc.narg(text_like) or regexp_instr(fr.SUMMARY, sqlc.narg(text_regexp)) > 0)
        or exists (
            select 1
            from FIELD_REPORT__REPORT_ENTRY frre
                join REPORT_ENTRY re
                    on re.ID = frre.REPORT_ENTRY
            where frre.EVENT = fr.EVENT
                and frre.FIELD_REPORT_NUMBER = fr.NUMBER
                and re.GENERATED = false
                and re.STRICKEN = false
                and (re.TEXT like sqlc.narg(text_like) or regexp_instr(re.TEXT, sqlc.narg(text_regexp)) > 0)
        )
    )
order by fr.CREATED desc
limit ?
;

-- name: SearchVisits :many
select
    v.EVENT,
    e.NAME as EVENT_NAME,
    v.NUMBER,
    v.CREATED,
    v.INCIDENT_NUMBER,
    v.GUEST_PREFERRED_NAME,
    v.GUEST_LEGAL_NAME,
    v.GUEST_CAMP_NAME,
    coalesce((
        select re.TEXT
        from VISIT__REPORT_ENTRY vre
            join REPORT_ENTRY re
                on re.ID = vre.REPORT_ENTRY
        where vre.EVENT = v.EVENT
            and vre.VISIT_NUMBER = v.NUMBER
            and re.GENERATED = false
            and re.STRICKEN = false
            and (re.TEXT like sqlc.narg(text_like) or regexp_instr(re.TEXT, sqlc.narg(text_regexp)) > 0)
        order by re.CREATED
        limit 1
    ), '') as MATCHED_ENTRY_TEXT
from VISIT v
    join EVENT e
        on e.ID = v.EVENT
where v.EVENT in (sqlc.slice(event_ids))
    and (
        (v.GUEST_PREFERRED_NAME like sqlc.narg(text_like) or regexp_instr(v.GUEST_PREFERRED_NAME, sqlc.narg(text_regexp)) > 0)
        or (v.GUEST_LEGAL_NAME like sqlc.narg(text_like) or regexp_instr(v.GUEST_LEGAL_NAME, sqlc.narg(text_regexp)) > 0)
        or (v.GUEST_DESCRIPTION like sqlc.narg(text_like) or regexp_instr(v.GUEST_DESCRIPTION, sqlc.narg(text_regexp)) > 0)
        or (v.GUEST_CAMP_NAME like sqlc.narg(text_like) or regexp_instr(v.GUEST_CAMP_NAME, sqlc.narg(text_regexp)) > 0)
        or (v.GUEST_CAMP_ADDRESS like sqlc.narg(text_like) or regexp_instr(v.GUEST_CAMP_ADDRESS, sqlc.narg(text_regexp)) > 0)
        or exists (
            select 1
            from VISIT__RANGER vr
            where vr.EVENT = v.EVENT
                and vr.VISIT_NUMBER = v.NUMBER
                and (vr.RANGER_HANDLE like sqlc.narg(text_like) or regexp_instr(vr.RANGER_HANDLE, sqlc.narg(text_regexp)) > 0)
        )
        or exists (
            select 1
            from VISIT__REPORT_ENTRY vre
                join REPORT_ENTRY re
                    on re.ID = vre.REPORT_ENTRY
            where vre.EVENT = v.EVENT
                and vre.VISIT_NUMBER = v.NUMBER
                and re.GENERATED = false
                and re.STRICKEN = false
                and (re.TEXT like sqlc.narg(text_like) or regexp_instr(re.TEXT, sqlc.narg(text_regexp)) > 0)
        )
    )
order by v.CREATED desc
limit ?
;
