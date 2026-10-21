# This file was created with much inspiration from
# https://www.alexedwards.net/blog/a-time-saving-makefile-for-your-go-projects

main_package_path = ./
binary_name = ranger-ims-go

## build: build the server
.PHONY: build
build:
	go run bin/build/build.go

## generate: run the code generators (sqlc, templ, tsc) without building the server.
## None of the generated code is checked in, so a fresh clone needs this (or `build`)
## before `go test`, `go vet` or your editor's language server will work.
.PHONY: generate
generate:
	go run bin/build/build.go -generate-only

## test: run all go tests
.PHONY: test
test:
	go test -v -race ./... && echo "All tests passed"

## test/ts: run all TypeScript tests
.PHONY: test/ts
test/ts:
	npm install
	npm test

## test/e2e: run Playwright browser tests. Reuses a running IMS stack on
## :8080 if there is one; otherwise starts (and later stops) compose/live.
.PHONY: test/e2e
test/e2e:
	cd playwright && npm install && npx playwright install && npx playwright test

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

## rust/build: build the experimental Rust port of the server (see rust/)
.PHONY: rust/build
rust/build:
	cargo build --manifest-path rust/Cargo.toml

## rust/test: run the Rust port's own tests
.PHONY: rust/test
rust/test:
	cargo test --manifest-path rust/Cargo.toml

## rust/test/parity: run the Go API integration suite against the Rust port
## (requires Docker)
.PHONY: rust/test/parity
rust/test/parity: rust/build
	IMS_TEST_TARGET=rust IMS_RUST_BIN=$(CURDIR)/rust/target/debug/ims go test ./api/integration

## rust/compose: run the Rust port with its own seeded databases on :8081
.PHONY: rust/compose
rust/compose:
	docker compose -f docker-compose.rust.yml up --build

## rust/run: run the Rust port of the server, configured from ./.env like `run`
.PHONY: rust/run
rust/run: rust/build
	./rust/target/debug/ims serve

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

## upgrade/deps/go: upgrade all Go deps
.PHONY: upgrade/deps/go
upgrade/deps/go:
	go get -u tool
	go get -t -u ./...
	go mod tidy

## upgrade/deps/npm: upgrade all npm deps (root + playwright) to their latest
## versions, rewriting the package.json ranges and refreshing each lockfile.
.PHONY: upgrade/deps/npm
upgrade/deps/npm:
	npx --yes npm-check-updates -u
	npm install
	npx --yes npm-check-updates -u --cwd playwright
	npm install --prefix playwright

## upgrade/deps/actions: upgrade all GitHub Actions in .github/workflows to their
## latest versions, keeping them pinned by commit SHA with a version comment, and
## upgrade the pre-commit hook repos in .pre-commit-config.yaml to their latest tags.
## Borrows the gh CLI's token when GITHUB_TOKEN isn't set, to avoid API rate limits.
.PHONY: upgrade/deps/actions
upgrade/deps/actions:
	GITHUB_TOKEN="$${GITHUB_TOKEN:-$$(gh auth token 2>/dev/null)}" \
		go tool github.com/suzuki-shunsuke/pinact/v5/cmd/pinact run --update
	uvx pre-commit autoupdate

# This is kind of silly, but it's similar to what the Go website itself
# does to check the latest version.
LATEST_GO_VERSION = $(shell curl "https://go.dev/dl/?mode=json" | grep version | sort | tail -n 1 | grep -oG '[0-9.]\+')

# upgrade/go: updates go.mod to the latest go language version
.PHONY: upgrade/go
upgrade/go:
	go mod edit -go=$(LATEST_GO_VERSION)

# upgrade/all: upgrade the Go version, Go and npm deps, and GitHub Actions
.PHONY: upgrade/all
upgrade/all: upgrade/go upgrade/deps/go upgrade/deps/npm upgrade/deps/actions
