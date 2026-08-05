/* Add embargo times for an Event's place locations and map link.

   Camp and art placement is confidential until Burning Man publishes it, so
   IMS must hold back those locations (and the event's map) from everyone but
   IMS admins until the release time arrives. A null column means no embargo:
   the data is available to anyone who can read the event's places. */

alter table `EVENT`
    -- Time at which camp locations become visible to non-admins.
    add column `CAMP_LOCATIONS_RELEASE` double after `MAP_URL`,
    -- Time at which art locations become visible to non-admins.
    add column `ART_LOCATIONS_RELEASE`  double after `CAMP_LOCATIONS_RELEASE`,
    -- Time at which the event's MAP_URL becomes visible to non-admins.
    add column `MAP_URL_RELEASE`        double after `ART_LOCATIONS_RELEASE`;

update `SCHEMA_INFO`
set `VERSION` = 40
where true;
