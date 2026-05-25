alter table `VISIT`
    add column `GUEST_CAMP_CONTACTS` varchar(512) after `GUEST_CAMP_DESCRIPTION`,
    add column `GUEST_ACTION_PLAN` varchar(512) after `GUEST_DESCRIPTION`,
    add column `RESOURCE_SITTER` varchar(256) after `DEPARTURE_STATE`,
    add column `RESOURCE_BED_ID` varchar(64) after `RESOURCE_SITTER`
;

update `SCHEMA_INFO`
set `VERSION` = 31
where true;
