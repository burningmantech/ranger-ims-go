insert into EVENT (ID, NAME, IS_GROUP, PARENT_GROUP)
values  (6, 'TestBRC', true, null);

insert into EVENT (ID, NAME, IS_GROUP, PARENT_GROUP)
values  (1, '2025', false, 6),
        (2, '2026', false, 6),
        (3, '2031', false, 6),
        (4, '2032', false, 6),
        (5, 'Test', false, null);

insert into CONCENTRIC_STREET (EVENT, ID, NAME)
values  (1, '0', 'Esplanade'),
        (1, '1', 'Alfa'),
        (1, '2', 'Bravo'),
        (1, '3', 'Charlie'),
        (1, '4', 'Delta'),
        (1, '5', 'Echo'),
        (1, '6', 'Foxtrot'),
        (4, '0', 'Esplanade'),
        (4, '1', 'Arcade'),
        (4, '2', 'Ballyhoo'),
        (4, '3', 'Carny'),
        (4, '4', 'Donniker'),
        (4, '5', 'Ersatz');

insert into EVENT_ACCESS (ID, EVENT, EXPRESSION, MODE, VALIDITY)
values  (1, 6, '*', 'write', 'always'),
        (2, 2, '*', 'read', 'always'),
        (3, 3, '*', 'report', 'always'),
        (4, 4, '*', 'write', 'always');

insert into INCIDENT (EVENT, NUMBER, CREATED, PRIORITY, STATE, STARTED, SUMMARY, LOCATION_NAME, LOCATION_CONCENTRIC, LOCATION_RADIAL_HOUR, LOCATION_RADIAL_MINUTE, LOCATION_DESCRIPTION)
values  (1, 1, 1748459852.644699, 3, 'dispatched', 1748459852.644699, 'Something bad 2025!', 'Dog Camp', null, null, null, null),
        (1, 2, 1748460242.68441, 3, 'new', 1748460242.68441, 'Report from the field 2025', null, null, null, null, null),
        (2, 1, 1748459852.644699, 3, 'dispatched', 1748459852.644699, 'Something bad 2026!', 'Dog Camp', null, null, null, null),
        (2, 2, 1748460242.68441, 3, 'new', 1748460242.68441, 'Report from the field 2026', null, null, null, null, null);

insert into FIELD_REPORT (EVENT, NUMBER, CREATED, SUMMARY, INCIDENT_NUMBER)
values  (1, 1, 1748460231.287398, 'Report from the field', 2);

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
values  (1, 1, 12),
        (1, 1, 13),
        (1, 1, 15);

insert into INCIDENT_TYPE (ID, NAME, HIDDEN)
values  (3, 'Sound Complaint', 0),
        (4, 'Found Child', 0),
        (5, 'Lost Child', 0),
        (6, 'MOOP', 0),
        (7, 'Medical', 0),
        (8, 'Transport', 0);

insert into INCIDENT__INCIDENT_TYPE (EVENT, INCIDENT_NUMBER, INCIDENT_TYPE)
values  (1, 1, 1),
        (1, 1, 6);

insert into INCIDENT__RANGER (ID, EVENT, INCIDENT_NUMBER, RANGER_HANDLE)
values  (1, 1, 1, 'Hardware'),
        (2, 1, 1, 'Defect'),
        (4, 1, 2, 'Loosy');

insert into INCIDENT__REPORT_ENTRY (EVENT, INCIDENT_NUMBER, REPORT_ENTRY)
values  (1, 1, 1),
        (1, 1, 2),
        (1, 1, 3),
        (1, 1, 4),
        (1, 1, 5),
        (1, 1, 6),
        (1, 1, 7),
        (1, 1, 8),
        (1, 1, 9),
        (1, 1, 10),
        (1, 1, 11),
        (1, 2, 14),
        (1, 2, 16),
        (1, 2, 17);

insert into DESTINATION (EVENT, TYPE, NUMBER, NAME, LOCATION_STRING, EXTERNAL_DATA)
values  (1, 'camp', 0, 'Ranger Outpost Berlin', '3:00 & C', '{"contact_email":"rangers@burningman.org","description":"Ranger Outpost Berlin - The Black Rock Ranger Station in the 3:00 sector where participants in need of assistance can\\nseek out the help of a friendly Black Rock Ranger. Located at 3:00 and C.","hometown":"Black Rock City","images":[],"landmark":"Ranger Outpost Berlin","location":{"dimensions":"100+ x 300","exact_location":"Corner - facing man \\u0026 6:00","frontage":"3:00","intersection":"C","intersection_type":"\\u0026"},"location_string":"3:00 \\u0026 C","name":"Ranger Outpost Berlin","uid":"a1XVI00000A0O7p2AF","url":"https://rangers.burningman.org","year":2025}'),
        (1, 'camp', 1, 'Kidsville', '5:00 & E', '{"contact_email":null,"description":"Burning Man\'s best place for kids!  Come enjoy our trampolines, playgrounds, kid-friendly treats, and meet other families that burn with people under 18.","hometown":null,"images":[],"landmark":"Big Rainbow \\"Kidsville\\" Perimeter","location":{"dimensions":"450 x 575-","exact_location":"Corner - facing man \\u0026 2:00","frontage":"5:00","intersection":"E","intersection_type":"\\u0026"},"location_string":"5:00 \\u0026 E","name":"Kidsville","uid":"a1XVI000009Cb532AC","url":"http://kidsville.org","year":2025}'),
        (1, 'art', 0, 'Temple of the Deep', '12:00 2500\', Open Playa', '{"artist":"Miguel Arraiz","category":"Open Playa","contact_email":null,"description":"The Temple of the Deep is a sanctuary for grief, love, and introspection, formed beneath a massive black stone that appears to hover above participants. This dark, fractured element symbolizes the weight of loss and the strength found in healing, inspired by kintsugi, where brokenness is embraced and honored. Seven narrow entrances guide visitors through the journey of mourning, leading to a central gathering space mirroring BRC\'s layout. Alcoves and chapels offer solitude and remembrance, while the seamless integration with the desert transforms sorrow into connection, grounding participants in shared reflection.","donation_link":null,"guided_tours":false,"hometown":"Valencia, Spain","images":[{"gallery_ref":null,"thumbnail_url":null}],"location":{"category":"Open Playa","distance":2500,"gps_latitude":40.791799176283455,"gps_longitude":-119.19660218660613,"hour":12,"minute":0},"location_string":"12:00 2500\', Open Playa","name":"Temple of the Deep","program":"Honorarium","self_guided_tour_map":true,"uid":"a2IVI000000yWeZ2AU","url":"https://www.2025temple.com/","year":2025}'),
        (1, 'other', 0, 'Ranger Outpost Geneva', '7:20 & H', '{"contact_email":null,"description":"It\'s Ranger Outpost Geneva!","hometown":null,"images":[],"landmark":"A bunch of Rangers","location_string":"7:20 \\u0026 H","name":"Ranger Outpost Geneva","year":2025}');
