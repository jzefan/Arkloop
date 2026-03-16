SERVICES := api gateway worker sandbox
SHARED   := src/services/shared

# Default: cloud build (no extra tags required)
.PHONY: build build-cloud build-desktop build-desktop-sidecar build-desktop-sidecar-all build-shared test test-cloud test-desktop lint setup-vm-dev sign-sidecar-dev test-vm-integration

build: build-cloud

## build-cloud: Build all services for cloud deployment (default)
build-cloud:
	@echo "==> Building cloud services..."
	cd src/services/api     && go build ./...
	cd src/services/gateway && go build ./...
	cd src/services/worker  && go build ./...
	cd src/services/sandbox && go build ./...

## build-desktop-sidecar: Cross-compile desktop sidecar for current platform
build-desktop-sidecar:
	@echo "==> Building desktop sidecar (current platform)..."
	node src/apps/desktop/scripts/build-sidecar.mjs

## build-desktop-sidecar-all: Cross-compile desktop sidecar for all platforms
build-desktop-sidecar-all:
	@echo "==> Building desktop sidecar (all platforms)..."
	node src/apps/desktop/scripts/build-sidecar.mjs --all

## build-desktop: Build worker for local Desktop mode (excludes Redis, PostgreSQL, S3 SDK)
# Note: api service is cloud-only in Phase 2; desktop support is planned for a later phase.
build-desktop:
	@echo "==> Building desktop services (tags: desktop)..."
	cd src/services/worker && go build -tags desktop ./cmd/...

## test-cloud: Run tests for cloud mode (default, no extra tags)
test-cloud:
	@echo "==> Running cloud tests..."
	cd $(SHARED)            && go test ./...
	cd src/services/api     && go test ./...
	cd src/services/worker  && go test ./...

## test-desktop: Run tests for desktop mode (tags: desktop)
# Only packages with no cloud-only (pgx/redis/S3) dependencies are tested.
# api is excluded (cloud-only in Phase 2); worker tests limited to portable packages.
WORKER_DESKTOP_PKGS := \
  ./internal/agent/... \
  ./internal/consumer/... \
  ./internal/llm/... \
  ./internal/memory/... \
  ./internal/queue/... \
  ./internal/runtime/... \
  ./internal/tools/... \
  ./internal/webhook/...

test-desktop:
	@echo "==> Running desktop tests (tags: desktop)..."
	cd $(SHARED)           && go test -tags desktop ./...
	cd src/services/worker && go test -tags desktop $(WORKER_DESKTOP_PKGS)

test: test-cloud

## lint: Run go vet on all services
lint:
	@echo "==> Linting cloud build..."
	cd $(SHARED)            && go vet ./...
	cd src/services/api     && go vet ./...
	cd src/services/gateway && go vet ./...
	cd src/services/worker  && go vet ./...
	@echo "==> Linting desktop build..."
	cd $(SHARED)           && go vet -tags desktop ./...
	cd src/services/worker && go vet -tags desktop ./cmd/...

## setup-vm-dev: Install dev VM assets from /tmp/vz-test/ into ~/.arkloop/vm/ (symlinks, no copy)
# rootfs: uses python3.12.ext4 (contains sandbox-agent + init script)
# kernel: vmlinux
# initrd:  initramfs-custom.gz (loads vsock modules before switch_root)
setup-vm-dev:
	@echo "==> Setting up VM dev assets from /tmp/vz-test/ ..."
	@mkdir -p ~/.arkloop/vm
	@ln -sfn "/tmp/vz-test/vmlinux"                    "$$HOME/.arkloop/vm/vmlinux"             && echo "  linked: vmlinux"
	@ln -sfn "/tmp/vz-test/rootfs-full/python3.12.ext4" "$$HOME/.arkloop/vm/rootfs.ext4"        && echo "  linked: rootfs.ext4 -> rootfs-full/python3.12.ext4"
	@ln -sfn "/tmp/vz-test/initramfs-custom.gz"         "$$HOME/.arkloop/vm/initramfs-custom.gz" && echo "  linked: initramfs-custom.gz"
	@echo '{"version":"dev-local","source":"/tmp/vz-test/rootfs-full/python3.12.ext4","linkedAt":"'$$(date -u +%Y-%m-%dT%H:%M:%SZ)'"}' \
		> ~/.arkloop/vm/vm.version.json
	@echo "==> Done. VM assets available at ~/.arkloop/vm/"
	@echo "    Open Settings > Isolation to switch to Apple VM mode."

## sign-sidecar-dev: Ad-hoc sign dev sidecar with com.apple.security.virtualization entitlement (macOS only)
SIDECAR_DEV_BIN := src/services/desktop/bin/desktop
ENTITLEMENTS_PLIST := /tmp/vz-test/entitlements.plist

sign-sidecar-dev:
	@echo "==> Signing dev sidecar with virtualization entitlement..."
	@if [ ! -f "$(SIDECAR_DEV_BIN)" ]; then \
		echo "  ERROR: $(SIDECAR_DEV_BIN) not found. Run build-desktop-sidecar first."; exit 1; \
	fi
	@if [ ! -f "$(ENTITLEMENTS_PLIST)" ]; then \
		echo "  ERROR: $(ENTITLEMENTS_PLIST) not found."; exit 1; \
	fi
	codesign --force --entitlements $(ENTITLEMENTS_PLIST) --sign - $(SIDECAR_DEV_BIN)
	@echo "==> Sidecar signed. Run 'codesign -d --entitlements :- $(SIDECAR_DEV_BIN)' to verify."

## test-vm-integration: Run VZ integration tests (requires /tmp/vz-test/ assets)
test-vm-integration:
	@echo "==> Running VZ integration tests ..."
	VZ_INTEGRATION=1 go test -v -timeout 300s -tags darwin \
		./src/services/sandbox/internal/vz/ -run TestIntegration

help:
	@grep -E '^##' Makefile | sed 's/## /  /'
