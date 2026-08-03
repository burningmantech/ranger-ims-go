/* Add a table for the Error Log.

   Until now, a failed API request left no trace beyond a line on the server's
   stderr, which isn't queryable and doesn't survive a container restart. Rows
   here record 5xx responses and recovered panics, along with enough request
   context to tell who hit what. Client errors (4xx) are deliberately excluded,
   as are any of the other requests that ACTION_LOG already covers. */

create table `ERROR_LOG` (
    `ID`                bigint not null auto_increment,
    `CREATED_AT`        double not null,

    -- response
    `HTTP_STATUS`       smallint not null,
    `RESPONSE_MESSAGE`  text,
    `INTERNAL_ERROR`    text,
    `STACK_TRACE`       text,

    -- request metadata
    `METHOD`            varchar(128),
    `PATH`              varchar(255),
    `REFERRER`          varchar(255),

    -- requestor metadata
    `USER_ID`           bigint,
    `USER_NAME`         varchar(128),
    `POSITION_ID`       bigint,
    `POSITION_NAME`     varchar(128),
    `CLIENT_ADDRESS`    varchar(128),
    `DURATION_MICROS`   bigint,

    primary key (`ID`)
) DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

update `SCHEMA_INFO`
set `VERSION` = 39
where true;
