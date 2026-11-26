VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build web-build go-build test test-go test-web dev-api dev-web clean

build: web-build go-build

web-build:
	cd web && npm ci && npm run build
	rm -rf internal/web/dist
	mkdir -p internal/web/dist
	cp -r web/dist/. internal/web/dist/
	touch internal/web/dist/.gitkeep

go-build:
	mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o bin/spool ./cmd/spool

test: test-go test-web

test-go:
	go test ./...

test-web:
	cd web && npm test

dev-api:
	go run ./cmd/spool -config config.yaml

dev-web:
	cd web && npm run dev

clean:
	rm -rf bin web/dist
	rm -rf internal/web/dist
	mkdir -p internal/web/dist
	touch internal/web/dist/.gitkeep
