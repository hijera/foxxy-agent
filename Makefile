.PHONY: build build-acp build-desktop icon test test-opencode-rules ui-test check-windows lint lint-ui lint-windows clean install print-version hooks intellij-build intellij-run vscode-build vscode-build-target vscode-package vscode-package-target

# ---- Build options (extend when you add optional Go build tags) ----
#   TAGS   optional extra `go build -tags` values (space-separated).
#     Recommended full binary (FULL_TAGS below; what the release CLI archives ship):
#       make build TAGS="http ui scheduler memory cli browser gateway"
#     http     OpenAI-compatible gateway (foxxycode http)
#     ui       embedded SPA for GET / (combine with http); runs npm ui-build first
#     scheduler       cron scheduler daemon and tools (see external/scheduler/)
#     memory          long-term memory copilot and /foxxycode memory REST (see external/memory/)
#     gateway.telegram  Telegram bot gateway only (foxxycode gateway; see external/gateway/)
#     gateway         all messenger gateways, currently Telegram (superset of gateway.telegram)
#     cli      interactive console TUI (bare `foxxycode` on a terminal; see external/cli/)
#     desktop         Windows WebView2 desktop shell (foxxycode desktop; combine with http ui)
#   Examples: make build TAGS=http
#             make build TAGS="http ui"
#             make build TAGS="http scheduler"
#             make build TAGS="http ui scheduler memory"
#             make build TAGS=cli
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
FULL_TAGS := http ui scheduler memory cli browser gateway

# Plain `make` must run `build`. Without this, the first rule would be `print-version`.
.DEFAULT_GOAL := build

ifneq ($(strip $(TAGS)),)
GO_TAGS_FLAG := -tags "$(strip $(TAGS))"
endif

# Embedded UI (go:embed) is included only with both http and ui tags.
ifneq ($(and $(findstring http,$(TAGS)),$(findstring ui,$(TAGS))),)
build: ui-build
endif

DESKTOP_TAGS := http ui scheduler memory desktop browser
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

# `npm --prefix DIR` installs INTO DIR but still reads package.json from the
# current directory on Windows npm, so it fails at the repo root with a confusing
# "Could not read package.json" pointing at a file that was never meant to exist.
# Changing directory works the same way on every platform.
ui-build:
	cd external/ui && npm install --no-fund --no-audit
	cd external/ui && npm run build:go

# Run the SPA's own unit suite (vitest, 1000+ tests) and type-check it. Neither
# was gated: `make test` only ever ran Go tests, and `vite build` compiles TypeScript
# with esbuild, which strips types without checking them - so a type error shipped
# silently. Kept separate from ui-build so a Go-only change does not pay for it,
# and wired into `test` below.
ui-test:
	cd external/ui && npm install --no-fund --no-audit
	cd external/ui && npm run typecheck
	cd external/ui && npm test

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

# Test the project plugin that attaches Cursor rules to OpenCode sessions.
test-opencode-rules:
	node --test .opencode/tests/project-rules.test.js

# Run all tests.
test: test-opencode-rules
	go test ./...
	go test -tags=memory ./...
	go test -tags=cli ./...
	go test -tags=cli,scheduler,memory ./...
	go test -tags=http,cli ./...
	go test -tags=http ./...
	go test -tags=http,memory ./...
	go test -tags=browser ./...
	go test -tags=scheduler ./...
	go test -tags=scheduler,memory ./...
	go test -tags=gateway ./...
	go test -tags=browser ./...
	go test -tags=http,gateway,scheduler,memory ./...
	$(MAKE) ui-build
	$(MAKE) ui-test
	go test -tags=http,ui ./...
	go test -tags=http,ui,memory ./...
	go test -tags=http,scheduler ./...
	go test -tags=http,scheduler,memory ./...
	go test -tags=http,scheduler,ui ./...
	go test -tags=http,scheduler,ui,memory ./...

# Type-check the Windows build without a Windows machine.
#
# The suite above runs on the host only, so every file behind //go:build windows
# — the process group probe, console output decoding, the shell detector — is
# invisible to it and to the linter. A change to a shared signature therefore
# compiles here and breaks there, which is exactly the kind of drift nobody sees
# until a user reports it. go vet builds test files too, so this covers the
# Windows-only tests as well.
#
# The ui tag is deliberately absent: it embeds assets that only exist after
# ui-build, and it carries no platform-specific code. desktop is the reverse -
# //go:build desktop && windows means this target is the only place it is ever
# type-checked, so it must stay in the list.
check-windows:
	GOOS=windows go build ./...
	GOOS=windows go build -tags=cli ./...
	GOOS=windows go vet ./...
	GOOS=windows go vet -tags=cli ./...
	GOOS=windows go vet -tags=cli,scheduler,memory ./...
	GOOS=windows go vet -tags=memory ./...
	GOOS=windows go vet -tags=http ./...
	GOOS=windows go vet -tags=http,memory ./...
	GOOS=windows go vet -tags=browser ./...
	GOOS=windows go vet -tags=scheduler ./...
	GOOS=windows go vet -tags=scheduler,memory ./...
	GOOS=windows go vet -tags=http,scheduler ./...
	GOOS=windows go vet -tags=http,scheduler,memory ./...
	GOOS=windows go vet -tags=gateway ./...
	GOOS=windows go vet -tags=browser ./...
	GOOS=windows go vet -tags=desktop,http,scheduler,memory ./...

# Clean build artifacts.
clean:
	rm -rf $(BUILD_DIR)

# Tag sets the linter must cover. A build tag hides whole files from the linter,
# so anything not listed here is never linted: for a long time only the untagged
# and cli passes ran, which left external/httpserver, external/memory and
# external/scheduler - the bulk of external/ - unchecked. Keep this in sync with
# the TAGS list at the top of this file whenever a new optional tag lands.
#
# The combinations compile every file at least once rather than enumerating the
# power set: http,scheduler,memory covers the optional server surfaces together,
# browser covers the chromedp tool, cli covers the TUI, gateway covers the
# messenger bots (gateway.telegram is a subset of gateway). The ui tag lives in
# lint-ui because it embeds a bundle that only exists after ui-build, and
# desktop lives in lint-windows because it is //go:build desktop && windows.
LINT_TAG_SETS := cli browser gateway http,scheduler,memory,gateway

# Fail on every finding rather than golangci-lint's default caps
# (max-issues-per-linter=50, max-same-issues=3), which silently hid most of a
# backlog of identical errcheck hits behind the first three of each kind.
LINT_FLAGS := --max-issues-per-linter 0 --max-same-issues 0

# Run the linter (requires golangci-lint): the untagged pass plus one pass per
# tag set above. Needs no npm - run `make lint-ui` for the embedded-SPA pass.
lint:
	golangci-lint run $(LINT_FLAGS) ./...
	@for t in $(LINT_TAG_SETS); do \
		echo "==> golangci-lint --build-tags $$t"; \
		golangci-lint run $(LINT_FLAGS) --build-tags "$$t" ./... || exit 1; \
	done

# Lint the http+ui surface (spa_embed_ui.go and friends). Separate from lint
# because the ui tag go:embeds external/ui/dist, which is gitignored and only
# exists after ui-build - so this target needs Node, and plain `make lint` does not.
lint-ui: ui-build
	golangci-lint run $(LINT_FLAGS) --build-tags http,scheduler,memory,ui ./...

# Run the linter against the Windows build, which lint above never compiles.
# desktop is Windows-only (//go:build desktop && windows), so this is the only
# pass that ever sees internal/desktop; it compiles without the ui tag, so no
# bundle is needed here either.
LINT_TAG_SETS_WINDOWS := $(LINT_TAG_SETS) desktop,http,scheduler,memory

lint-windows:
	GOOS=windows golangci-lint run $(LINT_FLAGS) ./...
	@for t in $(LINT_TAG_SETS_WINDOWS); do \
		echo "==> GOOS=windows golangci-lint --build-tags $$t"; \
		GOOS=windows golangci-lint run $(LINT_FLAGS) --build-tags "$$t" ./... || exit 1; \
	done

# Enable the repo's git hooks (pre-commit runs scripts/checks.sh). One-time per clone.
# Bypass a single commit with: git commit --no-verify
hooks:
	git config core.hooksPath .githooks
	@echo "Enabled .githooks — 'git commit' now runs the linter (scripts/checks.sh)."
	@echo "Add tests with FOXXYCODE_HOOK_TESTS=fast|full; skip lint with FOXXYCODE_HOOK_LINT=0; bypass once with --no-verify."

# ---- Editor plugins ----
# Build the JetBrains plugin from the repo root. Requires Go, Node/npm, and a JDK 17 on PATH.
# The Gradle build cross-compiles the bundled foxxycode binary for every desktop target and packs
# them into one plugin zip under editors/intellij/build/distributions/.
# Version defaults to the embedded VERSION; override with `make intellij-build PLUGIN_VERSION=1.2.3`.
PLUGIN_VERSION ?= $(VERSION)

intellij-build:
	cd editors/intellij && chmod +x gradlew && ./gradlew --no-daemon buildPlugin -Pproduction=true -PpluginVersion="$(PLUGIN_VERSION)"

# Run the plugin's Kotlin unit tests. buildPlugin does not depend on `test`, so
# until this existed the suite under editors/intellij/src/test was never executed
# by any gate. Requires a JDK 17 on PATH (the plugin's toolchain).
intellij-test:
	cd editors/intellij && chmod +x gradlew && ./gradlew --no-daemon test

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
