create table STAY (
    `EVENT`         integer  not null,
    NUMBER          integer  not null,
    CREATED         double   not null,
    INCIDENT_NUMBER integer,

    GUEST_PREFERRED_NAME        varchar(128),
    GUEST_LEGAL_NAME            varchar(128),
    GUEST_DESCRIPTION           varchar(256),
    GUEST_CAMP_NAME             varchar(256),
    GUEST_CAMP_ADDRESS          varchar(256),
    GUEST_CAMP_DESCRIPTION      varchar(256),

    ARRIVAL_TIME        double,
    ARRIVAL_METHOD      varchar(256),
    ARRIVAL_STATE       varchar(256),
    ARRIVAL_REASON      varchar(256),
    ARRIVAL_BELONGINGS  varchar(256),

    DEPARTURE_TIME      double,
    DEPARTURE_METHOD    varchar(256),
    DEPARTURE_STATE     varchar(256),

    RESOURCE_REST       varchar(256),
    RESOURCE_CLOTHES    varchar(256),
    RESOURCE_POGS       varchar(256),
    RESOURCE_FOOD_BEV   varchar(256),
    RESOURCE_OTHER      varchar(256),

    foreign key `STAY_TO_EVENT` (`EVENT`) references `EVENT`(ID),
    foreign key `STAY_TO_INCIDENT` (`EVENT`, INCIDENT_NUMBER) references INCIDENT(`EVENT`, NUMBER),

    primary key (`EVENT`, NUMBER)
) DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

create table STAY__REPORT_ENTRY (
    `EVENT`             integer not null,
    STAY_NUMBER         integer not null,
    REPORT_ENTRY        integer not null,

    foreign key `SRE_TO_EVENT` (`EVENT`) references `EVENT`(ID),
    foreign key `SRE_TO_GUEST_VISIT` (`EVENT`, STAY_NUMBER)
        references STAY(`EVENT`, NUMBER),
    foreign key `SRE_TO_REPORT_ENTRY` (REPORT_ENTRY)
        references REPORT_ENTRY(ID),

    primary key (`EVENT`, STAY_NUMBER, REPORT_ENTRY)
) DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

create table STAY__RANGER (
    ID                  integer     not null auto_increment,
    `EVENT`             integer     not null,
    STAY_NUMBER         integer     not null,
    RANGER_HANDLE       varchar(64) not null,
    ROLE                varchar(128),

    foreign key (`EVENT`) references `EVENT` (ID),
    foreign key (`EVENT`, STAY_NUMBER) references STAY (`EVENT`, NUMBER),

    primary key (ID)
) DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;


alter table EVENT_ACCESS
    modify column MODE enum ('read', 'write', 'report', 'write_stays') not null;


update `SCHEMA_INFO`
set `VERSION` = 27
where true;
