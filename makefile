VERSION := $(shell sh -c "cat version.txt" 2> /dev/null || cmd /c "type version.txt")

build-gui:
	go build -o bin/spice.exe -ldflags "-H=windowsgui -X config.AppVersion=$(VERSION)" ./cmd/spice

build-console:
	go build -o bin/spice-console.exe -ldflags "-X config.AppVersion=$(VERSION)" ./cmd/spice

lint:
	gofmt -w .
	golint ./...
	staticcheck ./...

test:
	go test ./...	

update-minor-deps:
	go get -u=patch ./...
	go mod tidy

update-major-deps:
	go get -u ./...
	go mod tidy

all: update-minor-deps lint test build-gui build-console

clean:
	del bin\spice.exe bin\spice-service.exe
