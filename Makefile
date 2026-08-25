BINARY := agentdomains
PKG := ./cmd/agentdomains

# VERSION is stamped into the binary (`agentdomains version`). Releases pass the
# tag in; a plain `make build` leaves the source default of "dev".
VERSION ?= dev
LDFLAGS := -s -w -X main.version=$(VERSION)

# Platforms we ship prebuilt binaries for.
PLATFORMS := darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64

.PHONY: build install test clean dist

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(PKG)

install:
	go install -ldflags "$(LDFLAGS)" $(PKG)

test:
	go vet ./...
	go test ./...

# Cross-compile every release artifact into dist/: one archive per platform
# (.tar.gz, or .zip for Windows) plus a SHA256SUMS file covering them all.
# Usage: make dist VERSION=v0.1.1
dist: clean
	@mkdir -p dist
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		out=$(BINARY); [ "$$os" = "windows" ] && out=$(BINARY).exe; \
		echo "building $$os/$$arch"; \
		rm -rf dist/stage && mkdir -p dist/stage; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 \
			go build -trimpath -ldflags "$(LDFLAGS)" -o dist/stage/$$out $(PKG) || exit 1; \
		cp README.md LICENSE dist/stage/; \
		base=$(BINARY)_$(VERSION)_$${os}_$${arch}; \
		if [ "$$os" = "windows" ]; then \
			(cd dist/stage && zip -q ../$$base.zip *); \
		else \
			tar -czf dist/$$base.tar.gz -C dist/stage .; \
		fi; \
	done
	@rm -rf dist/stage
	@cd dist && shasum -a 256 * > SHA256SUMS
	@echo "--- dist/ ---" && ls -l dist

clean:
	rm -rf $(BINARY) dist
