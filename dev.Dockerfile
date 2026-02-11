FROM golang:alpine
RUN apk add --no-cache git
RUN go install github.com/air-verse/air@v1.64.0
WORKDIR /src
ENTRYPOINT ["air"]
