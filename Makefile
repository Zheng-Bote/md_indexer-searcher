
build:
	go build -o bin/md-indexer ./cmd/indexer

search:
	go build -o bin/md-search ./cmd/md-search

api:
	go build -o bin/md-api ./cmd/md-api

api-run: api
	./bin/md-api -port 8081

ai:
	go build -o bin/md-ai ./cmd/md-ai

test:
	go test -v ./...


DESKTOP_DIR := apps/desktop
TARGET_TRIPLE = $(shell rustc -vV | sed -n 's/^host: //p')

desktop-sidecars: build api
	mkdir -p $(DESKTOP_DIR)/src-tauri/binaries
	cp bin/md-indexer $(DESKTOP_DIR)/src-tauri/binaries/md-indexer-$(TARGET_TRIPLE)
	cp bin/md-api $(DESKTOP_DIR)/src-tauri/binaries/md-api-$(TARGET_TRIPLE)

desktop-dev: desktop-sidecars
	npm --prefix $(DESKTOP_DIR) run tauri dev

desktop: desktop-sidecars
	npm --prefix $(DESKTOP_DIR) run tauri build
