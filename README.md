# Ranger IMS Server - now in Go

![](/gopher.jpeg)

The Ranger Incident Management System is used by the Black Rock Rangers to track incidents
that occur in Black Rock City.

## Getting started with IMS development:

1. Clone the repo
2. Install a recent version of Go. Have `go` on your PATH. https://go.dev/dl
3. (Optional: if you're doing TypeScript work in the repo) install the TypeScript transpiler and have `tsc` on your PATH: https://www.typescriptlang.org/download
4. (Optional: if you want to run the integration tests) install Docker Desktop or Docker Engine. https://www.docker.com/
6. (Optional: if you want to run the Playwright tests) install Playwright: https://playwright.dev/docs/intro
7. Do a one-time fetch of external client deps into your repo, by running
   ```shell
    go run bin/fetchclientdeps/fetchclientdeps.go
   ```

## Run tests

To run all the tests (excluding Playwright), just do:

```shell
go test ./...
```

or to run all those tests and see a coverage report, do:

```shell
go test -coverprofile=coverage.out --coverpkg ./... ./... && go tool cover -html=coverage.out
```

## Run IMS locally

1. Have a local MariaDB server running. An empty database is fine, as the IMS program will
   migrate your DB automatically on startup. e.g.
   ```shell
   password=$(openssl rand -hex 16)
   echo "Password is ${password}"
   docker run -it \
     -e MARIADB_RANDOM_ROOT_PASSWORD=true \
	 -e MARIADB_DATABASE=ims \
	 -e MARIADB_USER=rangers \
	 -e MARIADB_PASSWORD=${password} \
     -p 3306:3306 mariadb:10.5.27
   ```
2. Copy `.env.example` as `.env`, and set the various flags. Especially read the part in
   `.env.example` about `IMS_DIRECTORY` if you want to use TestUsers rather than a Clubhouse DB.
3. Run the following to build and launch the server: `bin/build.sh && ./ranger-ims-go serve`

## Build and run with Docker

```shell
docker build --tag ranger-ims-go .
docker run --env-file .env -it -p 80:8080 ranger-ims-go:latest
```

## Differences between the Go and Python IMS servers

1. We didn't bring over support for a SQLite IMS database, so MariaDB is the only option currently.
   It's kind of a pain supporting two different sets of SQLs statements and needing an abstraction layer
   in the middle. Also, sqlc doesn't support SQLite well yet, and this Go version of IMS makes heavy use
   of sqlc's glorious code generation. If we do end up wanting some lighter alternative to MariaDB for
   some reason, the easier thing would be to make a fake version of the Querier interface, i.e. creating
   an in-memory DB.
2. We use a `.env` file rather than `conf/imsd.conf` for local configuration. This ends up just being a
   lot simpler, since prod only uses env variables anyway, and this means each config setting just has
   one name.
3. We kept the "File" Directory type in spirit, but changed it to "TestUsers" and made it a compiled
   source file, `testusers.go`.
