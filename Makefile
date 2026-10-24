
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

# Installs the launcher and icons for the current user, so Wayland compositors show the app icon
# (also for `desktop-dev`); Exec points to the release binary built by `make desktop`.
XDG_DATA_HOME ?= $(HOME)/.local/share
DESKTOP_ICONS := $(DESKTOP_DIR)/src-tauri/icons

desktop-launcher:
	install -Dm644 $(DESKTOP_ICONS)/32x32.png $(XDG_DATA_HOME)/icons/hicolor/32x32/apps/md-indexer-desktop.png
	install -Dm644 $(DESKTOP_ICONS)/128x128.png $(XDG_DATA_HOME)/icons/hicolor/128x128/apps/md-indexer-desktop.png
	install -Dm644 $(DESKTOP_ICONS)/128x128@2x.png $(XDG_DATA_HOME)/icons/hicolor/256x256/apps/md-indexer-desktop.png
	install -Dm644 $(DESKTOP_ICONS)/icon.png $(XDG_DATA_HOME)/icons/hicolor/512x512/apps/md-indexer-desktop.png
	install -Dm644 $(DESKTOP_ICONS)/app-icon.svg $(XDG_DATA_HOME)/icons/hicolor/scalable/apps/md-indexer-desktop.svg
	mkdir -p $(XDG_DATA_HOME)/applications
	sed 's|^Exec=.*|Exec=$(CURDIR)/$(DESKTOP_DIR)/src-tauri/target/release/md-indexer-desktop|' \
		$(DESKTOP_DIR)/src-tauri/md-indexer-desktop.desktop > $(XDG_DATA_HOME)/applications/md-indexer-desktop.desktop
	-update-desktop-database $(XDG_DATA_HOME)/applications
	-kbuildsycoca6 --noincremental 2>/dev/null
