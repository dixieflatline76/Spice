# --- Extract version needed for EULA hash ---
VERSION := $(shell sh -c "cat version.txt" 2> /dev/null || cmd /c "type version.txt")

# --- Build targets ---
build-gui:
	go build -o bin/spice.exe -ldflags "-H=windowsgui -X config.AppVersion=$(VERSION)" ./cmd/spice

build-console:
	go build -o bin/spice-console.exe -ldflags "-X config.AppVersion=$(VERSION)" ./cmd/spice

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
	del /s /q bin\*

.PHONY: bump-patch bump-minor bump-major build-version-bump

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