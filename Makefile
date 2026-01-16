# This file was created with much inspiration from
# https://www.alexedwards.net/blog/a-time-saving-makefile-for-your-go-projects

main_package_path = ./
binary_name = ranger-ims-go

## build: build the server
.PHONY: build
build:
	go run bin/build/build.go

## test: run all go tests
.PHONY: test
test:
	go test -v -race ./... && echo "All tests passed"

## cover: run all go tests and open a coverage report
.PHONY: cover
cover:
	go test -race -covermode=atomic -coverprofile=coverage.out --coverpkg ./... ./...
	go tool cover -html=coverage.out

## run: run the server
.PHONY: run
run: build
	./${binary_name} serve

## run/live: run the application with reloading on file changes
.PHONY: run/live
run/live:
	go tool air

## compose/live: run the application stack with live reloading
.PHONY: compose/live
compose/live:
	docker compose -f docker-compose.dev.yml up

## upgrade-deps: upgrade all Go deps
.PHONY: upgrade-deps
upgrade-deps:
	go get -t -u ./...
	go mod tidy

# This is kind of silly, but it's similar to what the Go website itself
# does to check the latest version.
LATEST_GO_VERSION = $(shell curl "https://go.dev/dl/?mode=json" | grep version | sort | tail -n 1 | grep -oG '[0-9.]\+')

# upgrade-go: updates go.mod to the latest go language version
.PHONY: upgrade-go
upgrade-go:
	go mod edit -go=$(LATEST_GO_VERSION)

# upgrade-all: upgrade Go toolchain and code dependencies
upgrade-all: upgrade-go upgrade-deps
