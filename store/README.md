# IMS DB

## sqlc

We use [sqlc](https://docs.sqlc.dev/) to generate Go code based on the schema in `current.sql`
and queries in `queries.sql`. sqlc is not an ORM (Abraham dislikes ORMs).

## Migrations

To alter the IMS DB, create a new sql file in the `schema` directory. Follow the format of other files there.

A migration file must minimally contain a `SCHEMA_INFO` update, plus whatever other changes you're making.

```sql
-- example: for 03-from-02.sql
update `SCHEMA_INFO` set `VERSION` = 3;
```

After that, you must change the `current.sql` file to reflect your changes as well. Again, minimally, you must
find the `SCHEMA_INFO` update line near the top and change the schema version.

Once you've made your changes to your migration file and `current.sql`, make sure the migration test in
`store/integration` passes, via `go test ./store/integration`.

After that, you'll want to regenerate the `sqlc` code, either via `./bin/build.sh`, or `go tool sqlc generate`,
and make sure all the Go code still compiles, by running `go test ./...`. If your migration affected preexisting
tables and columns, then you'll need to update the `sqlc` input file, `store/queries.sql`, and any Go code that
interacts with those tables or columns.
