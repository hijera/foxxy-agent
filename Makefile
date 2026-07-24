.PHONY: build build-acp build-desktop icon test lint clean install print-version intellij-build intellij-run vscode-build vscode-build-target vscode-package vscode-package-target

# ---- Build options (extend when you add optional Go build tags) ----
#   TAGS   optional extra `go build -tags` values (space-separated).
#     Recommended full binary (matches default Docker BUILD_TAGS): make build TAGS="http ui scheduler memory"
#     http     OpenAI-compatible gateway (foxxycode http)
#     ui       embedded SPA for GET / (combine with http); runs npm ui-build first
#     scheduler       cron scheduler daemon and tools (see external/scheduler/)
#     memory          long-term memory copilot and /foxxycode memory REST (see external/memory/)
#     gateway.telegram  Telegram bot gateway only (foxxycode gateway; see external/gateway/)
#     gateway         all messenger gateways, currently Telegram (superset of gateway.telegram)
#     desktop         Windows WebView2 desktop shell (foxxycode desktop; combine with http ui)
#   Examples: make build TAGS=http
#             make build TAGS="http ui"
#             make build TAGS="http scheduler"
#             make build TAGS="http ui scheduler memory"
#             make build TAGS="gateway.telegram"
#             make build TAGS="http ui scheduler memory gateway"
#   Omit memory (or other tags) for a slimmer binary; runtime memory.enabled only applies when built with memory.
#   VERSION / LDFLAGS   embedded version string (see print-version).

# Prefer a tag that points at HEAD (semantically latest if several), else nearest tag from history,
# else abbreviated commit (only if this is a git checkout), else "dev".
VERSION := $(shell \
	point=$$(git tag -l --points-at HEAD --sort=-v:refname 2>/dev/null | head -n1); \
	if [ -n "$$point" ]; then echo $$point; \
	elif desc=$$(git describe --tags --dirty 2>/dev/null); then echo $$desc; \
	elif desc=$$(git describe --tags --always --dirty 2>/dev/null); then echo $$desc; \
	else echo dev; fi)
LDFLAGS := -X github.com/hijera/foxxycode-agent/internal/version.Version=$(VERSION)

TAGS ?=
BUILD_DIR := build
BINARY := $(BUILD_DIR)/foxxycode

# Default tag set for `make install` when build/foxxycode is missing (matches Docker BUILD_TAGS).
FULL_TAGS := http ui scheduler memory

# Plain `make` must run `build`. Without this, the first rule would be `print-version`.
.DEFAULT_GOAL := build

ifneq ($(strip $(TAGS)),)
GO_TAGS_FLAG := -tags "$(strip $(TAGS))"
endif

# Embedded UI (go:embed) is included only with both http and ui tags.
ifneq ($(and $(findstring http,$(TAGS)),$(findstring ui,$(TAGS))),)
build: ui-build
endif

DESKTOP_TAGS := http ui scheduler memory desktop
DESKTOP_LDFLAGS := -H=windowsgui $(LDFLAGS)

# Regenerate the Windows app icon resource from the source PNG. Run manually when
# foxxycode2-Photoroom.png changes; the generated .syso is committed so routine
# builds don't need this step. cmd/foxxycode/rsrc_windows_amd64.syso is auto-linked
# by every windows/amd64 go build (desktop shell and CLI) as the .exe file icon.
icon:
	go run internal/desktop/icon/gen.go foxxycode2-Photoroom.png build/foxxycode.ico
	go run github.com/akavel/rsrc -arch amd64 -ico build/foxxycode.ico -o cmd/foxxycode/rsrc_windows_amd64.syso

build-desktop: ui-build
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
	go build -tags "$(DESKTOP_TAGS)" -trimpath \
	  -ldflags "$(DESKTOP_LDFLAGS)" \
	  -o $(BUILD_DIR)/foxxycode-desktop.exe ./cmd/foxxycode/

ui-build:
	npm --prefix external/ui install --no-fund --no-audit
	npm --prefix external/ui run build:go

# Build the foxxycode CLI (skills commands + ACP entrypoint; optional modules via TAGS).
build:
	@mkdir -p $(BUILD_DIR)
	go build $(GO_TAGS_FLAG) -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/foxxycode/

# Print the same version string embedded by `make build` (for manual go build -ldflags).
print-version:
	@echo $(VERSION)

# Install binary: /usr/local/bin for root, ~/.local/bin for regular users.
INSTALL_DIR := $(if $(filter 0,$(shell id -u)),/usr/local/bin,$(HOME)/.local/bin)

# Install build/foxxycode onto PATH. Reuses an existing binary; builds FULL_TAGS only when missing.
install:
	@mkdir -p $(INSTALL_DIR)
	@if [ ! -f $(BINARY) ]; then \
		echo "No $(BINARY); building with TAGS=\"$(FULL_TAGS)\""; \
		$(MAKE) build TAGS="$(FULL_TAGS)"; \
	else \
		echo "Installing existing $(BINARY)"; \
	fi
	cp $(BINARY) $(INSTALL_DIR)/foxxycode
	@echo "Installed to $(INSTALL_DIR)/foxxycode"

# Run all tests.
test:
	go test ./...
	go test -tags=memory ./...
	go test -tags=http ./...
	go test -tags=http,memory ./...
	go test -tags=scheduler ./...
	go test -tags=scheduler,memory ./...
	$(MAKE) ui-build
	go test -tags=http,ui ./...
	go test -tags=http,ui,memory ./...
	go test -tags=http,scheduler ./...
	go test -tags=http,scheduler,memory ./...
	go test -tags=http,scheduler,ui ./...
	go test -tags=http,scheduler,ui,memory ./...

# Clean build artifacts.
clean:
	rm -rf $(BUILD_DIR)

# Run the linter (requires golangci-lint).
lint:
	golangci-lint run ./...

# ---- Editor plugins ----
# Build the JetBrains plugin from the repo root. Requires Go, Node/npm, and a JDK 17 on PATH.
# The Gradle build cross-compiles the bundled foxxycode binary for every desktop target and packs
# them into one plugin zip under editors/intellij/build/distributions/.
# Version defaults to the embedded VERSION; override with `make intellij-build PLUGIN_VERSION=1.2.3`.
PLUGIN_VERSION ?= $(VERSION)

intellij-build:
	cd editors/intellij && chmod +x gradlew && ./gradlew --no-daemon buildPlugin -Pproduction=true -PpluginVersion="$(PLUGIN_VERSION)"

# Launch a sandbox IDE with the plugin (host-platform binary only; fast dev loop).
intellij-run:
	cd editors/intellij && chmod +x gradlew && ./gradlew --no-daemon runIde

# ---- VS Code extension ----
# Build the foxxycode VS Code extension. Two packaging modes:
#   make vscode-build           -> universal: bundle ALL 5 desktop binaries into one VSIX
#   make vscode-build-target TARGET=<goos>-<goarch>
#                              -> build ONE target binary only (fast dev loop / platform-specific)
#   make vscode-package         -> universal VSIX at editors/vscode/foxxycode-vscode-$(PLUGIN_VERSION).vsix
#   make vscode-package-target TARGET=<goos>-<goarch> VSCE_TARGET=<vsce-target>
#                              -> platform-specific VSIX (one per target)
# VSCE_TARGET is the VS Code target id (linux-x64, linux-arm64, darwin-x64, darwin-arm64, win32-x64);
# scripts/prepare-binary.mjs prints the Go -> VS Code mapping.
VSCE_TARGET ?=

# FOXXYCODE_PLUGIN_VERSION stamps the bundled binary's internal/version.Version (read by
# scripts/prepare-binary.mjs), mirroring the IntelliJ gradle build. The vsce version argument
# (guarded to semver-looking PLUGIN_VERSION; CI always passes X.Y.Z or 0.0.0-dev-<sha>) rewrites
# the VSIX **manifest** version — vsce reads package.json, so without it every VSIX shipped the
# static package.json version regardless of the release tag. package.json is snapshotted and
# restored so the source tree is not left dirty.
vscode-build:
	cd editors/vscode && npm install --no-fund --no-audit && FOXXYCODE_PLUGIN_VERSION="$(PLUGIN_VERSION)" npm run build

vscode-build-target:
	cd editors/vscode && npm install --no-fund --no-audit && FOXXYCODE_PLUGIN_VERSION="$(PLUGIN_VERSION)" node scripts/prepare-binary.mjs --target $(TARGET) && npm run compile

vscode-package:
	cd editors/vscode && npm install --no-fund --no-audit && FOXXYCODE_PLUGIN_VERSION="$(PLUGIN_VERSION)" npm run prepare-binary && npm run compile && { \
		cp package.json package.json.vsce.bak; cp package-lock.json package-lock.json.vsce.bak; \
		case "$(PLUGIN_VERSION)" in \
			[0-9]*.[0-9]*.[0-9]*) npx vsce package "$(PLUGIN_VERSION)" --no-git-tag-version -o foxxycode-vscode-$(PLUGIN_VERSION).vsix ;; \
			*) npx vsce package -o foxxycode-vscode-$(PLUGIN_VERSION).vsix ;; \
		esac; \
		status=$$?; mv package.json.vsce.bak package.json; mv package-lock.json.vsce.bak package-lock.json; exit $$status; \
	}

vscode-package-target:
	cd editors/vscode && npm install --no-fund --no-audit && FOXXYCODE_PLUGIN_VERSION="$(PLUGIN_VERSION)" node scripts/prepare-binary.mjs --target $(TARGET) && npm run compile && { \
		cp package.json package.json.vsce.bak; cp package-lock.json package-lock.json.vsce.bak; \
		case "$(PLUGIN_VERSION)" in \
			[0-9]*.[0-9]*.[0-9]*) npx vsce package "$(PLUGIN_VERSION)" --no-git-tag-version --target $(VSCE_TARGET) -o foxxycode-vscode-$(VSCE_TARGET)-$(PLUGIN_VERSION).vsix ;; \
			*) npx vsce package --target $(VSCE_TARGET) -o foxxycode-vscode-$(VSCE_TARGET)-$(PLUGIN_VERSION).vsix ;; \
		esac; \
		status=$$?; mv package.json.vsce.bak package.json; mv package-lock.json.vsce.bak package-lock.json; exit $$status; \
	}
