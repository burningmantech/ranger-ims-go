/* Let admins name particular days of an event (e.g. "Man Burn"), for the web
   UI to show alongside timestamps. */

create table `EVENT_DAY_NAME` (
    `EVENT` integer      not null,
    `DATE`  date         not null,
    `NAME`  varchar(128) not null,

    primary key (`EVENT`, `DATE`),
    foreign key `EVENT_DAY_NAME_EVENT` (`EVENT`) references `EVENT`(`ID`)
) default charset=utf8mb4 collate=utf8mb4_unicode_ci;

update `SCHEMA_INFO`
set `VERSION` = 49
where true;
