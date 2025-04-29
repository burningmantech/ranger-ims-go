FROM golang:1.24.2-alpine3.21 AS build

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY api/ ./api/
COPY auth/ ./auth/
COPY web/ ./web/
COPY store/ ./store/
COPY cmd/ ./cmd/
COPY json/ ./json/
COPY directory/ ./directory/
COPY bin/ ./bin/
COPY *.go ./

# Copy go files, but not toml files in this dir,
# which may contain real credentials
COPY conf/*.go ./conf/

RUN bin/fetch_client_deps.sh

RUN CGO_ENABLED=0 GOOS=linux go build -o /app/ranger-ims-go

FROM gcr.io/distroless/static-debian12
COPY --from=build /app /

EXPOSE 80

CMD ["/ranger-ims-go", "serve"]
