# Ranger IMS Server - now in Go

![](/gopher.jpeg)

The Ranger Incident Management System is used by the Black Rock Rangers to track incidents
that occur in Black Rock City.

## Getting started with IMS development:

1. Clone the repo
2. Install a recent version of Go: https://go.dev/dl
3. Install the TypeScript transpiler, `tsc`: https://www.typescriptlang.org/download
4. Install Docker Desktop or Docker Engine (needed for tests). https://www.docker.com/
5. Have `go` and `tsc` on your PATH
6. (Optionally) install Playwright: https://playwright.dev/docs/intro
7. Do a one-time fetch of external client deps into your repo, by running
   ```shell
    go run bin/fetchclientdeps/fetchclientdeps.go
   ```

## Run tests

To run all the non-Playwright tests, just do:

```shell
go test ./...
```

or to run all those tests and see a coverage report, do:

```shell
go test -coverprofile=coverage.out --coverpkg ./... ./... && go tool cover -html=coverage.out
```

## Run IMS locally

1. Copy `.env.example` as `.env`, and set the configuration for your machine.
   (Note: ranger-ims-server used a `conf/imsd.conf` file. ranger-ims-go uses `.env` instead)
2. Run the following to build and launch the server: `bin/build.sh && ./ranger-ims-go serve`

## Build and run with Docker

```shell
docker build --tag ranger-ims-go .
docker run --env-file .env -it -p 80:8080 ranger-ims-go:latest
```
