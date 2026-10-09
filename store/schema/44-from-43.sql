/* Move the active event out of EVENT and into a one-row table.

   Setting EVENT.IS_ACTIVE wrote every EVENT row, so it could deadlock with
   any concurrent event write. Now it writes just the one ACTIVE_EVENT row. */

create table `ACTIVE_EVENT` (
    `ID`    tinyint not null default 1,
    `EVENT` integer,

    primary key (`ID`),
    constraint `ACTIVE_EVENT_SINGLE_ROW` check (`ID` = 1),
    foreign key `ACTIVE_EVENT_TO_EVENT` (`EVENT`) references `EVENT`(`ID`) on delete set null
) DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

insert into `ACTIVE_EVENT` (`ID`, `EVENT`)
values (1, (select `ID` from `EVENT` where `IS_ACTIVE` limit 1));

alter table `EVENT`
    drop column `IS_ACTIVE`;

update `SCHEMA_INFO`
set `VERSION` = 44
where true;
