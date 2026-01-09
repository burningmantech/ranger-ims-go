# Ranger IMS Server - now in Go

![](/gopher.png)

The Ranger Incident Management System is used by the Black Rock Rangers to track incidents
that occur in Black Rock City.

## Getting started with IMS development:

1. Clone the repo
2. Install Go and have `go` on your PATH. https://go.dev/dl
3. (Optional: if you want to run the integration tests) install Docker Desktop or Docker Engine. https://www.docker.com/
4. (Optional: if you want to run the Playwright tests) install Playwright: https://playwright.dev/docs/intro
5. Do a one-time fetch of external build dependencies into your repo, by running
   ```shell
    go run bin/fetchbuilddeps/fetchbuilddeps.go
   ```

## Run IMS locally with docker compose

The fastest way to get a local IMS server up-and-running is to use Docker Compose. This
requires only that you install Docker in advance (you don't even need Go!). This approach
uses a live code-reloading mechanism, so any changes to the source code will cause the
server to rebuild and relaunch.

There's good documentation in `docker-compose.dev.yml` that's worth a read.

```shell
docker compose -f docker-compose.dev.yml up
# or
make compose/live
```

## Run IMS locally with MariaDB

**This documentation is slightly wrong, because the TestUsers part is no longer a thing. Just use the docker compose way for now.**

1. Have a local MariaDB server running. An empty database is fine, as the IMS program will
   migrate your DB automatically on startup from nothing. e.g.
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
3. Run the following to build and launch the server. These *should* work on Windows as well as OSX
   and Linux, but Windows is so far untested.
   ```shell
   go run bin/build/build.go
   ./ranger-ims-go serve
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

## Build and run with Docker

```shell
docker build --tag ranger-ims-go .
docker run --env-file .env -it -p 80:8080 ranger-ims-go:latest
```

or use `docker compose up`

## Upgrade Go dependencies

Upgrade the Go toolchain simply by increasing the Go value in `go.mod`, e.g. https://github.com/burningmantech/ranger-ims-go/pull/64. Even Go major version upgrades (e.g. 1.23 to 1.24) are very unlikely to break anything, thanks to the Go 1.0 backward compatibility guarantee. If all the tests pass, you're all good.

This line in go.mod should be left as the only line in the repo that specifies the Go version. For example, the Dockerfile depends on Go, but it inherits the value in go.mod.

Upgrade all Go dependencies by running:

```shell
# Upgrade all normal and test dependencies
go get -t -u ./...

# Tidy up go.mod and go.sum
go mod tidy
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
