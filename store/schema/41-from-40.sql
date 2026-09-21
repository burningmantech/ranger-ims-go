/* Record the mutation an action log row describes.

   The metadata columns say who did what to which record, but not what they
   actually changed. REQUEST_BODY holds the request's JSON body, which for the
   mutating endpoints is the mutation itself. It's null for routes that carry no
   body worth keeping: reads, logins, and attachment uploads. */

alter table `ACTION_LOG`
    add column `REQUEST_BODY` text after `REFERRER`;

update `SCHEMA_INFO`
set `VERSION` = 41
where true;
