FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY src/services/gateway/ .
RUN go build -ldflags="-s -w" -o /gateway ./cmd/gateway

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata wget
COPY --from=builder /gateway /usr/local/bin/gateway
ENTRYPOINT ["gateway"]
