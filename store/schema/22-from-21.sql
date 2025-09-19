create table INCIDENT__LINKED_INCIDENT (
    EVENT_1             integer not null,
    INCIDENT_NUMBER_1   integer not null,
    EVENT_2             integer not null,
    INCIDENT_NUMBER_2   integer not null,

    foreign key (EVENT_1, INCIDENT_NUMBER_1) references INCIDENT(`EVENT`, NUMBER),
    foreign key (EVENT_2, INCIDENT_NUMBER_2) references INCIDENT(`EVENT`, NUMBER),

    primary key (EVENT_1, INCIDENT_NUMBER_1, EVENT_2, INCIDENT_NUMBER_2)
) DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

update `SCHEMA_INFO`
set `VERSION` = 22
where true;
