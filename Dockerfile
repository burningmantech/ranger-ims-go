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

# Fetch client deps that we need to embed in the binary
COPY ./bin/fetchbuilddeps/ ./bin/fetchbuilddeps/
RUN go run ./bin/fetchbuilddeps/fetchbuilddeps.go

# Copy everything in the repo, including the .git directory,
# because we want Go to bake the repo's state into the build.
# See https://pkg.go.dev/debug/buildinfo#BuildInfo
COPY --exclude=playwright ./ ./

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
ENV IMS_DIRECTORY="ClubhouseDB"

# Use a non-root user to run the server
USER daemon:daemon

# This should match the IMS_PORT above
EXPOSE 80

CMD [ "/opt/ims/bin/ims", "serve" ]
