FROM ghcr.io/hybridgroup/opencv:4.9.0 AS builder
ENV GOPATH /go

WORKDIR /go/src/gocv.io/x/gocv/

COPY go.mod /go/src/gocv.io/x/gocv/go.mod
COPY go.sum /go/src/gocv.io/x/gocv/go.sum 
RUN go mod download
COPY . /go/src/gocv.io/x/gocv/
RUN GOOS=linux go build -o /build/recognizer main.go
CMD ["/build/recognizer"]
