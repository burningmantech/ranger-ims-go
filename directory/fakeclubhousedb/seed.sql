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
(603,   "Runner",   "runner@example.com",   "$argon2id$v=19$m=8192,t=4,p=1$YY+aGB2MTKrnHFEQqdfdNg$Q9yByHhJ1GpOW+fUWePv+u76wE9fCGXSONkawTiyJEs", "active",   true),
(604,   "TheMan",   "theman@example.com",   "$argon2id$v=19$m=8192,t=4,p=1$5zituz25m5nT7J2MgGAwfQ$EtaP17cnQCVOaVx3Ns8TZPPN1s3zY7bLvKW4QlwSR9Y", "active",   true)
;

insert into `position`
(`id`, `title`)
values
(701, "Driver"),
(702, "Dancer")
;

insert into `person_position`
(`person_id`, `position_id`)
values
(600, 701),
(600, 702)
;

insert into `team`
(`id`, `title`)
values
(800, "Driving Team")
;

insert into `person_team`
(`person_id`, `team_id`)
values
(600, 800)
;

insert into `timesheet`
(`person_id`, `position_id`, `on_duty`, `off_duty`)
values
(600, 702, now(), null)
;
