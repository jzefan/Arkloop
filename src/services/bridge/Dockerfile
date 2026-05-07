FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY src/services/bridge/go.mod src/services/bridge/go.sum src/services/bridge/
COPY src/services/shared/go.mod src/services/shared/go.sum src/services/shared/
RUN printf 'go 1.26.0\nuse (\n\t./src/services/bridge\n\t./src/services/shared\n)\n' > go.work
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY src/services/bridge/ src/services/bridge/
COPY src/services/shared/ src/services/shared/
COPY install/ install/
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -ldflags="-s -w" -o /bridge ./src/services/bridge/cmd/bridge

FROM alpine:3.21
RUN apk add --no-cache docker-cli docker-cli-compose
COPY --from=builder /bridge /usr/local/bin/bridge
COPY install/modules.yaml /etc/arkloop/modules.yaml
ENTRYPOINT ["bridge"]
