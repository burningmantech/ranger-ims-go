alter table `REPORT_ENTRY`
add column `ATTACHED_FILE_ORIGINAL_NAME` varchar(128),
add column `ATTACHED_FILE_MEDIA_TYPE` varchar(128);

update `REPORT_ENTRY`
set `ATTACHED_FILE_ORIGINAL_NAME` = `ATTACHED_FILE`,
    `ATTACHED_FILE_MEDIA_TYPE` = 'application/octet-stream'
where `ATTACHED_FILE` is not null;

update `SCHEMA_INFO`
set `VERSION` = 16
where true;
