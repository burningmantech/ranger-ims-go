# syntax=docker/dockerfile:1.7-labs

# -----------------
# Build image stage
# -----------------
FROM golang:alpine AS build

# Used for setcap below
RUN apk add --no-cache libcap
# Used by Go when building to inject repo metadata into the program
RUN apk add --no-cache git

WORKDIR /app

# Install all the module dependencies early, so that this layer
# can be cached before ranger-ims-go code is copied over.
COPY go.mod go.sum ./
RUN go mod download

# Pre-compile the code generators into the Go build cache. The only inputs to
# this layer are go.mod and go.sum, so it stays cached when only source changes,
# leaving the generate step below with just a link to do.
#
# Deliberately not a `RUN --mount=type=cache` on GOCACHE: BuildKit cache mounts
# aren't exported by `cache-to: type=gha`, so they don't survive across CI
# runners. The warm build cache has to live in the layer itself to be restorable.
RUN CGO_ENABLED=0 go build -o /tmp/tools/ \
      github.com/sqlc-dev/sqlc/cmd/sqlc \
      github.com/a-h/templ/cmd/templ \
      github.com/microsoft/typescript-go/cmd/tsgo

# Fetch client deps that we need to embed in the binary
COPY ./bin/fetchbuilddeps/ ./bin/fetchbuilddeps/
RUN go run ./bin/fetchbuilddeps/fetchbuilddeps.go

# Copy everything in the repo, including the .git directory,
# because we want Go to bake the repo's state into the build.
# See https://pkg.go.dev/debug/buildinfo#BuildInfo
COPY ./ ./

# None of the sqlc/templ/tsgo output is checked in, so generate it here. This
# re-runs fetchbuilddeps too, but that's a no-op: it hash-checks the files the
# layer above already fetched and skips the download.
RUN CGO_ENABLED=0 go run bin/build/build.go -generate-only

# Build the server
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/ranger-ims-go

# Allow IMS to bind to privileged port numbers
RUN setcap "cap_net_bind_service=+ep" /app/ranger-ims-go


# --------------------
# Deployed image stage
# --------------------
FROM alpine:latest
COPY --from=build /app/ranger-ims-go /opt/ims/bin/ims

# Docker-specific default configuration
ENV IMS_HOSTNAME="0.0.0.0"
ENV IMS_PORT="80"
ENV IMS_DB_STORE_TYPE="mariadb"
ENV IMS_DIRECTORY="clubhousedb"

# Use a non-root user to run the server
USER daemon:daemon

# This should match the IMS_PORT above
EXPOSE 80

CMD [ "/opt/ims/bin/ims", "serve" ]
