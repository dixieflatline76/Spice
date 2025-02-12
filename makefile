# --- Extract version needed for EULA hash ---
VERSION := $(shell sh -c "cat version.txt" 2> /dev/null || cmd /c "type version.txt")

# --- Build targets ---
build-gui:
	set GOOS=windows&& set GOARCH=amd64&& go build -o bin/Spice.exe -ldflags "-H=windowsgui -X main.version=$(VERSION)" ./cmd/spice

build-console:
	set GOOS=windows&& set GOARCH=amd64&& go build -o bin/Spice-console.exe -ldflags "-X main.version=$(VERSION)" ./cmd/spice

build-linux:
	GOOS=linux GOARCH=amd64 go build -o bin/Spice -ldflags "-X main.version=$(VERSION)" ./cmd/spice

build-darwin-amd64:
	GOOS=darwin GOARCH=amd64 go build -o bin/Spice_darwin_amd64 -ldflags "-X main.version=$(VERSION)" ./cmd/spice

build-darwin-arm64:
	GOOS=darwin GOARCH=arm64 go build -o bin/Spice_darwin_arm64 -ldflags "-X main.version=$(VERSION)" ./cmd/spice


# --- Other targets ---
lint:
	gofmt -w .
	golint ./...
	staticcheck ./...

test:
	go test ./...	

update-minor-deps:
	go get -u ./...
	go mod tidy

update-major-deps:
	go get -u=patch ./... 
	go mod tidy

all: update-minor-deps lint test build-gui build-console

# --- Clean target (cross-platform) ---
clean:
ifeq ($(OS),Windows_NT)
	del /s /q bin\*
else
	$(RM) -r bin
endif

# Build rule for the version_bump utility
build-version-bump:
	@echo "Building version_bump utility..."
	@if not exist bin\util mkdir bin\util
	go build -o bin/util/version_bump ./cmd/util/version_bump.go

# Bump rules, now depending on build-version-bump
bump-patch: build-version-bump
	@echo "Bumping patch version..."
	./bin/util/version_bump patch

bump-minor: build-version-bump
	@echo "Bumping minor version..."
	./bin/util/version_bump minor

bump-major: build-version-bump
	@echo "Bumping major version..."
	./bin/util/version_bump major

.PHONY: all build-gui build-console build-linux build-darwin-amd64 build-darwin-arm64 lint test update-minor-deps update-major-deps clean bump-patch bump-minor bump-major build-version-bump