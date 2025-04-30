FROM golang:alpine AS build

WORKDIR /app

# Install all the module dependencies early, so that this layer
# can be cached before ranger-ims-go code is copied over.
COPY go.mod go.sum ./
RUN go mod download

# Copy everything in the repo, including the .git directory,
# because we want Go to bake the repo's state into the build.
# See https://pkg.go.dev/debug/buildinfo#BuildInfo
COPY ./ ./

# Fetch client deps that we need to embed in the binary
RUN go run bin/fetchclientdeps/fetchclientdeps.go

# Build the server
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/ranger-ims-go

# Start a new stage and only copy over the IMS binary.
FROM alpine:latest
COPY --from=build /app/ranger-ims-go /
EXPOSE 80
CMD ["/ranger-ims-go", "serve"]
