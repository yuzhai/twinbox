BIN := bin
GO  := go

.PHONY: build test vet fmt sandbox-test attack verify clean

build:
	$(GO) build -trimpath -ldflags "-s -w" -o $(BIN)/twinbox ./cmd/twinbox
	$(GO) build -trimpath -ldflags "-s -w" -o $(BIN)/twinbox-shim ./cmd/twinbox-shim

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

# 沙箱行为与红队用例（需要 bwrap，仅在本地/专用机器上跑）
sandbox-test:
	./scripts/sandbox-test.sh

attack:
	./scripts/attack.sh

verify: vet test build attack
	@echo "verify: 全部通过"

clean:
	rm -rf $(BIN)
