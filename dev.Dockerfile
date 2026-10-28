FROM golang:alpine
RUN apk add --no-cache git
RUN go install github.com/air-verse/air@latest
WORKDIR /src
ENTRYPOINT ["air"]
