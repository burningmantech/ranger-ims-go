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
-- An all_rangers position, which access rules can target only through onduty:,
-- and which the on-duty list names.
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

-- The Clubhouse writes America/Phoenix wall-clock times (UTC-7, with no DST),
-- whatever the database server's own time zone is.
set @phoenix_now = utc_timestamp() - interval 7 hour;

insert into `timesheet`
(`person_id`, `position_id`, `on_duty`, `off_duty`)
values
(6001, 7000, @phoenix_now, null),
-- Two open shifts, from a missed sign-out. IMS uses the newer.
(6000, 7000, @phoenix_now - interval 2 hour, null),
(6000, 7001, @phoenix_now - interval 1 hour, null),
-- A finished shift, for looking back at who was on duty earlier.
(6001, 7000, @phoenix_now - interval 5 hour, @phoenix_now - interval 3 hour)
;

-- A slot spanning the last several hours and the next few, which the pods below
-- are out on.
insert into `slot`
(`id`, `begins`, `ends`, `position_id`, `timezone_abbr`)
values
(9100, @phoenix_now - interval 6 hour, @phoenix_now + interval 6 hour, 7000, "MST")
;

insert into `pod`
(`id`, `type`, `transport`, `location`, `formed_at`, `disbanded_at`, `slot_id`)
values
(9000, "shift", "bicycle", "3:00 & C", @phoenix_now - interval 6 hour, null, 9100),
-- Disbanded 100 minutes ago.
(9001, "mentor", "foot", null, @phoenix_now - interval 6 hour, @phoenix_now - interval 100 minute, 9100),
-- The admin left this one 10 minutes ago.
(9002, "mitten", "vehicle", null, @phoenix_now - interval 6 hour, null, 9100)
;

insert into `person_pod`
(`pod_id`, `person_id`, `joined_at`, `left_at`)
values
(9000, 6000, @phoenix_now - interval 2 hour, null),
(9000, 6001, @phoenix_now - interval 30 minute, null),
(9001, 6000, @phoenix_now - interval 2 hour, null),
(9002, 6000, @phoenix_now - interval 30 minute, @phoenix_now - interval 10 minute)
;
