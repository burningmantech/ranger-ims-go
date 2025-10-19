create table `DESTINATION` (
    `EVENT`             integer not null,
    `TYPE`              enum('camp', 'art', 'other') not null,
    `NUMBER`            integer not null,
    `NAME`              varchar(1024) not null,
    `LOCATION_STRING`   varchar(1024) not null,
    `EXTERNAL_DATA`     json,

    primary key (`EVENT`, `TYPE`, `NUMBER`),
    foreign key `DEST_EVENT` (`EVENT`) references `EVENT`(ID)
) default charset=utf8mb4 collate=utf8mb4_unicode_ci;

update `SCHEMA_INFO`
set `VERSION` = 23
where true;
