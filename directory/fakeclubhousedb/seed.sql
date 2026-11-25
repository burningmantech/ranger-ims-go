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
(`id`,  `callsign`, `email`,                `password`,                     `status`,   `on_site`)
values
-- All of these users just have passwords equal to their case-sensitive callsigns
(600,   "Hardware", "hardware@example.com", "$argon2id$v=19$m=8192,t=4,p=1$pGcXPQ1tP2Lz54c+R+8jgg$FZryn3Gttrxi7GoZXxyG/rGl/xNjOEjjDGeboEGMph8", "active",   true),
(601,   "Loosy",    "loosy@example.com",    "$argon2id$v=19$m=8192,t=4,p=1$qGMBHtiPMl0MtCQncHhNdA$f1LbS0MIcIYjfiisvxAyEJC21lGVMBB11Yr9ctLP7aI", "active",   true),
(602,   "Doggy",    "doggy@example.com",    "$argon2id$v=19$m=8192,t=4,p=1$L1EBUN9twap4arU4qurBdA$ML6fWDryXTmbyiAfzIQMFUoHYbJoOhwh58Oq3ffxeWA", "active",   true),
(603,   "Runner",   "runner@example.com",   "$argon2id$v=19$m=8192,t=4,p=1$T9NHbTTp5i+AS1ik8gTJrw$aTO48H4+X7oyKKk3gpX7UvkxNifULL1vjL2Ymq/9X/Y", "active",   true),
(604,   "TheMan",   "theman@example.com",   "$argon2id$v=19$m=8192,t=4,p=1$5zituz25m5nT7J2MgGAwfQ$EtaP17cnQCVOaVx3Ns8TZPPN1s3zY7bLvKW4QlwSR9Y", "active",   true),
(605,   "someGuy",  "someguy@example.com",  "$argon2id$v=19$m=8192,t=4,p=1$agCgRJ4xhInkn3sugdUoQg$dVQCRWYXzGDczSijK/6bAb1XuzbBHB+iMkz/VLmRqho", "active",   true)
;

insert into `position`
(`id`, `title`, `all_rangers`)
values
(701, "Driver", false),
(702, "Dancer", false),
(703, "Dirt", true)
;

insert into `person_position`
(`person_id`, `position_id`)
values
(600, 701),
(600, 702),
(603, 703),
(605, 703)
;

insert into `team`
(`id`, `title`)
values
(800, "Driving Team")
;

insert into `person_team`
(`person_id`, `team_id`)
values
(600, 800),
(605, 800)
;

-- The Clubhouse writes America/Phoenix wall-clock times (UTC-7, with no DST).
insert into `timesheet`
(`person_id`, `position_id`, `on_duty`, `off_duty`)
values
(600, 702, utc_timestamp() - interval 7 hour, null),
(601, 701, utc_timestamp() - interval 7 hour, null),
(603, 703, utc_timestamp() - interval 7 hour, null),
(604, 703, utc_timestamp() - interval 7 hour, null),
(605, 703, utc_timestamp() - interval 7 hour, null)
;

insert into `slot`
(`id`, `begins`, `ends`, `position_id`, `timezone_abbr`)
values
(900, utc_timestamp() - interval 9 hour, utc_timestamp() - interval 3 hour, 703, "MST")
;

insert into `pod`
(`id`, `type`, `transport`, `location`, `formed_at`, `slot_id`)
values
(900, "shift", "bicycle", "3:00 & C", utc_timestamp() - interval 9 hour, 900),
(901, "shift", "foot", "Center Camp", utc_timestamp() - interval 9 hour, 900)
;

insert into `person_pod`
(`pod_id`, `person_id`, `joined_at`)
values
(900, 603, utc_timestamp() - interval 7 hour),
(900, 604, utc_timestamp() - interval 7 hour),
(901, 600, utc_timestamp() - interval 7 hour),
(901, 605, utc_timestamp() - interval 7 hour)
;
