BIN := bin
GO  := go

.PHONY: build test vet fmt sandbox-test attack e2e-real verify clean

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

# 真实 agent + 真实网关；会消耗模型额度
e2e-real: build
	./scripts/e2e-real.sh

verify: vet test build attack
	@echo "verify: 全部通过"

clean:
	rm -rf $(BIN)
