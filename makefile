# --- Extract version needed for EULA hash ---
VERSION := $(shell sh -c "cat version.txt" 2> /dev/null || cmd /c "type version.txt")

# --- Build flags ---
LDFLAGS_SECRETS := $(shell go run cmd/util/load_secrets/main.go)
LDFLAGS_COMMON := -X main.version=$(VERSION) $(LDFLAGS_SECRETS)

# --- Extension Utils ---
sync-extension:
	go run cmd/util/sync_regex/main.go

pack-extension:
	go run cmd/util/pack_extension/main.go

# --- Code Generation ---
gen-i18n:
	go run cmd/util/gen_i18n/main.go

generate: gen-i18n
	go generate ./...

# --- Build targets ---
build-extension:
	@echo "Checking if we should build Safari Extension..."
ifneq ($(SKIP_EXTENSION_BUILD),)
	@echo "SKIP_EXTENSION_BUILD is set. Skipping xcodebuild (assuming CI or manual skip)..."
	@echo "Ensuring artifact exists..."
	@if [ ! -d "Spice Wallpaper Manager Extension.app" ]; then echo "Warning: Extension app not found. Building without it."; fi
else
	@echo "Building Safari Extension..."
	xcodebuild -project "Spice Wallpaper Manager Extension/Spice Wallpaper Manager Extension.xcodeproj" \
		-scheme "Spice Wallpaper Manager Extension (macOS)" \
		-configuration Release \
		-derivedDataPath "build/extension" \
		clean build
	@echo "Moving built extension to root for packaging..."
	rm -rf "Spice Wallpaper Manager Extension.app"
	cp -R "build/extension/Build/Products/Release/Spice Wallpaper Manager Extension.app" .
endif

build-win-amd64:
	set GOOS=windows&& set GOARCH=amd64&& go build -tags release -o bin/Spice.exe -ldflags "-H=windowsgui $(LDFLAGS_COMMON)" ./cmd/spice

build-win-console-amd64:
	set GOOS=windows&& set GOARCH=amd64&& go build -tags release -o bin/Spice-console.exe -ldflags "$(LDFLAGS_COMMON)" ./cmd/spice

build-win-arm64:
	set GOOS=windows&& set GOARCH=arm64&& go build -tags release -o bin/Spice-arm64.exe -ldflags "-H=windowsgui $(LDFLAGS_COMMON)" ./cmd/spice

build-linux-amd64:
	GOOS=linux GOARCH=amd64 go build -tags release -o bin/Spice-amd64 -ldflags "$(LDFLAGS_COMMON)" ./cmd/spice

build-linux-arm64:
	GOOS=linux GOARCH=arm64 go build -tags release -o bin/Spice-arm64 -ldflags "$(LDFLAGS_COMMON)" ./cmd/spice

build-darwin-amd64: build-extension
	@echo "Building Go executable for darwin/amd64..."
	GOOS=darwin GOARCH=amd64 go build -tags release -o bin/Spice-darwin-amd64 -ldflags "$(LDFLAGS_COMMON)" ./cmd/spice

	@echo "Packaging Spice.app..."
	fyne package -os darwin --executable ./bin/Spice-darwin-amd64 -icon asset/icons/tray.png -name Spice -appID com.dixieflatline76.spice

	@echo "Modifying Info.plist to set LSUIElement=true..."
	plutil -insert LSUIElement -bool true Spice.app/Contents/Info.plist

	@echo "Moving final Spice.app to ./bin/..."
	rm -rf ./bin/Spice.app && mv Spice.app ./bin/

build-darwin-arm64: build-extension
	@echo "Building Go executable for darwin/arm64..."
	GOOS=darwin GOARCH=arm64 go build -tags release -o bin/Spice-darwin-arm64 -ldflags "$(LDFLAGS_COMMON)" ./cmd/spice

	@echo "Packaging Spice.app..."
	fyne package -os darwin --executable ./bin/Spice-darwin-arm64 -icon asset/icons/tray.png -name Spice -appID com.dixieflatline76.spice

	@echo "Modifying Info.plist to set LSUIElement=true..."
	plutil -insert LSUIElement -bool true Spice.app/Contents/Info.plist

ifneq ($(SIGNING_IDENTITY),)
	@echo "Signing image processor binary..."
	codesign --force --options=runtime --sign "$(SIGNING_IDENTITY)" --timestamp bin/Spice-darwin-arm64
	@echo "Signing the application bundle..."
	codesign --force --options=runtime --sign "$(SIGNING_IDENTITY)" --timestamp Spice.app
	@echo "Verifying application bundle signature..."
	codesign -dv --verbose=4 Spice.app
else
	@echo "No SIGNING_IDENTITY set, skipping code signing."
endif

	@echo "Creating styled DMG..."
	mkdir -p dist/dmg-staging
	rm -rf dist/dmg-staging/*

	# Copy Main App to Staging
	cp -R "Spice.app" dist/dmg-staging/

	# Copy and Sign Extension in Staging if it exists
ifneq ($(SIGNING_IDENTITY),)
	if [ -d "Spice Wallpaper Manager Extension.app" ]; then \
		echo "Found Safari Extension, verifying paths..."; \
		ls -la "Spice Wallpaper Manager Extension.app/Contents/"; \
		cp -R "Spice Wallpaper Manager Extension.app" dist/dmg-staging/; \
		if [ -d "dist/dmg-staging/Spice Wallpaper Manager Extension.app/Contents/Frameworks" ]; then \
			echo "Signing Extension Frameworks..."; \
			codesign --force --options=runtime --sign "$(SIGNING_IDENTITY)" --timestamp "dist/dmg-staging/Spice Wallpaper Manager Extension.app/Contents/Frameworks/"*.dylib; \
		fi; \
		echo "Signing Extension Appex..."; \
		codesign --force --options=runtime --entitlements "Spice Wallpaper Manager Extension/macOS (Extension)/Spice Wallpaper Manager Extension.entitlements" --sign "$(SIGNING_IDENTITY)" --timestamp "dist/dmg-staging/Spice Wallpaper Manager Extension.app/Contents/PlugIns/Spice Wallpaper Manager Extension Extension.appex"; \
		echo "Signing Extension Wrapper..."; \
		codesign --force --options=runtime --entitlements "Spice Wallpaper Manager Extension/macOS (App)/Spice Wallpaper Manager Extension.entitlements" --sign "$(SIGNING_IDENTITY)" --timestamp "dist/dmg-staging/Spice Wallpaper Manager Extension.app"; \
	fi
else
	if [ -d "Spice Wallpaper Manager Extension.app" ]; then \
		echo "Found Safari Extension but no SIGNING_IDENTITY, skipping extension signing..."; \
		cp -R "Spice Wallpaper Manager Extension.app" dist/dmg-staging/; \
	fi
endif
	
	rm -f "dist/Spice-$(VERSION)-arm64.dmg"
	
	@echo "Constructing create-dmg command..."
	CMD="create-dmg --volname 'Spice Installer' --background 'images/Spice-dmg-bg.png' --window-pos 200 120 --window-size 640 480 --icon-size 130 --icon 'Spice.app' 175 200 --hide-extension 'Spice.app' --app-drop-link 465 200"; \
	if [ -d "dist/dmg-staging/Spice Wallpaper Manager Extension.app" ]; then \
		CMD="$$CMD --icon 'Spice Wallpaper Manager Extension.app' 175 350"; \
	fi; \
	CMD="$$CMD 'dist/Spice-$(VERSION)-arm64.dmg' 'dist/dmg-staging/'"; \
	eval $$CMD

	@echo "Moving final Spice.app to ./bin/..."
	rm -rf ./bin/Spice.app && mv Spice.app ./bin/

build-darwin-appstore-arm64: build-extension
	@echo "Building Go executable for macOS App Store (arm64)..."
	GOOS=darwin GOARCH=arm64 go build -tags release -o bin/Spice-darwin-appstore-arm64 -ldflags "$(LDFLAGS_COMMON)" ./cmd/spice

	@echo "Packaging signed .pkg for App Store..."
	@if [ -f "embedded.provisionprofile" ]; then \
		echo "Using provided provisioning profile..."; \
		fyne release -os darwin -category utilities -executable ./bin/Spice-darwin-appstore-arm64 -icon asset/icons/tray.png -name Spice -appID com.dixieflatline76.spice -profile embedded.provisionprofile; \
	else \
		echo "No provisioning profile found, proceeding without it..."; \
		fyne release -os darwin -category utilities -executable ./bin/Spice-darwin-appstore-arm64 -icon asset/icons/tray.png -name Spice -appID com.dixieflatline76.spice; \
	fi

	@echo "Moving final Spice.pkg to ./dist/..."
	mkdir -p dist
	mv Spice.pkg dist/Spice-$(VERSION)-macos-arm64-AppStore.pkg

# --- Development build targets ---
build-win-amd64-dev:
	set GOOS=windows&& set GOARCH=amd64&& go build -o bin/Spice.exe -ldflags "-H=windowsgui $(LDFLAGS_COMMON)" ./cmd/spice

build-win-console-amd64-dev:
	set GOOS=windows&& set GOARCH=amd64&& go build -o bin/Spice-console.exe -ldflags "$(LDFLAGS_COMMON)" ./cmd/spice

build-linux-amd64-dev:
	GOOS=linux GOARCH=amd64 go build -o bin/Spice-amd64 -ldflags "$(LDFLAGS_COMMON)" ./cmd/spice

build-linux-arm64-dev:
	GOOS=linux GOARCH=arm64 go build -o bin/Spice-arm64 -ldflags "$(LDFLAGS_COMMON)" ./cmd/spice

build-darwin-amd64-dev:
	@echo "Building Go executable for darwin/amd64..."
	GOOS=darwin GOARCH=amd64 go build -o bin/Spice-darwin-amd64 -ldflags "$(LDFLAGS_COMMON)" ./cmd/spice

	@echo "Packaging Spice.app..."
	fyne package -os darwin --executable ./bin/Spice-darwin-amd64 -icon asset/icons/tray.png -name Spice -appID com.dixieflatline76.spice

	@echo "Modifying Info.plist to set LSUIElement=true..."
	plutil -insert LSUIElement -bool true Spice.app/Contents/Info.plist

	@echo "Moving final Spice.app to ./bin/..."
	rm -rf ./bin/Spice.app && mv Spice.app ./bin/

build-darwin-arm64-dev:
	@echo "Building Go executable for darwin/arm64..."
	GOOS=darwin GOARCH=arm64 go build -o bin/Spice-darwin-arm64 -ldflags "$(LDFLAGS_COMMON)" ./cmd/spice

	@echo "Packaging Spice.app..."
	fyne package -os darwin --executable ./bin/Spice-darwin-arm64 -icon asset/icons/tray.png -name Spice -appID com.dixieflatline76.spice

	@echo "Modifying Info.plist to set LSUIElement=true..."
	plutil -insert LSUIElement -bool true Spice.app/Contents/Info.plist

	@echo "Moving final Spice.app to ./bin/..."
	rm -rf ./bin/Spice.app && mv Spice.app ./bin/

# --- Other targets ---
lint:
	gofmt -w .
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run --timeout=10m ./...

security:
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run --timeout=10m --enable gosec --tests=false ./...

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
win-amd64: generate update-patch-deps lint test build-win-amd64 build-win-console-amd64

win-amd64-dev: generate update-patch-deps lint test build-win-amd64-dev build-win-console-amd64-dev

linux-amd64: generate update-patch-deps lint test build-linux-amd64

linux-amd64-dev: generate update-patch-deps lint test build-linux-amd64-dev

darwin-amd64: generate update-patch-deps lint test build-darwin-amd64

darwin-amd64-dev: generate update-patch-deps lint test build-darwin-amd64-dev

darwin-arm64: generate update-patch-deps lint test build-darwin-arm64

darwin-arm64-dev: generate update-patch-deps lint test build-darwin-arm64-dev

# --- Clean target (cross-platform) ---
clean:
ifeq ($(OS),Windows_NT)
	if exist bin rmdir /s /q bin
	if exist coverage* del /q coverage*
	if exist *.out del /q *.out
	if exist *.html del /q *.html
else
	$(RM) -r bin coverage* *.out *.html
endif
	go clean

# Combined coverage target
coverage:
	go test -v -coverprofile=coverage.out ./...
	@echo "Filtering coverage report..."
	@grep -v "cmd/util" coverage.out > coverage.filtered.out
	@move /Y coverage.filtered.out coverage.out
	go tool cover -html=coverage.out -o coverage.html
	go tool cover -func=coverage.out > docs/coverage_report.md
	@echo "Coverage report generated: coverage.html & docs/coverage_report.md"
	@echo "Summary:"
	@go tool cover -func=coverage.out

# Legacy aliases
test-coverage: coverage
coverage-report: coverage

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

.PHONY: build-win-amd64 build-win-console-amd64 build-win-arm64 build-linux-amd64 build-darwin-amd64 build-darwin-arm64 build-win-amd64-dev build-win-console-amd64-dev build-linux-amd64-dev lint test update-patch-deps update-minor-deps list-updates win-amd64 win-amd64-dev linux-amd64 linux-amd64-dev darwin-amd64 darwin-amd64-dev darwin-arm64 darwin-arm64-dev clean build-version-bump bump-patch bump-minor bump-major notarize-mac-arm64 coverage
