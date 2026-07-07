--
-- See the file COPYRIGHT for copyright information.
--
-- Licensed under the Apache License, Version 2.0 (the "License");
-- you may not use this file except in compliance with the License.
-- You may obtain a copy of the License at
--
--     http://www.apache.org/licenses/LICENSE-2.0
--
-- Unless required by applicable law or agreed to in writing, software
-- distributed under the License is distributed on an "AS IS" BASIS,
-- WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
-- See the License for the specific language governing permissions and
-- limitations under the License.
--

-- These tables are IMS's own user directory, used when IMS_DIRECTORY is set
-- to "ims" rather than "clubhousedb". They are unused (and empty) on
-- deployments that use a Clubhouse database as their directory.

create table DIRECTORY_PERSON (
    ID       bigint       not null auto_increment,
    HANDLE   varchar(128) not null,
    EMAIL    varchar(256),
    -- An argon2id PHC-format hash. This may also hold a non-PHC placeholder
    -- string that can never verify, for users who have no password set.
    PASSWORD varchar(256) not null,
    ACTIVE   boolean      not null default true,
    ONSITE   boolean      not null default false,

    primary key (ID),
    unique key UNIQUE_HANDLE (HANDLE),
    unique key UNIQUE_EMAIL (EMAIL)
) DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

create table DIRECTORY_TEAM (
    ID     bigint       not null auto_increment,
    TITLE  varchar(128) not null,
    ACTIVE boolean      not null default true,

    primary key (ID),
    unique key UNIQUE_TITLE (TITLE)
) DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

create table DIRECTORY_POSITION (
    ID     bigint       not null auto_increment,
    TITLE  varchar(128) not null,
    ACTIVE boolean      not null default true,

    primary key (ID),
    unique key UNIQUE_TITLE (TITLE)
) DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

create table DIRECTORY_PERSON__TEAM (
    PERSON_ID bigint not null,
    TEAM_ID   bigint not null,

    primary key (PERSON_ID, TEAM_ID),
    foreign key (PERSON_ID) references DIRECTORY_PERSON (ID) on delete cascade,
    foreign key (TEAM_ID)   references DIRECTORY_TEAM (ID)   on delete cascade
) DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

create table DIRECTORY_PERSON__POSITION (
    PERSON_ID   bigint not null,
    POSITION_ID bigint not null,

    primary key (PERSON_ID, POSITION_ID),
    foreign key (PERSON_ID)   references DIRECTORY_PERSON (ID)   on delete cascade,
    foreign key (POSITION_ID) references DIRECTORY_POSITION (ID) on delete cascade
) DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

update `SCHEMA_INFO`
set `VERSION` = 34
where true;
