alter table `INCIDENT`
add column `CLOSED` double after `STARTED`;

update
    `INCIDENT`
    join
        (select
             ire.INCIDENT_NUMBER AS `INCIDENT_NUMBER`
              ,ire.EVENT AS `EVENT_ID`
              ,max(re.created) as `CLOSE_TIME`
         from INCIDENT__REPORT_ENTRY ire
                  join REPORT_ENTRY re
                       on ire.report_entry = re.id
         where (
            re.text like '%Changed state: closed%'
            or re.text like '%Changed state to: closed%'
            or re.text like '%State changed to: closed%'
         )
         group by 1,2
        ) as `ENTRY`
        on
            INCIDENT.NUMBER = ENTRY.INCIDENT_NUMBER
                and INCIDENT.EVENT = ENTRY.EVENT_ID
set
    `CLOSED` = ENTRY.CLOSE_TIME
where
    INCIDENT.STATE = 'closed'
;

update `SCHEMA_INFO`
set `VERSION` = 21
where true;
