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
(600,   "Hardware", "hardware@example.com", "$argon2id$v=19$m=65536,t=3,p=4$ipVDinFcaxnDwt20s1YPZg$Y1XUdJBb4+VVy7hQ29ORNZvgsMu7ySbzLfdXgGMO6lo",  "active",   true),
(601,   "Loosy",    "loosy@example.com",    concat(":", sha1("Loosy")),     "active",   true),
(602,   "Doggy",    "doggy@example.com",    concat(":", sha1("Doggy")),     "active",   true),
(603,   "Runner",   "runner@example.com",   concat(":", sha1("Runner")),    "active",   true),
-- The below password is "TheMan"
(604,   "TheMan",   "theman@example.com",   "$argon2id$v=19$m=65536,t=1,p=12$YqQK04inaw4iIkQf5k0hyg$O1/TG6705h9w6R0C96qodP26JkN1pwM1vTod07yz3BM",    "active",   true)
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
