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
