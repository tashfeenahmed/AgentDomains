BINARY := agentdomains
PKG := ./cmd/agentdomains

.PHONY: build install test clean dist

build:
	go build -o $(BINARY) $(PKG)

install:
	go install $(PKG)

test:
	go vet ./...
	go test ./...

# Cross-compile release binaries.
dist:
	mkdir -p dist
	GOOS=darwin  GOARCH=arm64 go build -o dist/$(BINARY)-darwin-arm64  $(PKG)
	GOOS=darwin  GOARCH=amd64 go build -o dist/$(BINARY)-darwin-amd64  $(PKG)
	GOOS=linux   GOARCH=arm64 go build -o dist/$(BINARY)-linux-arm64   $(PKG)
	GOOS=linux   GOARCH=amd64 go build -o dist/$(BINARY)-linux-amd64   $(PKG)
	GOOS=windows GOARCH=amd64 go build -o dist/$(BINARY)-windows-amd64.exe $(PKG)

clean:
	rm -rf $(BINARY) dist
