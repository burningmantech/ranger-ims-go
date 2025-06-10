insert into EVENT (ID, NAME)
values  (1, '2025'),
        (2, '2031'),
        (3, '2032'),
        (4, 'Test');

insert into CONCENTRIC_STREET (EVENT, ID, NAME)
values  (1, '0', 'Esplanade'),
        (1, '1', 'Arno'),
        (1, '2', 'Botticelli'),
        (1, '3', 'Cosimo'),
        (4, '0', 'Esplanade'),
        (4, '1', 'Arcade'),
        (4, '2', 'Ballyhoo'),
        (4, '3', 'Carny'),
        (4, '4', 'Donniker'),
        (4, '5', 'Ersatz');

insert into EVENT_ACCESS (ID, EVENT, EXPRESSION, MODE, VALIDITY)
values  (1, 1, '*', 'write', 'always'),
        (2, 2, '*', 'read', 'always'),
        (3, 3, '*', 'report', 'always'),
        (4, 4, '*', 'write', 'always');

insert into INCIDENT (EVENT, NUMBER, CREATED, PRIORITY, STATE, STARTED, SUMMARY, LOCATION_NAME, LOCATION_CONCENTRIC, LOCATION_RADIAL_HOUR, LOCATION_RADIAL_MINUTE, LOCATION_DESCRIPTION)
values  (4, 1, 1748459852.644699, 3, 'dispatched', 1748459852.644699, 'Something bad!', 'Dog Camp', '2', 2, 10, null),
        (4, 2, 1748460242.68441, 3, 'new', 1748460242.68441, 'Report from the field', null, null, null, null, null);

insert into FIELD_REPORT (EVENT, NUMBER, CREATED, SUMMARY, INCIDENT_NUMBER)
values  (4, 1, 1748460231.287398, 'Report from the field', 2);

insert into REPORT_ENTRY (ID, AUTHOR, TEXT, CREATED, GENERATED, STRICKEN, ATTACHED_FILE)
values  (1, 'Abraham', 'Changed priority: 3
Changed state: new
Changed summary: Something bad!', 1748459852.649554, 1, 0, null),
        (2, 'Abraham', 'Changed state: dispatched', 1748459854.045146, 1, 0, null),
        (3, 'Abraham', 'Added Ranger: Hardware', 1748460179.390828, 1, 0, null),
        (4, 'Abraham', 'Added Ranger: Defect', 1748460180.76589, 1, 0, null),
        (5, 'Abraham', 'Added type: Admin', 1748460183.617861, 1, 0, null),
        (6, 'Abraham', 'Added type: MOOP', 1748460184.78715, 1, 0, null),
        (7, 'Abraham', 'Changed location name: Dog Camp', 1748460192.069875, 1, 0, null),
        (8, 'Abraham', 'Changed location radial hour: 2', 1748460193.558534, 1, 0, null),
        (9, 'Abraham', 'Changed location radial minute: 10', 1748460194.422526, 1, 0, null),
        (10, 'Abraham', 'Changed location concentric: 2', 1748460196.736211, 1, 0, null),
        (11, 'Abraham', 'Something happened!', 1748460208.873552, 0, 0, null),
        (12, 'Abraham', 'Changed summary to: Report from the field', 1748460231.289929, 1, 0, null),
        (13, 'Abraham', 'Something happened out in the dust', 1748460241.587492, 0, 0, null),
        (14, 'Abraham', 'Changed summary: Report from the field
Added Ranger: Abraham', 1748460242.688133, 1, 0, null),
        (15, 'Abraham', 'Attached to incident: 2', 1748460242.696368, 1, 0, null),
        (16, 'Abraham', 'Added Ranger: Loosy', 1748460254.830443, 1, 0, null),
        (17, 'Abraham', 'Removed Ranger: Abraham', 1748460256.071517, 1, 0, null);

insert into FIELD_REPORT__REPORT_ENTRY (EVENT, FIELD_REPORT_NUMBER, REPORT_ENTRY)
values  (4, 1, 12),
        (4, 1, 13),
        (4, 1, 15);

insert into INCIDENT_TYPE (ID, NAME, HIDDEN)
values  (3, 'Sound Complaint', 0),
        (4, 'Found Child', 0),
        (5, 'Lost Child', 0),
        (6, 'MOOP', 0),
        (7, 'Medical', 0),
        (8, 'Transport', 0);

insert into INCIDENT__INCIDENT_TYPE (EVENT, INCIDENT_NUMBER, INCIDENT_TYPE)
values  (4, 1, 1),
        (4, 1, 6);

insert into INCIDENT__RANGER (ID, EVENT, INCIDENT_NUMBER, RANGER_HANDLE)
values  (1, 4, 1, 'Hardware'),
        (2, 4, 1, 'Defect'),
        (4, 4, 2, 'Loosy');

insert into INCIDENT__REPORT_ENTRY (EVENT, INCIDENT_NUMBER, REPORT_ENTRY)
values  (4, 1, 1),
        (4, 1, 2),
        (4, 1, 3),
        (4, 1, 4),
        (4, 1, 5),
        (4, 1, 6),
        (4, 1, 7),
        (4, 1, 8),
        (4, 1, 9),
        (4, 1, 10),
        (4, 1, 11),
        (4, 2, 14),
        (4, 2, 16),
        (4, 2, 17);
