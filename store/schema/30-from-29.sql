rename table `DESTINATION` to `PLACE`;

update `SCHEMA_INFO`
set `VERSION` = 30
where true;
