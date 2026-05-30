FROM golang:alpine
RUN apk add --no-cache git
RUN go install github.com/air-verse/air@v1.65.3
WORKDIR /src
ENTRYPOINT ["air"]
