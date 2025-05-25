# syntax=docker/dockerfile:1.7-labs

FROM golang:alpine AS build

WORKDIR /app

# Install all the module dependencies early, so that this layer
# can be cached before ranger-ims-go code is copied over.
COPY go.mod go.sum ./
RUN go mod download

# Fetch client deps that we need to embed in the binary
COPY ./bin/fetchclientdeps/ ./bin/fetchclientdeps/
RUN go run ./bin/fetchclientdeps/fetchclientdeps.go

# Copy everything in the repo, including the .git directory,
# because we want Go to bake the repo's state into the build.
# See https://pkg.go.dev/debug/buildinfo#BuildInfo
COPY --exclude=playwright ./ ./

# Build the server
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/ranger-ims-go

# Start a new stage and only copy over the IMS binary.
FROM alpine:latest
# Include curl for the health check's sake
RUN apk add curl
COPY --from=build /app/ranger-ims-go /

# Use a non-root user to run the server
USER daemon:daemon

# Docker-specific default configuration
ENV IMS_HOSTNAME="0.0.0.0"
ENV IMS_PORT="80"
ENV IMS_DIRECTORY="ClubhouseDB"

# This should match the IMS_PORT above
EXPOSE 80

CMD ["/ranger-ims-go", "serve"]
