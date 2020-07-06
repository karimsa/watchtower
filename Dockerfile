## Builder
FROM golang:1.13-alpine AS builder

RUN apk add --no-cache make build-base

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY rebuild.go rebuild.go
RUN go vet ./... \
		&& go build -o rebuild_container rebuild.go

## Final image
FROM alpine

RUN apk add --no-cache bash jq docker-cli

COPY --from=builder /app/rebuild_container /usr/local/bin/rebuild_container
COPY watchtower.sh /usr/local/bin/watchtower
RUN chmod +x /usr/local/bin/watchtower

ENTRYPOINT ["/usr/local/bin/watchtower"]
