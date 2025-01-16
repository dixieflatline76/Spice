# --- Determine OS-specific commands and settings ---
ifeq ($(OS),Windows_NT) # Windows
	RM := del
	GO_BUILD_ARGS := -ldflags "-H=windowsgui -X config.AppVersion=$(VERSION)"
else # macOS or Linux
	RM := rm -f
	GO_BUILD_ARGS := -ldflags "-X config.AppVersion=$(VERSION)"
endif

# --- Build targets ---
build-gui:
	go build $(GO_BUILD_ARGS) -o bin/spice ./cmd/spice

build-console:
	go build -o bin/spice-console -ldflags "-X config.AppVersion=$(VERSION)" ./cmd/spice

# --- Other targets ---
lint:
	gofmt -w .
	golint ./...
	staticcheck ./...

test:
	go test ./...	

update-patch-deps:
	go get -u=patch ./...
	go mod tidy

update-minor-deps:
	go get -u=minor ./...
	go mod tidy

update-major-deps:
	go get -u ./... 
	go mod tidy

all: update-patch-deps update-minor-deps lint test build-gui build-console

# --- Clean target (cross-platform) ---
clean:
	-$(RM) bin/spice.exe bin/spice-service.exe bin/spice bin/spice-console.exe
	-$(RM) bin/spice-console

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