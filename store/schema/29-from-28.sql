rename table STAY to VISIT;

alter table VISIT
    drop foreign key STAY_TO_EVENT,
    drop foreign key STAY_TO_INCIDENT,
    add constraint VISIT_TO_EVENT foreign key (`EVENT`) references `EVENT`(ID),
    add constraint VISIT_TO_INCIDENT foreign key (`EVENT`, INCIDENT_NUMBER) references INCIDENT(`EVENT`, NUMBER);

rename table STAY__REPORT_ENTRY to VISIT__REPORT_ENTRY;

alter table VISIT__REPORT_ENTRY
    rename column STAY_NUMBER to VISIT_NUMBER,
    drop foreign key SRE_TO_EVENT,
    drop foreign key SRE_TO_GUEST_VISIT,
    drop foreign key SRE_TO_REPORT_ENTRY,
    add constraint VRE_TO_EVENT foreign key (`EVENT`) references `EVENT`(ID),
    add constraint VRE_TO_GUEST_VISIT foreign key (`EVENT`, VISIT_NUMBER) references VISIT(`EVENT`, NUMBER),
    add constraint VRE_TO_REPORT_ENTRY foreign key (REPORT_ENTRY) references REPORT_ENTRY(ID);

rename table STAY__RANGER to VISIT__RANGER;

alter table VISIT__RANGER
    rename column STAY_NUMBER to VISIT_NUMBER;

-- Safely rename the write_stays enum value to write_visits.
alter table EVENT_ACCESS
    modify column MODE enum ('read', 'write', 'report', 'write_stays', 'write_visits') not null;

update EVENT_ACCESS
set MODE = 'write_visits'
where MODE = 'write_stays';

alter table EVENT_ACCESS
    modify column MODE enum ('read', 'write', 'report', 'write_visits') not null;

update `SCHEMA_INFO`
set `VERSION` = 29
where true;
