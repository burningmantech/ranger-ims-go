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

## test/ts: run all TypeScript tests
.PHONY: test/ts
test/ts:
	npm install
	npm test

## cover: run all go tests and open a coverage report
.PHONY: cover
cover:
	go test -race -covermode=atomic -coverprofile=coverage.out --coverpkg ./... ./...
	go tool cover -html=coverage.out

## cover/ts: run all TypeScript tests and open a coverage report
.PHONY: cover/ts
cover/ts:
	npm install
	npm run test:coverage
	open coverage/index.html

## run: run the server
.PHONY: run
run: build
	./${binary_name} serve

## run/live: run the application with reloading on file changes
.PHONY: run/live
run/live:
	go tool air


# The compose targets pass an explicit --env-file so the stacks read their own
# env files instead of the ./.env used by `ims serve` run directly on your host.
# The .env.dev / .env.quickstart files are gitignored and created on first run
# from their checked-in *.example templates by the rules below, with each
# replace_with_secure_random placeholder filled in with a distinct random value.
define fill-secrets
	@while grep -q 'replace_with_secure_random' $@; do \
		secret=$$(openssl rand -hex 24) || exit 1; \
		awk -v s="$$secret" '!done && sub(/replace_with_secure_random/, s) { done=1 } { print }' $@ > $@.tmp && mv $@.tmp $@; \
	done
endef

.env.dev:
	cp .env.dev.example .env.dev
	$(fill-secrets)
.env.quickstart:
	cp .env.quickstart.example .env.quickstart
	$(fill-secrets)

## compose/build: build the stack for live reloading
.PHONY: compose/build
compose/build: .env.dev
	docker compose --env-file .env.dev -f docker-compose.dev.yml build --pull

## compose/live: run the application stack with live reloading
.PHONY: compose/live
compose/live: .env.dev
	docker compose --env-file .env.dev -f docker-compose.dev.yml up

## compose/quickstart: run IMS with the IMS-native directory (no Clubhouse DB)
.PHONY: compose/quickstart
compose/quickstart: .env.quickstart
	docker compose --env-file .env.quickstart -f docker-compose.quickstart.yml up --build

## upgrade-deps: upgrade all Go deps
.PHONY: upgrade-deps
upgrade-deps:
	go get tool
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
