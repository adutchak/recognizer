FROM golang:1.20-buster AS builder
RUN apt-get update && apt-get install --no-install-recommends -y git
WORKDIR $GOPATH/src/recognizer

COPY go.mod .
COPY go.sum .
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-extldflags=-static" -o /tmp/recognizer  main.go

FROM debian:11-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates
COPY --from=builder /tmp/recognizer /usr/local/bin/recognizer
CMD ["/usr/local/bin/recognizer"]
