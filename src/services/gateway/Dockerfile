FROM golang:1.26-alpine AS builder
WORKDIR /workspace
COPY src/services/gateway src/services/gateway
COPY src/services/shared src/services/shared
RUN printf 'go 1.26.0\nuse (\n\t./src/services/gateway\n\t./src/services/shared\n)\n' > go.work
RUN go build -ldflags="-s -w" -o /gateway ./src/services/gateway/cmd/gateway

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata wget
COPY --from=builder /gateway /usr/local/bin/gateway
ENTRYPOINT ["gateway"]
