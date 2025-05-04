create table CONCENTRIC_STREET
(
    EVENT int          not null,
    ID    varchar(16)  not null,
    NAME  varchar(128) not null,
    primary key (EVENT, ID)
)
    charset = latin1;

create table EVENT
(
    ID   int auto_increment
        primary key,
    NAME varchar(128) not null,
    constraint NAME
        unique (NAME)
)
    charset = latin1;

create table EVENT_ACCESS
(
    EVENT      int                              not null,
    EXPRESSION varchar(128)                     not null,
    MODE       enum ('read', 'write', 'report') not null,
    primary key (EVENT, EXPRESSION),
    constraint EVENT_ACCESS_ibfk_1
        foreign key (EVENT) references EVENT (ID)
)
    charset = latin1;

create table INCIDENT
(
    EVENT                  int                                                         not null,
    NUMBER                 int                                                         not null,
    CREATED                double                                                      not null,
    PRIORITY               tinyint                                                     not null,
    STATE                  enum ('new', 'on_hold', 'dispatched', 'on_scene', 'closed') not null,
    SUMMARY                varchar(1024)                                               null,
    LOCATION_NAME          varchar(1024)                                               null,
    LOCATION_CONCENTRIC    varchar(64)                                                 null,
    LOCATION_RADIAL_HOUR   tinyint                                                     null,
    LOCATION_RADIAL_MINUTE tinyint                                                     null,
    LOCATION_DESCRIPTION   varchar(1024)                                               null,
    primary key (EVENT, NUMBER),
    constraint INCIDENT_ibfk_1
        foreign key (EVENT) references EVENT (ID),
    constraint INCIDENT_ibfk_2
        foreign key (EVENT, LOCATION_CONCENTRIC) references CONCENTRIC_STREET (EVENT, ID)
)
    charset = latin1;

create index EVENT
    on INCIDENT (EVENT, LOCATION_CONCENTRIC);

create table INCIDENT_REPORT
(
    EVENT           int           not null,
    NUMBER          int           not null,
    CREATED         double        not null,
    SUMMARY         varchar(1024) null,
    INCIDENT_NUMBER int           null,
    primary key (EVENT, NUMBER),
    constraint INCIDENT_REPORT_ibfk_1
        foreign key (EVENT) references EVENT (ID),
    constraint INCIDENT_REPORT_ibfk_2
        foreign key (EVENT, INCIDENT_NUMBER) references INCIDENT (EVENT, NUMBER)
)
    charset = latin1;

create index EVENT
    on INCIDENT_REPORT (EVENT, INCIDENT_NUMBER);

create table INCIDENT_TYPE
(
    ID     int auto_increment
        primary key,
    NAME   varchar(128) not null,
    HIDDEN tinyint(1)   not null,
    constraint NAME
        unique (NAME)
)
    charset = latin1;

create table INCIDENT__INCIDENT_TYPE
(
    EVENT           int not null,
    INCIDENT_NUMBER int not null,
    INCIDENT_TYPE   int not null,
    primary key (EVENT, INCIDENT_NUMBER, INCIDENT_TYPE),
    constraint INCIDENT__INCIDENT_TYPE_ibfk_1
        foreign key (EVENT) references EVENT (ID),
    constraint INCIDENT__INCIDENT_TYPE_ibfk_2
        foreign key (EVENT, INCIDENT_NUMBER) references INCIDENT (EVENT, NUMBER),
    constraint INCIDENT__INCIDENT_TYPE_ibfk_3
        foreign key (INCIDENT_TYPE) references INCIDENT_TYPE (ID)
)
    charset = latin1;

create index INCIDENT_TYPE
    on INCIDENT__INCIDENT_TYPE (INCIDENT_TYPE);

create table INCIDENT__RANGER
(
    EVENT           int         not null,
    INCIDENT_NUMBER int         not null,
    RANGER_HANDLE   varchar(64) not null,
    primary key (EVENT, INCIDENT_NUMBER, RANGER_HANDLE),
    constraint INCIDENT__RANGER_ibfk_1
        foreign key (EVENT) references EVENT (ID),
    constraint INCIDENT__RANGER_ibfk_2
        foreign key (EVENT, INCIDENT_NUMBER) references INCIDENT (EVENT, NUMBER)
)
    charset = latin1;

create table REPORT_ENTRY
(
    ID          int auto_increment
        primary key,
    AUTHOR      varchar(64) not null,
    TEXT        text        not null,
    CREATED     double      not null,
    `GENERATED` tinyint(1)  not null,
    STRICKEN    tinyint(1)  not null
)
    charset = latin1;

create table INCIDENT_REPORT__REPORT_ENTRY
(
    EVENT                  int not null,
    INCIDENT_REPORT_NUMBER int not null,
    REPORT_ENTRY           int not null,
    primary key (EVENT, INCIDENT_REPORT_NUMBER, REPORT_ENTRY),
    constraint INCIDENT_REPORT__REPORT_ENTRY_ibfk_1
        foreign key (EVENT) references EVENT (ID),
    constraint INCIDENT_REPORT__REPORT_ENTRY_ibfk_2
        foreign key (EVENT, INCIDENT_REPORT_NUMBER) references INCIDENT_REPORT (EVENT, NUMBER),
    constraint INCIDENT_REPORT__REPORT_ENTRY_ibfk_3
        foreign key (REPORT_ENTRY) references REPORT_ENTRY (ID)
)
    charset = latin1;

create index REPORT_ENTRY
    on INCIDENT_REPORT__REPORT_ENTRY (REPORT_ENTRY);

create table INCIDENT__REPORT_ENTRY
(
    EVENT           int not null,
    INCIDENT_NUMBER int not null,
    REPORT_ENTRY    int not null,
    primary key (EVENT, INCIDENT_NUMBER, REPORT_ENTRY),
    constraint INCIDENT__REPORT_ENTRY_ibfk_1
        foreign key (EVENT) references EVENT (ID),
    constraint INCIDENT__REPORT_ENTRY_ibfk_2
        foreign key (EVENT, INCIDENT_NUMBER) references INCIDENT (EVENT, NUMBER),
    constraint INCIDENT__REPORT_ENTRY_ibfk_3
        foreign key (REPORT_ENTRY) references REPORT_ENTRY (ID)
)
    charset = latin1;

create index REPORT_ENTRY
    on INCIDENT__REPORT_ENTRY (REPORT_ENTRY);

create table SCHEMA_INFO
(
    VERSION smallint not null
)
    charset = latin1;

insert into INCIDENT_TYPE (NAME, HIDDEN) values ('Admin', 0);
insert into INCIDENT_TYPE (NAME, HIDDEN) values ('Junk' , 0);

insert into SCHEMA_INFO (VERSION) values (6);
