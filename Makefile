# Makefile for claudecodetricks
# Robust build system with versioning, testing, and installation

.DEFAULT_GOAL := build
SHELL := /bin/bash

# ==================================================================================== #
# CONFIGURATION
# ==================================================================================== #

# Binary output directory
BIN_DIR := plugins/session-hooks/hooks

# Binary names
VOICE_BIN := $(BIN_DIR)/voice-announcer
LOGGER_BIN := $(BIN_DIR)/session-logger
GIT_BIN := $(BIN_DIR)/git-committer

# Source directories
SRC_DIR := src
CMD_DIR := $(SRC_DIR)/cmd
INTERNAL_DIR := $(SRC_DIR)/internal
PKG_DIR := $(SRC_DIR)/pkg

# Installation directories
INSTALL_DIR := $(HOME)/.claude/hooks
LOG_DIR := $(HOME)/.claude/log

# Go configuration
GO := go
GOFMT := gofmt
GOVET := $(GO) vet
GOTEST := $(GO) test
GOLINT := golangci-lint
GO_FILES := $(shell find $(SRC_DIR) -name '*.go' -type f)
GO_PACKAGES := $(shell cd $(SRC_DIR) && $(GO) list ./...)

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
# PHONY TARGETS
# ==================================================================================== #

.PHONY: all build clean test test-integration test-unit test-hook
.PHONY: deps deps-check deps-update
.PHONY: fmt fmt-check vet lint check
.PHONY: install uninstall
.PHONY: coverage coverage-html
.PHONY: watch
.PHONY: version info
.PHONY: help

# ==================================================================================== #
# BUILD TARGETS
# ==================================================================================== #

## all: Build all binaries (default target)
all: check-tools build

## build: Build all binaries
build: $(VOICE_BIN) $(LOGGER_BIN) $(GIT_BIN)
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

## clean: Remove built binaries and test artifacts
clean:
	$(Q)echo -e "$(COLOR_YELLOW)→ Cleaning build artifacts...$(COLOR_RESET)"
	$(Q)rm -f $(VOICE_BIN) $(LOGGER_BIN) $(GIT_BIN)
	$(Q)rm -f coverage.out coverage.html
	$(Q)rm -rf $(SRC_DIR)/vendor
	$(Q)echo -e "$(COLOR_GREEN)✓ Clean complete$(COLOR_RESET)"

## rebuild: Clean and rebuild all binaries
rebuild: clean build

# ==================================================================================== #
# TESTING TARGETS
# ==================================================================================== #

## test: Run all tests
test: test-unit test-hook
	$(Q)echo -e "$(COLOR_GREEN)✓ All tests passed$(COLOR_RESET)"

## test-unit: Run Go unit tests
test-unit: check-tools
	$(Q)echo -e "$(COLOR_BLUE)→ Running unit tests...$(COLOR_RESET)"
	$(Q)cd $(SRC_DIR) && $(GOTEST) $(VERBOSE) -race -timeout 30s ./...

## test-hook: Test hooks with sample JSON input
test-hook: build
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

## coverage: Generate test coverage report
coverage: check-tools
	$(Q)echo -e "$(COLOR_BLUE)→ Generating coverage report...$(COLOR_RESET)"
	$(Q)cd $(SRC_DIR) && $(GOTEST) -coverprofile=../coverage.out -covermode=atomic ./...
	$(Q)cd $(SRC_DIR) && $(GO) tool cover -func=../coverage.out
	$(Q)echo -e "$(COLOR_GREEN)✓ Coverage report: coverage.out$(COLOR_RESET)"

## coverage-html: Generate HTML coverage report
coverage-html: coverage
	$(Q)echo -e "$(COLOR_BLUE)→ Generating HTML coverage report...$(COLOR_RESET)"
	$(Q)cd $(SRC_DIR) && $(GO) tool cover -html=../coverage.out -o ../coverage.html
	$(Q)echo -e "$(COLOR_GREEN)✓ HTML coverage report: coverage.html$(COLOR_RESET)"
	$(Q)which open >/dev/null && open coverage.html || true

# ==================================================================================== #
# QUALITY TARGETS
# ==================================================================================== #

## fmt: Format all Go source files
fmt: check-tools
	$(Q)echo -e "$(COLOR_BLUE)→ Formatting Go files...$(COLOR_RESET)"
	$(Q)$(GOFMT) -w -s $(GO_FILES)
	$(Q)echo -e "$(COLOR_GREEN)✓ Formatting complete$(COLOR_RESET)"

## fmt-check: Check if Go files are formatted
fmt-check: check-tools
	$(Q)echo -e "$(COLOR_BLUE)→ Checking Go formatting...$(COLOR_RESET)"
	$(Q)UNFORMATTED=$$($(GOFMT) -l $(GO_FILES)); \
	if [ -n "$$UNFORMATTED" ]; then \
		echo -e "$(COLOR_YELLOW)⚠ Unformatted files:$(COLOR_RESET)"; \
		echo "$$UNFORMATTED"; \
		exit 1; \
	fi
	$(Q)echo -e "$(COLOR_GREEN)✓ All files are formatted$(COLOR_RESET)"

## vet: Run go vet on all packages
vet: check-tools
	$(Q)echo -e "$(COLOR_BLUE)→ Running go vet...$(COLOR_RESET)"
	$(Q)cd $(SRC_DIR) && $(GOVET) ./...
	$(Q)echo -e "$(COLOR_GREEN)✓ go vet passed$(COLOR_RESET)"

## lint: Run golangci-lint (if available)
lint:
	$(Q)if command -v $(GOLINT) >/dev/null 2>&1; then \
		echo -e "$(COLOR_BLUE)→ Running golangci-lint...$(COLOR_RESET)"; \
		cd $(SRC_DIR) && $(GOLINT) run $(VERBOSE) ./...; \
		echo -e "$(COLOR_GREEN)✓ Linting complete$(COLOR_RESET)"; \
	else \
		echo -e "$(COLOR_YELLOW)⚠ golangci-lint not installed, skipping$(COLOR_RESET)"; \
		echo "  Install: curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b \$$(go env GOPATH)/bin"; \
	fi

## check: Run all quality checks (fmt-check, vet, test)
check: fmt-check vet test-unit
	$(Q)echo -e "$(COLOR_GREEN)✓ All checks passed$(COLOR_RESET)"

# ==================================================================================== #
# DEPENDENCY TARGETS
# ==================================================================================== #

## deps: Install and tidy Go dependencies
deps:
	$(Q)echo -e "$(COLOR_BLUE)→ Installing dependencies...$(COLOR_RESET)"
	$(Q)cd $(SRC_DIR) && $(GO) mod download
	$(Q)cd $(SRC_DIR) && $(GO) mod tidy
	$(Q)echo -e "$(COLOR_GREEN)✓ Dependencies installed$(COLOR_RESET)"

## deps-check: Verify dependencies
deps-check:
	$(Q)echo -e "$(COLOR_BLUE)→ Verifying dependencies...$(COLOR_RESET)"
	$(Q)cd $(SRC_DIR) && $(GO) mod verify
	$(Q)echo -e "$(COLOR_GREEN)✓ Dependencies verified$(COLOR_RESET)"

## deps-update: Update all dependencies
deps-update:
	$(Q)echo -e "$(COLOR_BLUE)→ Updating dependencies...$(COLOR_RESET)"
	$(Q)cd $(SRC_DIR) && $(GO) get -u ./...
	$(Q)cd $(SRC_DIR) && $(GO) mod tidy
	$(Q)echo -e "$(COLOR_GREEN)✓ Dependencies updated$(COLOR_RESET)"

# ==================================================================================== #
# INSTALLATION TARGETS
# ==================================================================================== #

## install: Install hooks to Claude Code directory
install: build
	$(Q)echo -e "$(COLOR_BLUE)→ Installing hooks...$(COLOR_RESET)"
	$(Q)mkdir -p $(INSTALL_DIR)
	$(Q)mkdir -p $(LOG_DIR)
	$(Q)cp -f $(VOICE_BIN) $(INSTALL_DIR)/
	$(Q)cp -f $(LOGGER_BIN) $(INSTALL_DIR)/
	$(Q)cp -f $(GIT_BIN) $(INSTALL_DIR)/
	$(Q)chmod +x $(INSTALL_DIR)/voice-announcer
	$(Q)chmod +x $(INSTALL_DIR)/session-logger
	$(Q)chmod +x $(INSTALL_DIR)/git-committer
	$(Q)echo -e "$(COLOR_GREEN)✓ Hooks installed to $(INSTALL_DIR)$(COLOR_RESET)"
	$(Q)echo -e "$(COLOR_CYAN)  Note: Update ~/.claude/settings.json to enable hooks$(COLOR_RESET)"

## uninstall: Remove installed hooks
uninstall:
	$(Q)echo -e "$(COLOR_YELLOW)→ Uninstalling hooks...$(COLOR_RESET)"
	$(Q)rm -f $(INSTALL_DIR)/voice-announcer
	$(Q)rm -f $(INSTALL_DIR)/session-logger
	$(Q)rm -f $(INSTALL_DIR)/git-committer
	$(Q)echo -e "$(COLOR_GREEN)✓ Hooks uninstalled$(COLOR_RESET)"

# ==================================================================================== #
# DEVELOPMENT TARGETS
# ==================================================================================== #

## watch: Watch for changes and rebuild (requires fswatch or inotifywait)
watch:
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

## version: Display version information
version:
	$(Q)echo -e "$(COLOR_CYAN)Version:    $(VERSION)$(COLOR_RESET)"
	$(Q)echo -e "$(COLOR_CYAN)Commit:     $(GIT_COMMIT)$(COLOR_RESET)"
	$(Q)echo -e "$(COLOR_CYAN)Build Time: $(BUILD_TIME)$(COLOR_RESET)"

## info: Display build configuration
info:
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
	$(Q)echo ""
	$(Q)echo -e "$(COLOR_BOLD)Build Status:$(COLOR_RESET)"
	$(Q)for bin in $(VOICE_BIN) $(LOGGER_BIN) $(GIT_BIN); do \
		if [ -f $$bin ]; then \
			echo -e "  $(COLOR_GREEN)✓$(COLOR_RESET) $$bin ($$(du -h $$bin | cut -f1))"; \
		else \
			echo -e "  $(COLOR_YELLOW)✗$(COLOR_RESET) $$bin (not built)"; \
		fi \
	done

# ==================================================================================== #
# UTILITY TARGETS
# ==================================================================================== #

## check-tools: Verify required tools are installed
check-tools:
	$(Q)if ! command -v $(GO) >/dev/null 2>&1; then \
		echo -e "$(COLOR_YELLOW)⚠ Go not found. Install from https://golang.org/$(COLOR_RESET)"; \
		exit 1; \
	fi

## help: Display this help message
help:
	$(Q)echo -e "$(COLOR_BOLD)claudecodetricks - Makefile Help$(COLOR_RESET)"
	$(Q)echo ""
	$(Q)echo -e "$(COLOR_BOLD)USAGE:$(COLOR_RESET)"
	$(Q)echo "  make [target] [V=1]"
	$(Q)echo ""
	$(Q)echo -e "$(COLOR_BOLD)BUILD TARGETS:$(COLOR_RESET)"
	$(Q)sed -n 's/^##//p' $(MAKEFILE_LIST) | grep -E "(all|build|clean|rebuild)" | column -t -s ':' | sed -e 's/^/  /'
	$(Q)echo ""
	$(Q)echo -e "$(COLOR_BOLD)TESTING TARGETS:$(COLOR_RESET)"
	$(Q)sed -n 's/^##//p' $(MAKEFILE_LIST) | grep -E "(test|coverage)" | column -t -s ':' | sed -e 's/^/  /'
	$(Q)echo ""
	$(Q)echo -e "$(COLOR_BOLD)QUALITY TARGETS:$(COLOR_RESET)"
	$(Q)sed -n 's/^##//p' $(MAKEFILE_LIST) | grep -E "(fmt|vet|lint|check)" | column -t -s ':' | sed -e 's/^/  /'
	$(Q)echo ""
	$(Q)echo -e "$(COLOR_BOLD)DEPENDENCY TARGETS:$(COLOR_RESET)"
	$(Q)sed -n 's/^##//p' $(MAKEFILE_LIST) | grep -E "deps" | column -t -s ':' | sed -e 's/^/  /'
	$(Q)echo ""
	$(Q)echo -e "$(COLOR_BOLD)INSTALLATION TARGETS:$(COLOR_RESET)"
	$(Q)sed -n 's/^##//p' $(MAKEFILE_LIST) | grep -E "(install|uninstall)" | column -t -s ':' | sed -e 's/^/  /'
	$(Q)echo ""
	$(Q)echo -e "$(COLOR_BOLD)DEVELOPMENT TARGETS:$(COLOR_RESET)"
	$(Q)sed -n 's/^##//p' $(MAKEFILE_LIST) | grep -E "watch" | column -t -s ':' | sed -e 's/^/  /'
	$(Q)echo ""
	$(Q)echo -e "$(COLOR_BOLD)INFORMATION TARGETS:$(COLOR_RESET)"
	$(Q)sed -n 's/^##//p' $(MAKEFILE_LIST) | grep -E "(version|info|help)" | column -t -s ':' | sed -e 's/^/  /'
	$(Q)echo ""
	$(Q)echo -e "$(COLOR_BOLD)FLAGS:$(COLOR_RESET)"
	$(Q)echo "  V=1          Verbose output (show commands)"
	$(Q)echo ""
	$(Q)echo -e "$(COLOR_BOLD)EXAMPLES:$(COLOR_RESET)"
	$(Q)echo "  make build           Build all binaries"
	$(Q)echo "  make test            Run all tests"
	$(Q)echo "  make check           Run quality checks"
	$(Q)echo "  make install         Install hooks to ~/.claude/hooks/"
	$(Q)echo "  make V=1 build       Build with verbose output"
	$(Q)echo ""
