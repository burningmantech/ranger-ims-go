alter table `INCIDENT`
add column `LOCATION_ADDRESS` varchar(1024)
    after `LOCATION_NAME`
;

update
    INCIDENT as i
        left join (
        select
            EVENT,
            ID,
            NAME
        from
            CONCENTRIC_STREET
    ) as cs
        on i.EVENT = cs.EVENT
            and i.LOCATION_CONCENTRIC = cs.ID
set
    LOCATION_ADDRESS = if(
        i.LOCATION_RADIAL_HOUR is not null
        or i.LOCATION_RADIAL_MINUTE is not null
        or cs.NAME is not null,
      concat(
          coalesce(convert(i.LOCATION_RADIAL_HOUR, char), '?'),
          ':',
          coalesce(lpad(convert(i.LOCATION_RADIAL_MINUTE, char), 2, '0'), '??'),
          ' & ',
          coalesce(cs.NAME, '?')
      ),
      null
   )
where true
;


update `SCHEMA_INFO`
set `VERSION` = 24
where true;
