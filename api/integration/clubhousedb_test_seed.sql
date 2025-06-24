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
(6000,   "AdminTestRanger", "admintestranger@example.com",  concat(":", sha1(")'(")),       "active",   true),
(6001,   "AliceTestRanger", "alicetestranger@example.com",  concat(":", sha1("password")),  "active",   true)
;

insert into `position`
(`id`, `title`)
values
(7000, "Nooperator")
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
(6001, 7000, now(), null)
;
