-- Set defaults on these several NOT NULL columns whose values are irrelevant for IMS.
-- This makes the INSERT statement less cluttered.
alter table `person`
    modify column `first_name` varchar(25) NOT NULL DEFAULT '',
    modify column `last_name` varchar(25) NOT NULL DEFAULT '',
    modify column `callsign_normalized` varchar(128) NOT NULL DEFAULT '',
    modify column `callsign_soundex` varchar(128) NOT NULL DEFAULT '',
    modify column `pronouns_custom` varchar(191) NOT NULL DEFAULT ''
;

insert into `person`
(`id`,      `callsign`,     `email`,                        `password`,                     `status`,   `on_site`)
values
-- password is )'(
(6000,   "AdminTestRanger", "admintestranger@example.com",  "$argon2id$v=19$m=1,t=1,p=1$51uXrZoFRb6O4Tw4TsAJVQ$SedDwp+hPpIJc42QcnFJy6EOtE+b5kyYFpnuRHl/5qs",       "active",   true),
-- password is password
(6001,   "AliceTestRanger", "alicetestranger@example.com",  "$argon2id$v=19$m=1,t=1,p=1$eg9U8hLotCSmyCph1BQroA$KFfy0uDDpP+cXPnkSQRXt3z0Shd7M39tsrwJZuDrOdU",  "active",   true)
;

insert into `position`
(`id`, `title`, `all_rangers`)
values
(7000, "Nooperator", false),
-- An all_rangers position is left out of the directory's positions,
-- but the on-duty list still names it.
(7001, "Dirt", true)
;

insert into `person_position`
(`person_id`, `position_id`)
values
(6001, 7000)
;

insert into `team`
(`id`, `title`)
values
(8000, "Brown Dot")
;

insert into `person_team`
(`person_id`, `team_id`)
values
(6000, 8000)
;

insert into `timesheet`
(`person_id`, `position_id`, `on_duty`, `off_duty`)
values
(6001, 7000, now(), null),
-- Two open shifts, from a missed sign-out. The on-duty list shows the newer.
(6000, 7000, date_sub(now(), interval 2 hour), null),
(6000, 7001, date_sub(now(), interval 1 hour), null),
-- A finished shift, for looking back at who was on duty earlier.
(6001, 7000, date_sub(now(), interval 5 hour), date_sub(now(), interval 3 hour))
;
