/* Add a human-readable DESCRIPTION to each event-access rule.

   A "grant" on the admin page is a group of EVENT_ACCESS rows that share an
   access level, validity, and date window. The description is a shared,
   grant-level note explaining the purpose/reasoning for the grant, so it's
   part of what identifies a grant: editing it rewrites every member row. */

alter table `EVENT_ACCESS` add column `DESCRIPTION` varchar(255) not null default '';

update `SCHEMA_INFO`
set `VERSION` = 37
where true;
