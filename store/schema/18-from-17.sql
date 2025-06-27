create table `ACTION_LOG` (
    `ID`                bigint not null auto_increment,
    `CREATED_AT`        timestamp not null default current_timestamp,

    -- request metadata
    `ACTION_TYPE`       varchar(128) not null,
    `METHOD`            varchar(128),
    `PATH`              varchar(128),
    `REFERRER`          varchar(128),

    -- requestor metadata
    `USER_ID`           bigint,
    `USER_NAME`         varchar(128),
    `POSITION_ID`       bigint,
    `POSITION_NAME`     varchar(128),
    `CLIENT_ADDRESS`    varchar(128),

    -- response metadata
    `HTTP_STATUS`       smallint,
    `DURATION_MICROS`   bigint,

    primary key (`ID`)
) DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

update `SCHEMA_INFO`
set `VERSION` = 18
where true;
