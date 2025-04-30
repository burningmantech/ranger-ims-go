FROM golang:1.24.2-alpine3.21 AS build

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY *.go ./
COPY api/ ./api/
COPY auth/ ./auth/
COPY bin/ ./bin/
COPY cmd/ ./cmd/
COPY conf/ ./conf/
COPY directory/ ./directory/
COPY json/ ./json/
COPY store/ ./store/
COPY web/ ./web/

RUN bin/fetch_client_deps.sh
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/ranger-ims-go

FROM alpine:3.21
COPY --from=build /app/ranger-ims-go /
EXPOSE 80
CMD ["/ranger-ims-go", "serve"]
