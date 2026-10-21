# syntax=docker/dockerfile:1.7-labs

# Builds the Rust port of the IMS server. Run from the repo root:
#
#   docker build -f rust/Dockerfile -t ranger-ims-rust .

# -----------------------------------------------------------------
# The web client's JavaScript and the external client libraries are
# produced by the Go build tooling, as for the Go server's image.
# -----------------------------------------------------------------
FROM golang:alpine AS web
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY ./bin/fetchbuilddeps/ ./bin/fetchbuilddeps/
RUN go run ./bin/fetchbuilddeps/fetchbuilddeps.go
COPY ./ ./
RUN CGO_ENABLED=0 go run bin/build/build.go -generate-only
# The git metadata the Rust build can't read without .git.
RUN git rev-parse HEAD > /app/git-revision || echo deadbeef > /app/git-revision
RUN echo "$(TZ=UTC git show -s --format=%cd --date=format-local:%Y%m%d%H%M%S HEAD)-$(git rev-parse --short=12 HEAD)" \
      > /app/version-name || date -u +%Y%m%d%H%M%S > /app/version-name

# -----------------
# Build the server
# -----------------
FROM rust:alpine AS build
# musl-dev, a C toolchain and cmake build aws-lc-sys (the AWS SDK's TLS
# crypto); libcap is used for setcap below
RUN apk add --no-cache musl-dev build-base cmake perl libcap
WORKDIR /app
COPY --exclude=rust/target --exclude=.git ./ ./
COPY --from=web /app/web/static/ ./web/static/
COPY --from=web /app/git-revision /app/version-name /tmp/
# All TLS is rustls, so on musl this links fully statically, like the Go server
RUN IMS_GIT_REVISION=$(cat /tmp/git-revision) IMS_VERSION_NAME=$(cat /tmp/version-name) \
      cargo build --release --manifest-path rust/Cargo.toml \
    && cp rust/target/release/ims /app/ims

# Allow IMS to bind to privileged port numbers
RUN setcap "cap_net_bind_service=+ep" /app/ims

# --------------------
# Deployed image stage
# --------------------
FROM alpine:latest
COPY --from=build /app/ims /opt/ims/bin/ims

# Docker-specific default configuration, as in the Go server's image
ENV IMS_HOSTNAME="0.0.0.0"
ENV IMS_PORT="80"
ENV IMS_DB_STORE_TYPE="mariadb"
ENV IMS_DIRECTORY="clubhousedb"

USER daemon:daemon
EXPOSE 80
CMD [ "/opt/ims/bin/ims", "serve" ]
