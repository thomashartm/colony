# schema = 1
GOLANGCI_LINT ?= golangci-lint
GORELEASER ?= goreleaser

.PHONY: build test vet lint check cross-build snapshot

build:
	CGO_ENABLED=0 go build -trimpath -o bin/motley ./cmd/motley
	ln -sf motley bin/mtly

test:
	bash scripts/test-install.sh
	python3 scripts/test-uninstall.py
	node --test integrations/opencode/plugin.test.mjs
	go test ./...

vet:
	go vet ./...

lint:
	$(GOLANGCI_LINT) run

check: test vet lint cross-build

cross-build:
	@set -e; for os in darwin linux; do \
		for arch in amd64 arm64; do \
			CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -o bin/motley-$$os-$$arch ./cmd/motley; \
		done; \
	done

snapshot:
	$(GORELEASER) release --snapshot --clean
