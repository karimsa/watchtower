FROM golang:1.13-alpine AS builder

RUN apk add make build-base

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY main.go main.go
RUN go vet ./... \
		&& go build -o watchtower main.go

FROM alpine
COPY --from=builder /app/watchtower /usr/local/bin/watchtower
CMD /usr/local/bin/watchtower
