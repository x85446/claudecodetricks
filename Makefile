# Makefile for claudecodetricks
# Robust build system with versioning, testing, and installation

.DEFAULT_GOAL := build
SHELL := /bin/bash

# ==================================================================================== #
# ==================================================================================== #

# Binary output directory
BIN_DIR := plugins/session-hooks/hooks

# Standalone tool output directory (not a Claude Code hook, just a CLI)
TOOLS_BIN_DIR := bin

# Binary names
VOICE_BIN := $(BIN_DIR)/voice-announcer
LOGGER_BIN := $(BIN_DIR)/session-logger
GIT_BIN := $(BIN_DIR)/git-committer
CHAIN_GUARD_BIN := $(BIN_DIR)/chain-guard
DOC_GUARD_BIN := $(BIN_DIR)/doc-guard
ITERATE_RUN_BIN := $(TOOLS_BIN_DIR)/iterate-run
TESTMASTER_BIN := $(TOOLS_BIN_DIR)/testmaster

# Where standalone tools (as opposed to hooks) get installed — already on
# PATH for a standard Go setup, unlike INSTALL_DIR below.
TOOLS_INSTALL_DIR := $(HOME)/go/bin

# Source directories
SRC_DIR := src
CMD_DIR := $(SRC_DIR)/cmd
INTERNAL_DIR := $(SRC_DIR)/internal
PKG_DIR := $(SRC_DIR)/pkg

# Installation directories
INSTALL_DIR := $(HOME)/.claude/hooks
LOG_DIR := $(HOME)/.claude/log

# Always-on dashboard daemon (launchd, macOS only — see makehelp.sh).
# ITERATE_SERVE_PORT is the fixed default port also baked into
# `iterate-run serve`'s own --port default (src/cmd/iterate-run/main.go);
# kept as one value here so the Chrome daemon's start URL can never drift
# out of sync with the port the serve daemon actually binds.
ITERATE_SERVE_PORT := 8420
DASHBOARD_URL := http://localhost:$(ITERATE_SERVE_PORT)/

# Keep-alive Chrome tab on the dashboard (launchd, macOS only). CHROME_BIN
# is overridable so this can point at Chromium or any Chromium-family
# browser instead of Google Chrome (proprietary, run as an installed app,
# never linked or redistributed) without editing the plist by hand:
#   make chrome-install CHROME_BIN=/path/to/Chromium
# CHROME_DEBUG_PORT is deliberately NOT Chrome's conventional devtools
# port (9222): this user runs browser-automation tooling that expects
# 9222 to be their normal Chrome profile, and squatting that well-known
# port would make anything attaching to it silently land in this isolated
# profile instead — no extensions, no logins — a "my automation broke"
# bug wearing a "something took my port" costume. 9242 is ours, out of
# the well-known range, in the same spirit as 8420 for serve rather than
# 8080; confirmed free on this machine (lsof -iTCP -sTCP:LISTEN) before
# picking it. The debug port is what makes this a daemon in the sense
# that matters, since something else can attach to it.
CHROME_BIN ?= /Applications/Google Chrome.app/Contents/MacOS/Google Chrome
CHROME_DEBUG_PORT := 9242
CHROME_PROFILE_DIR := $(HOME)/Library/Application Support/iterate-run/chrome-profile

# Go configuration
GO := go
GOFMT := gofmt
GOVET := $(GO) vet
GOTEST := $(GO) test
GOLINT := golangci-lint
GO_FILES = $(shell find $(SRC_DIR) -name '*.go' -type f)
GO_PACKAGES = $(shell cd $(SRC_DIR) && $(GO) list ./...)

# Version information from git
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_TAG := $(shell git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
GIT_DIRTY := $(shell git diff --quiet 2>/dev/null || echo "-dirty")
VERSION := $(GIT_TAG)$(GIT_DIRTY)
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')

# Build flags
LDFLAGS := -s -w
LDFLAGS += -X main.Version=$(VERSION)
LDFLAGS += -X main.Commit=$(GIT_COMMIT)
LDFLAGS += -X main.BuildTime=$(BUILD_TIME)
GOFLAGS := -ldflags="$(LDFLAGS)" -trimpath

# Colors for output
COLOR_RESET := \033[0m
COLOR_BOLD := \033[1m
COLOR_GREEN := \033[32m
COLOR_YELLOW := \033[33m
COLOR_BLUE := \033[34m
COLOR_CYAN := \033[36m

# Verbose flag
V ?= 0
ifeq ($(V),1)
    Q :=
    VERBOSE := -v
else
    Q := @
    VERBOSE :=
endif

# ==================================================================================== #
# ==================================================================================== #

.PHONY: all build check-tools clean test test-integration test-unit test-hook
.PHONY: deps deps-check deps-update
.PHONY: fmt fmt-check vet lint check
.PHONY: install uninstall
.PHONY: coverage coverage-html tutorial-box-test iterate-contract-test
.PHONY: nightly-install nightly-uninstall nightly-status nightly-tick
.PHONY: run watch
.PHONY: version info
.PHONY: help
.PHONY: serve-install serve-uninstall serve-status
.PHONY: chrome-install chrome-uninstall chrome-status

# ==================================================================================== #
# ==================================================================================== #


##@ General

help:  ## Display this help message
	@awk 'BEGIN {FS = ":.*##"; printf "\n\033[1mclaudecodetricks\033[0m\n\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)

version:  ## Display version information
	$(Q)echo -e "$(COLOR_CYAN)Version:    $(VERSION)$(COLOR_RESET)"
	$(Q)echo -e "$(COLOR_CYAN)Commit:     $(GIT_COMMIT)$(COLOR_RESET)"
	$(Q)echo -e "$(COLOR_CYAN)Build Time: $(BUILD_TIME)$(COLOR_RESET)"

info:  ## Display build configuration
	$(Q)echo -e "$(COLOR_BOLD)Build Configuration:$(COLOR_RESET)"
	$(Q)echo -e "  Go version:    $$($(GO) version)"
	$(Q)echo -e "  Source dir:    $(SRC_DIR)"
	$(Q)echo -e "  Output dir:    $(BIN_DIR)"
	$(Q)echo -e "  Install dir:   $(INSTALL_DIR)"
	$(Q)echo ""
	$(Q)echo -e "$(COLOR_BOLD)Version Information:$(COLOR_RESET)"
	$(Q)echo -e "  Version:       $(VERSION)"
	$(Q)echo -e "  Commit:        $(GIT_COMMIT)"
	$(Q)echo -e "  Build time:    $(BUILD_TIME)"
	$(Q)echo ""
	$(Q)echo -e "$(COLOR_BOLD)Binaries:$(COLOR_RESET)"
	$(Q)echo -e "  voice-announcer: $(VOICE_BIN)"
	$(Q)echo -e "  session-logger:  $(LOGGER_BIN)"
	$(Q)echo -e "  git-committer:   $(GIT_BIN)"
	$(Q)echo -e "  chain-guard:     $(CHAIN_GUARD_BIN)"
	$(Q)echo -e "  doc-guard:       $(DOC_GUARD_BIN)"
	$(Q)echo ""
	$(Q)echo -e "$(COLOR_BOLD)Build Status:$(COLOR_RESET)"
	$(Q)for bin in $(VOICE_BIN) $(LOGGER_BIN) $(GIT_BIN) $(CHAIN_GUARD_BIN) $(DOC_GUARD_BIN); do \
		if [ -f $$bin ]; then \
			echo -e "  $(COLOR_GREEN)✓$(COLOR_RESET) $$bin ($$(du -h $$bin | cut -f1))"; \
		else \
			echo -e "  $(COLOR_YELLOW)✗$(COLOR_RESET) $$bin (not built)"; \
		fi \
	done

# ==================================================================================== #
# UTILITY TARGETS
# ==================================================================================== #

check-tools:  ## Verify required tools are installed
	$(Q)if ! command -v $(GO) >/dev/null 2>&1; then \
		echo -e "$(COLOR_YELLOW)⚠ Go not found. Install from https://golang.org/$(COLOR_RESET)"; \
		exit 1; \
	fi

##@ Build

all: build  ## Alias for build — builds everything

build: check-tools $(VOICE_BIN) $(LOGGER_BIN) $(GIT_BIN) $(CHAIN_GUARD_BIN) $(DOC_GUARD_BIN) $(ITERATE_RUN_BIN) $(TESTMASTER_BIN)  ## Build all binaries
	$(Q)echo -e "$(COLOR_GREEN)✓ Build complete$(COLOR_RESET)"
	$(Q)echo -e "$(COLOR_CYAN)Version: $(VERSION) ($(GIT_COMMIT))$(COLOR_RESET)"

$(VOICE_BIN): $(wildcard $(CMD_DIR)/voice-announcer/*.go) $(GO_FILES)
	$(Q)echo -e "$(COLOR_BLUE)→ Building voice-announcer...$(COLOR_RESET)"
	$(Q)mkdir -p $(BIN_DIR)
	$(Q)cd $(SRC_DIR) && $(GO) build $(VERBOSE) $(GOFLAGS) -o ../$@ ./cmd/voice-announcer
	$(Q)echo -e "  $(COLOR_GREEN)✓$(COLOR_RESET) voice-announcer → $@"

$(LOGGER_BIN): $(wildcard $(CMD_DIR)/session-logger/*.go) $(GO_FILES)
	$(Q)echo -e "$(COLOR_BLUE)→ Building session-logger...$(COLOR_RESET)"
	$(Q)mkdir -p $(BIN_DIR)
	$(Q)cd $(SRC_DIR) && $(GO) build $(VERBOSE) $(GOFLAGS) -o ../$@ ./cmd/session-logger
	$(Q)echo -e "  $(COLOR_GREEN)✓$(COLOR_RESET) session-logger → $@"

$(GIT_BIN): $(wildcard $(CMD_DIR)/git-committer/*.go) $(GO_FILES)
	$(Q)echo -e "$(COLOR_BLUE)→ Building git-committer...$(COLOR_RESET)"
	$(Q)mkdir -p $(BIN_DIR)
	$(Q)cd $(SRC_DIR) && $(GO) build $(VERBOSE) $(GOFLAGS) -o ../$@ ./cmd/git-committer
	$(Q)echo -e "  $(COLOR_GREEN)✓$(COLOR_RESET) git-committer → $@"

$(CHAIN_GUARD_BIN): $(wildcard $(CMD_DIR)/chain-guard/*.go) $(wildcard $(INTERNAL_DIR)/chainguard/*.go)
	$(Q)echo -e "$(COLOR_BLUE)→ Building chain-guard...$(COLOR_RESET)"
	$(Q)mkdir -p $(BIN_DIR)
	$(Q)cd $(SRC_DIR) && $(GO) build $(VERBOSE) $(GOFLAGS) -o ../$@ ./cmd/chain-guard
	$(Q)echo -e "  $(COLOR_GREEN)✓$(COLOR_RESET) chain-guard → $@"

$(DOC_GUARD_BIN): $(wildcard $(CMD_DIR)/doc-guard/*.go) $(wildcard $(INTERNAL_DIR)/docguard/*.go)
	$(Q)echo -e "$(COLOR_BLUE)→ Building doc-guard...$(COLOR_RESET)"
	$(Q)mkdir -p $(BIN_DIR)
	$(Q)cd $(SRC_DIR) && $(GO) build $(VERBOSE) $(GOFLAGS) -o ../$@ ./cmd/doc-guard
	$(Q)echo -e "  $(COLOR_GREEN)✓$(COLOR_RESET) doc-guard → $@"

$(ITERATE_RUN_BIN): $(wildcard $(CMD_DIR)/iterate-run/*.go) $(wildcard $(INTERNAL_DIR)/iterrun/*.go)
	$(Q)echo -e "$(COLOR_BLUE)→ Building iterate-run...$(COLOR_RESET)"
	$(Q)mkdir -p $(TOOLS_BIN_DIR)
	$(Q)cd $(SRC_DIR) && $(GO) build $(VERBOSE) $(GOFLAGS) -o ../$@ ./cmd/iterate-run
	$(Q)echo -e "  $(COLOR_GREEN)✓$(COLOR_RESET) iterate-run → $@"

$(TESTMASTER_BIN): $(wildcard $(CMD_DIR)/testmaster/*.go) $(wildcard $(INTERNAL_DIR)/testmaster/*.go)
	$(Q)echo -e "$(COLOR_BLUE)→ Building testmaster...$(COLOR_RESET)"
	$(Q)mkdir -p $(TOOLS_BIN_DIR)
	$(Q)cd $(SRC_DIR) && $(GO) build $(VERBOSE) $(GOFLAGS) -o ../$@ ./cmd/testmaster
	$(Q)echo -e "  $(COLOR_GREEN)✓$(COLOR_RESET) testmaster → $@"

rebuild: clean build  ## Clean and rebuild all binaries

# ==================================================================================== #
# TESTING TARGETS
# ==================================================================================== #

##@ Test

test: test-unit test-hook  ## Run all tests
	$(Q)echo -e "$(COLOR_GREEN)✓ All tests passed$(COLOR_RESET)"

test-unit: check-tools  ## Run Go unit tests
	$(Q)echo -e "$(COLOR_BLUE)→ Running unit tests...$(COLOR_RESET)"
	$(Q)cd $(SRC_DIR) && $(GOTEST) $(VERBOSE) -race -timeout 30s ./...

test-hook: build  ## Test hooks with sample JSON input
	$(Q)echo -e "$(COLOR_BLUE)→ Testing voice-announcer...$(COLOR_RESET)"
	$(Q)echo '{"hook_event_name":"Stop","cwd":"$(HOME)/.claude"}' | $(VOICE_BIN) && \
		echo -e "  $(COLOR_GREEN)✓$(COLOR_RESET) voice-announcer test passed" || \
		echo -e "  $(COLOR_YELLOW)⚠$(COLOR_RESET) voice-announcer test completed with errors"
	$(Q)echo ""
	$(Q)echo -e "$(COLOR_BLUE)→ Testing session-logger...$(COLOR_RESET)"
	$(Q)echo '{"hook_event_name":"Stop","transcript_path":"","cwd":"$(HOME)/.claude"}' | $(LOGGER_BIN) && \
		echo -e "  $(COLOR_GREEN)✓$(COLOR_RESET) session-logger test passed" || \
		echo -e "  $(COLOR_YELLOW)⚠$(COLOR_RESET) session-logger test completed with errors"
	$(Q)echo ""
	$(Q)echo -e "$(COLOR_BLUE)→ Testing git-committer...$(COLOR_RESET)"
	$(Q)echo '{"hook_event_name":"PostToolUse","tool_name":"Write","cwd":"/tmp","tool_input":{"file_path":"test.txt"},"permission_mode":"default"}' | $(GIT_BIN) && \
		echo -e "  $(COLOR_GREEN)✓$(COLOR_RESET) git-committer test passed" || \
		echo -e "  $(COLOR_YELLOW)⚠$(COLOR_RESET) git-committer test completed with errors"

tutorial-box-test:  ## Render test for the tutorial runtime's boxes and menu on a pty at 60/80/132 columns (box-1..box-7)
	$(Q)bash skills/tutorial/tests/box-test.sh

iterate-contract-test:  ## Iterate family contract-expansion rules, in source and as installed (contract-1..contract-5)
	$(Q)bash skills/iterate/tests/contract-test.sh

coverage: check-tools  ## Generate test coverage report
	$(Q)echo -e "$(COLOR_BLUE)→ Generating coverage report...$(COLOR_RESET)"
	$(Q)cd $(SRC_DIR) && $(GOTEST) -coverprofile=../coverage.out -covermode=atomic ./...
	$(Q)cd $(SRC_DIR) && $(GO) tool cover -func=../coverage.out
	$(Q)echo -e "$(COLOR_GREEN)✓ Coverage report: coverage.out$(COLOR_RESET)"

coverage-html: coverage  ## Generate HTML coverage report
	$(Q)echo -e "$(COLOR_BLUE)→ Generating HTML coverage report...$(COLOR_RESET)"
	$(Q)cd $(SRC_DIR) && $(GO) tool cover -html=../coverage.out -o ../coverage.html
	$(Q)echo -e "$(COLOR_GREEN)✓ HTML coverage report: coverage.html$(COLOR_RESET)"
	$(Q)which open >/dev/null && open coverage.html || true

# ==================================================================================== #
# QUALITY TARGETS
# ==================================================================================== #

##@ Quality

fmt: check-tools  ## Format all Go source files
	$(Q)echo -e "$(COLOR_BLUE)→ Formatting Go files...$(COLOR_RESET)"
	$(Q)$(GOFMT) -w -s $(GO_FILES)
	$(Q)echo -e "$(COLOR_GREEN)✓ Formatting complete$(COLOR_RESET)"

fmt-check: check-tools  ## Check if Go files are formatted
	$(Q)echo -e "$(COLOR_BLUE)→ Checking Go formatting...$(COLOR_RESET)"
	$(Q)UNFORMATTED=$$($(GOFMT) -l $(GO_FILES)); \
	if [ -n "$$UNFORMATTED" ]; then \
		echo -e "$(COLOR_YELLOW)⚠ Unformatted files:$(COLOR_RESET)"; \
		echo "$$UNFORMATTED"; \
		exit 1; \
	fi
	$(Q)echo -e "$(COLOR_GREEN)✓ All files are formatted$(COLOR_RESET)"

vet: check-tools  ## Run go vet on all packages
	$(Q)echo -e "$(COLOR_BLUE)→ Running go vet...$(COLOR_RESET)"
	$(Q)cd $(SRC_DIR) && $(GOVET) ./...
	$(Q)echo -e "$(COLOR_GREEN)✓ go vet passed$(COLOR_RESET)"

lint:  ## Run golangci-lint (if available)
	$(Q)if command -v $(GOLINT) >/dev/null 2>&1; then \
		echo -e "$(COLOR_BLUE)→ Running golangci-lint...$(COLOR_RESET)"; \
		cd $(SRC_DIR) && $(GOLINT) run $(VERBOSE) ./...; \
		echo -e "$(COLOR_GREEN)✓ Linting complete$(COLOR_RESET)"; \
	else \
		echo -e "$(COLOR_YELLOW)⚠ golangci-lint not installed, skipping$(COLOR_RESET)"; \
		echo "  Install: curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b \$$(go env GOPATH)/bin"; \
	fi

check: fmt-check vet test-unit  ## Run all quality checks (fmt-check, vet, test)
	$(Q)echo -e "$(COLOR_GREEN)✓ All checks passed$(COLOR_RESET)"

# ==================================================================================== #
# DEPENDENCY TARGETS
# ==================================================================================== #

##@ Install

# codesign --force --sign - after each cp is required on macOS: cp -f onto
# an existing file leaves the copy's ad-hoc signature invalid on some
# macOS/filesystem combinations (confirmed live — the resulting binary
# still runs fine from the build directory, but launched from its
# installed path it's SIGKILLed on every invocation with
# "Taskgated Invalid Signature", CODESIGNING/1, no error message at all).
# Re-signing after the copy fixes it. No-op (skipped) where codesign isn't
# on PATH, i.e. Linux.
install: build  ## Install hooks to Claude Code directory, and tools onto PATH
	$(Q)echo -e "$(COLOR_BLUE)→ Installing hooks...$(COLOR_RESET)"
	$(Q)mkdir -p $(INSTALL_DIR)
	$(Q)mkdir -p $(LOG_DIR)
	$(Q)cp -f $(VOICE_BIN) $(INSTALL_DIR)/
	$(Q)cp -f $(LOGGER_BIN) $(INSTALL_DIR)/
	$(Q)cp -f $(GIT_BIN) $(INSTALL_DIR)/
	$(Q)cp -f $(CHAIN_GUARD_BIN) $(INSTALL_DIR)/
	$(Q)cp -f $(DOC_GUARD_BIN) $(INSTALL_DIR)/
	$(Q)chmod +x $(INSTALL_DIR)/voice-announcer
	$(Q)chmod +x $(INSTALL_DIR)/session-logger
	$(Q)chmod +x $(INSTALL_DIR)/git-committer
	$(Q)chmod +x $(INSTALL_DIR)/chain-guard
	$(Q)chmod +x $(INSTALL_DIR)/doc-guard
	$(Q)if command -v codesign >/dev/null 2>&1; then \
		codesign --force --sign - $(INSTALL_DIR)/voice-announcer $(INSTALL_DIR)/session-logger $(INSTALL_DIR)/git-committer $(INSTALL_DIR)/chain-guard $(INSTALL_DIR)/doc-guard; \
	fi
	$(Q)echo -e "$(COLOR_GREEN)✓ Hooks installed to $(INSTALL_DIR)$(COLOR_RESET)"
	$(Q)echo -e "$(COLOR_CYAN)  Note: Update ~/.claude/settings.json to enable hooks$(COLOR_RESET)"
	$(Q)echo -e "$(COLOR_BLUE)→ Installing tools...$(COLOR_RESET)"
	$(Q)mkdir -p $(TOOLS_INSTALL_DIR)
	$(Q)cp -f $(ITERATE_RUN_BIN) $(TOOLS_INSTALL_DIR)/
	$(Q)cp -f $(TESTMASTER_BIN) $(TOOLS_INSTALL_DIR)/
	$(Q)chmod +x $(TOOLS_INSTALL_DIR)/iterate-run $(TOOLS_INSTALL_DIR)/testmaster
	$(Q)if command -v codesign >/dev/null 2>&1; then \
		codesign --force --sign - $(TOOLS_INSTALL_DIR)/iterate-run $(TOOLS_INSTALL_DIR)/testmaster; \
	fi
	$(Q)echo -e "$(COLOR_GREEN)✓ iterate-run and testmaster installed to $(TOOLS_INSTALL_DIR)$(COLOR_RESET)"

uninstall:  ## Remove installed hooks and tools
	$(Q)echo -e "$(COLOR_YELLOW)→ Uninstalling hooks...$(COLOR_RESET)"
	$(Q)rm -f $(INSTALL_DIR)/voice-announcer
	$(Q)rm -f $(INSTALL_DIR)/session-logger
	$(Q)rm -f $(INSTALL_DIR)/git-committer
	$(Q)rm -f $(INSTALL_DIR)/chain-guard
	$(Q)rm -f $(INSTALL_DIR)/doc-guard
	$(Q)rm -f $(TOOLS_INSTALL_DIR)/iterate-run
	$(Q)rm -f $(TOOLS_INSTALL_DIR)/testmaster
	$(Q)echo -e "$(COLOR_GREEN)✓ Hooks and tools uninstalled$(COLOR_RESET)"

# ==================================================================================== #
# DAEMON TARGETS (macOS only — always-on dashboard + Chrome, via launchd)
# ==================================================================================== #
# Complex logic (plist templating, launchctl sequencing, OS branching) lives
# in makehelp.sh, not inline here — see that file's header comment. Every
# target below fails with a clear message on non-macOS rather than half
# installing, since launchd is not portable.

serve-install: install  ## Load the always-on dashboard as a launchd agent (RunAtLoad + KeepAlive)
	$(Q)./makehelp.sh serve-install "$(TOOLS_INSTALL_DIR)/iterate-run" "$(ITERATE_SERVE_PORT)"

serve-uninstall:  ## Unload and remove the dashboard launchd agent
	$(Q)./makehelp.sh serve-uninstall

serve-status:  ## Show whether the dashboard launchd agent is loaded and running
	$(Q)./makehelp.sh serve-status

# CHROME_BIN overrides the browser binary (Chromium/any Chromium-family
# browser instead of Google Chrome) without editing the plist:
#   make chrome-install CHROME_BIN=/path/to/Chromium
chrome-install:  ## Load a dedicated, always-on Chrome tab on the dashboard as a launchd agent
	$(Q)./makehelp.sh chrome-install "$(CHROME_BIN)" "$(CHROME_DEBUG_PORT)" "$(CHROME_PROFILE_DIR)" "$(DASHBOARD_URL)"

chrome-uninstall:  ## Unload and remove the Chrome launchd agent (stops it for good)
	$(Q)./makehelp.sh chrome-uninstall

chrome-status:  ## Show whether the Chrome launchd agent is loaded and running
	$(Q)./makehelp.sh chrome-status

# The nightly tick (com.x85446.iterate-nightly) runs enrolled projects' staged
# plans unattended; the installed iterate-run owns the plist, the switch and
# the per-project enrollment, so these are thin aliases onto it.
nightly-install: install  ## Arm the nightly launchd tick (iterate-run nightly install, every 10m)
	$(Q)$(TOOLS_INSTALL_DIR)/iterate-run nightly install $(if $(NIGHTLY_EVERY),--every $(NIGHTLY_EVERY),)

nightly-uninstall:  ## Boot out and remove the nightly launchd tick
	$(Q)$(TOOLS_INSTALL_DIR)/iterate-run nightly uninstall

nightly-status:  ## Show the nightly tick: plist, switch, and each enrolled project's last run
	$(Q)$(TOOLS_INSTALL_DIR)/iterate-run nightly status

nightly-tick:  ## Dry-run one nightly tick: what would launch, what is skipped and why
	$(Q)$(TOOLS_INSTALL_DIR)/iterate-run nightly tick --dry-run

# ==================================================================================== #
# DEVELOPMENT TARGETS
# ==================================================================================== #

##@ Development

deps:  ## Install and tidy Go dependencies
	$(Q)echo -e "$(COLOR_BLUE)→ Installing dependencies...$(COLOR_RESET)"
	$(Q)cd $(SRC_DIR) && $(GO) mod download
	$(Q)cd $(SRC_DIR) && $(GO) mod tidy
	$(Q)echo -e "$(COLOR_GREEN)✓ Dependencies installed$(COLOR_RESET)"

deps-check:  ## Verify dependencies
	$(Q)echo -e "$(COLOR_BLUE)→ Verifying dependencies...$(COLOR_RESET)"
	$(Q)cd $(SRC_DIR) && $(GO) mod verify
	$(Q)echo -e "$(COLOR_GREEN)✓ Dependencies verified$(COLOR_RESET)"

deps-update:  ## Update all dependencies
	$(Q)echo -e "$(COLOR_BLUE)→ Updating dependencies...$(COLOR_RESET)"
	$(Q)cd $(SRC_DIR) && $(GO) get -u ./...
	$(Q)cd $(SRC_DIR) && $(GO) mod tidy
	$(Q)echo -e "$(COLOR_GREEN)✓ Dependencies updated$(COLOR_RESET)"

# ==================================================================================== #
# INSTALLATION TARGETS
# ==================================================================================== #

run: build  ## Run the iterate dashboard on a free port and print its URL (ARGS="..." to pass arguments)
	$(Q)./makehelp.sh run "$(ITERATE_RUN_BIN)" $(ARGS)

watch:  ## Watch for changes and rebuild (requires fswatch or inotifywait)
	$(Q)if command -v fswatch >/dev/null 2>&1; then \
		echo -e "$(COLOR_BLUE)→ Watching for changes (fswatch)...$(COLOR_RESET)"; \
		fswatch -o $(SRC_DIR) | xargs -n1 -I{} make build; \
	elif command -v inotifywait >/dev/null 2>&1; then \
		echo -e "$(COLOR_BLUE)→ Watching for changes (inotifywait)...$(COLOR_RESET)"; \
		while true; do \
			inotifywait -r -e modify $(SRC_DIR); \
			make build; \
		done; \
	else \
		echo -e "$(COLOR_YELLOW)⚠ fswatch or inotifywait not found$(COLOR_RESET)"; \
		echo "  macOS: brew install fswatch"; \
		echo "  Linux: apt install inotify-tools"; \
		exit 1; \
	fi

# ==================================================================================== #
# INFORMATION TARGETS
# ==================================================================================== #

##@ Cleanup

clean:  ## Remove built binaries and test artifacts
	$(Q)echo -e "$(COLOR_YELLOW)→ Cleaning build artifacts...$(COLOR_RESET)"
	$(Q)rm -f $(VOICE_BIN) $(LOGGER_BIN) $(GIT_BIN) $(CHAIN_GUARD_BIN) $(DOC_GUARD_BIN) $(ITERATE_RUN_BIN)
	$(Q)rm -f coverage.out coverage.html
	$(Q)rm -rf $(SRC_DIR)/vendor
	$(Q)echo -e "$(COLOR_GREEN)✓ Clean complete$(COLOR_RESET)"
