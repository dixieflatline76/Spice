# --- Extract version needed for EULA hash ---
VERSION := $(shell sh -c "cat version.txt" 2> /dev/null || cmd /c "type version.txt")

# --- Build targets ---
build-win-amd64:
	set GOOS=windows&& set GOARCH=amd64&& go build -tags release -o bin/Spice.exe -ldflags "-H=windowsgui -X main.version=$(VERSION)" ./cmd/spice

build-win-console-amd64:
	set GOOS=windows&& set GOARCH=amd64&& go build -tags release -o bin/Spice-console.exe -ldflags "-X main.version=$(VERSION)" ./cmd/spice

build-win-arm64:
	set GOOS=windows&& set GOARCH=arm64&& go build -tags release -o bin/Spice-arm64.exe -ldflags "-H=windowsgui -X main.version=$(VERSION)" ./cmd/spice

build-linux-amd64:
	GOOS=linux GOARCH=amd64 go build -tags release -o bin/Spice-amd64 -ldflags "-X main.version=$(VERSION)" ./cmd/spice

build-darwin-amd64:
	@echo "Building Go executable for darwin/amd64..."
	GOOS=darwin GOARCH=amd64 go build -tags release -o bin/Spice-darwin-amd64 -ldflags "-X main.version=$(VERSION)" ./cmd/spice

	@echo "Packaging Spice.app..."
	fyne package -os darwin --executable ./bin/Spice-darwin-amd64 -icon asset/icons/tray.png -name Spice

	@echo "Modifying Info.plist to set LSUIElement=true..."
	plutil -insert LSUIElement -bool true Spice.app/Contents/Info.plist

	@echo "Moving final Spice.app to ./bin/..."
	rm -rf ./bin/Spice.app && mv Spice.app ./bin/

build-darwin-arm64:
	@echo "Building Go executable for darwin/arm64..."
	GOOS=darwin GOARCH=arm64 go build -tags release -o bin/Spice-darwin-arm64 -ldflags "-X main.version=$(VERSION)" ./cmd/spice

	@echo "Packaging Spice.app..."
	fyne package -os darwin --executable ./bin/Spice-darwin-arm64 -icon asset/icons/tray.png -name Spice

	@echo "Modifying Info.plist to set LSUIElement=true..."
	plutil -insert LSUIElement -bool true Spice.app/Contents/Info.plist

	@echo "Signing the application bundle..."
	# SIGNING_IDENTITY will be provided by the GitHub Actions workflow
	codesign --force --deep --options=runtime --sign "${SIGNING_IDENTITY}" --timestamp Spice.app

	@echo "Creating styled DMG..."
	mkdir -p dist

	create-dmg \
		--volname "Spice Installer" \
		--background "images/Spice-dmg-bg.png" \
		--window-pos 200 120 \
		--window-size 640 480 \
		--icon-size 130 \
		--icon "Spice.app" 175 200 \
		--hide-extension "Spice.app" \
		--app-drop-link 465 200 \
		"dist/Spice-$(VERSION)-arm64.dmg" \
		"Spice.app/"

	@echo "Moving final Spice.app to ./bin/..."
	rm -rf ./bin/Spice.app && mv Spice.app ./bin/

# --- Development build targets ---
build-win-amd64-dev:
	set GOOS=windows&& set GOARCH=amd64&& go build -o bin/Spice.exe -ldflags "-H=windowsgui -X main.version=$(VERSION)" ./cmd/spice

build-win-console-amd64-dev:
	set GOOS=windows&& set GOARCH=amd64&& go build -o bin/Spice-console.exe -ldflags "-X main.version=$(VERSION)" ./cmd/spice

build-linux-amd64-dev:
	GOOS=linux GOARCH=amd64 go build -o bin/Spice-amd64 -ldflags "-X main.version=$(VERSION)" ./cmd/spice

build-darwin-amd64-dev:
	@echo "Building Go executable for darwin/amd64..."
	GOOS=darwin GOARCH=amd64 go build -o bin/Spice-darwin-amd64 -ldflags "-X main.version=$(VERSION)" ./cmd/spice

	@echo "Packaging Spice.app..."
	fyne package -os darwin --executable ./bin/Spice-darwin-amd64 -icon asset/icons/tray.png -name Spice

	@echo "Modifying Info.plist to set LSUIElement=true..."
	plutil -insert LSUIElement -bool true Spice.app/Contents/Info.plist

	@echo "Moving final Spice.app to ./bin/..."
	rm -rf ./bin/Spice.app && mv Spice.app ./bin/

build-darwin-arm64-dev:
	@echo "Building Go executable for darwin/arm64..."
	GOOS=darwin GOARCH=arm64 go build -o bin/Spice-darwin-arm64 -ldflags "-X main.version=$(VERSION)" ./cmd/spice

	@echo "Packaging Spice.app..."
	fyne package -os darwin --executable ./bin/Spice-darwin-arm64 -icon asset/icons/tray.png -name Spice

	@echo "Modifying Info.plist to set LSUIElement=true..."
	plutil -insert LSUIElement -bool true Spice.app/Contents/Info.plist

	@echo "Moving final Spice.app to ./bin/..."
	rm -rf ./bin/Spice.app && mv Spice.app ./bin/

# --- Other targets ---
lint:
	gofmt -w .
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run --timeout=5m ./...

test:
	go test ./...

test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

update-patch-deps:
	go get -u=patch ./... 
	go mod tidy

update-minor-deps:
	go get -u ./...
	go mod tidy

list-updates:
	@echo "Checking for all available updates (including major)..."
	go list -m -u all
	@echo "Review the list above. Update major versions manually using 'go get module/vX@latest'."

# --- Main build targets ---
win-amd64: update-patch-deps lint test build-win-amd64 build-win-console-amd64

win-amd64-dev: update-patch-deps lint test build-win-amd64-dev build-win-console-amd64-dev

linux-amd64: update-patch-deps lint test build-linux-amd64

linux-amd64-dev: update-patch-deps lint test build-linux-amd64-dev

darwin-amd64: update-patch-deps lint test build-darwin-amd64

darwin-amd64-dev: update-patch-deps lint test build-darwin-amd64-dev

darwin-arm64: update-patch-deps lint test build-darwin-arm64

darwin-arm64-dev: update-patch-deps lint test build-darwin-arm64-dev

# --- Clean target (cross-platform) ---
clean:
ifeq ($(OS),Windows_NT)
	del /s /q bin\*
else
	$(RM) -r bin
endif
	go clean

# Define the command based on OS
# Default for Linux/macOS
CREATE_DIR_CMD := mkdir -p bin/util

# Override for Windows
ifeq ($(OS),Windows_NT)
  # Use cmd /c for native Windows commands. Use backslashes inside the cmd string for safety.
  CREATE_DIR_CMD := cmd /c "(if not exist bin mkdir bin) && (if not exist bin\util mkdir bin\util)"
endif

# Build rule for the version_bump utility
bin/util/version_bump: ./cmd/util/version_bump.go
	@echo "Building version_bump utility..."
	$(CREATE_DIR_CMD)
	go build -o $@ $<

build-version-bump: bin/util/version_bump
	@echo "version_bump utility is up-to-date."

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

notarize-mac-arm64:
	@echo "Uploading DMG for notarization..."
	xcrun notarytool submit "dist/Spice-$(VERSION)-arm64.dmg" --keychain-profile "AC_PASSWORD" --wait
	@echo "Stapling notarization ticket to DMG..."
	xcrun stapler staple "dist/Spice-$(VERSION)-arm64.dmg"

.PHONY: build-win-amd64 build-win-console-amd64 build-win-arm64 build-linux-amd64 build-darwin-amd64 build-darwin-arm64 build-win-amd64-dev build-win-console-amd64-dev build-linux-amd64-dev lint test update-patch-deps update-minor-deps list-updates win-amd64 win-amd64-dev linux-amd64 linux-amd64-dev darwin-amd64 darwin-amd64-dev darwin-arm64 darwin-arm64-dev clean build-version-bump bump-patch bump-minor bump-major notarize-mac-arm64
