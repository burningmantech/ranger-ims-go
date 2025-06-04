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
	go test -v -race ./...

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
	go tool air \
		--build.cmd "make build" --build.bin "./${binary_name} serve --print-config=false" --build.delay "100" \
		--build.include_ext "go,tpl,tmpl,templ,html,css,scss,js,ts,sql,jpeg,jpg,gif,png,bmp,svg,webp,ico,env" \
		--build.exclude_file "web/template/*_templ.go,web/static/*.js,store/imsdb/*.go,directory/clubhousedb/*.go" \
		--build.exclude_dir "playwright,ims-attachments" \
		--misc.clean_on_exit "true" --tmp_dir=air_tmp

## update: update all deps
.PHONY: update
update:
	go get -t -u ./...
	go mod tidy

# This is kind of hacky, but it's similar to what the Go website itself
# does to check the latest version.
LATEST_GO_VERSION = $(shell curl "https://go.dev/dl/?mode=json" | grep version | sort | tail -n 1 | grep -oG '[0-9.]\+')

# upgrade-go: updates go.mod to the latest go language version
.PHONY: upgrade-go
upgrade-go:
	go mod edit -go=$(LATEST_GO_VERSION)
