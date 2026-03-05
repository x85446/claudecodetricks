# Makefile Guide

This guide covers the 2-layer Makefile system used in this project. Part 1 describes universal principles applicable to any project. Part 2 applies these principles to Warden specifically.

---

# Part 1: Universal Principles

## Philosophy

The Makefile serves as the **single interface** for all development operations. Developers should never need to remember raw tool commands—`make` handles everything.

### The 2-Layer System

```
┌─────────────────────────────────────┐
│           Makefile                  │  ← Declarative interface
│  (targets, dependencies, help)      │     What to do
├─────────────────────────────────────┤
│          makehelp.sh                │  ← Imperative logic
│  (complex scripts, OS branching)    │     How to do it
└─────────────────────────────────────┘
```

- **Makefile**: Defines targets, dependencies, and short commands. Readable, scannable.
- **makehelp.sh**: Contains complex shell logic extracted from the Makefile. Testable, maintainable.

### Default Target = Help

Running `make` with no arguments should always display help:

```makefile
.DEFAULT_GOAL := help
```

## Mantra

Three rules govern what goes where:

1. **"If it's a build-time operation, it's in make"**
   - Compiling, testing, linting, formatting, installing dev tools

2. **"If it's a runtime operation, it's in the CLI"**
   - Daemon management, service lifecycle, version queries, user-facing commands

3. **"Complex shell logic belongs in makehelp.sh, not inline"**
   - Anything over ~10 lines, OS-specific branching, multi-step operations

## Separation of Concerns

| Belongs in Makefile | Belongs in CLI |
|---------------------|----------------|
| Build / compile | Daemon management |
| Test execution | Runtime configuration |
| Code quality (lint, fmt) | Version / info queries |
| Dependency management | Service lifecycle |
| Install (initial setup) | User-facing operations |
| Development workflow | |
| Cleanup | |

## Standard Target Categories

Every project should have these categories:

| Category | Targets | Purpose |
|----------|---------|---------|
| **General** | `help`, `prereqs` | Entry points, environment setup |
| **Build** | `build`, `build-production` | Compile binaries |
| **Test** | `test`, `test-unit`, `test-integration`, `test-coverage`, `test-pkg` | All test workflows |
| **Quality** | `lint`, `fmt`, `check` | Code standards enforcement |
| **Install** | `install`, `install-dev`, `install-production`, `uninstall` | Binary installation (dev vs prod modes) |
| **Development** | `dev`, `cycle`, `run`, `watch` | Developer workflow helpers |
| **Cleanup** | `clean`, `clean-all` | Remove artifacts |


### General Pattern
always includes, but not limited to:
- help: displays all targets and categories
- prereqs: displays OS-specific prerequisites and or installs them

### Build Pattern
always includes, but not limited to:
- build: cross-compiles for local platform
- build-all: cross-compiles for all platforms
- build-production: cross-compiles for production platform

### Test Pattern
always includes, but not limited to:
- test: runs all tests
- test-unit: runs unit tests
- test-integration: runs integration tests

### Quality Pattern
always includes, but not limited to:
- lint: runs linter
- fmt: runs formatter
- check: runs linter and tests
  

### Install Pattern
always includes, but not limited to:
- install: alias for install-dev (common shorthand)
- install-dev: follows guide-install.md for developer install (symlinks to build output)
- install-production: follows guide-install.md for production install (copies binaries)
- uninstall: removes installed binaries and symlinks

### Development Pattern
always includes, but not limited to:
- dev: tbd
- cycle: tbd
- run: tbd
- watch: tbd

### Cleanup Pattern
always includes, but not limited to:
- clean: removes build artifacts

Projects should support two install modes:
- **`install`**: Development mode—creates symlinks to build output (rebuild without reinstall)
- **`install-production`**: Production mode—copies actual binaries (self-contained)

## Conventions

### Help Annotations

```makefile
##@ Section Name           # Creates a section header in help output

.PHONY: target-name
target-name:  ## Short description    # Appears in help under that section
	@command
```

### Naming

- **Lowercase with hyphens**: `test-unit`, `build-production`
- **Not**: `testUnit`, `build_production`, `TEST`

### Target Declarations

```makefile
.PHONY: target-name        # Always declare non-file targets as PHONY
target-name: dependency    # Dependencies come after the colon
	@command               # @ suppresses command echo (clean output)
```

### Variables at the Top

```makefile
# Tools
GO := go
LINT := golangci-lint

# Paths
BINARY_DIR := ./bin
COVERAGE_FILE := coverage.out

# Derived
VERSION := $(shell git describe --tags 2>/dev/null || echo "0.0.0-dev")
```

## When to Use makehelp.sh

Extract logic to `makehelp.sh` when:

- **More than ~10 lines** of shell in a single target
- **OS-specific branching** (Darwin vs Linux vs Windows)
- **Multi-step operations** with user prompts or confirmations
- **Reusable logic** needed by multiple targets
- **Complex string manipulation** or loops

### Pattern

**Makefile:**
```makefile
.PHONY: prereqs
prereqs:  ## Check and install prerequisites
	@./makehelp.sh prereqs
```

**makehelp.sh:**
```bash
cmd_prereqs() {
    # Complex logic here
    check_go
    check_docker
    # ...
}

case "$1" in
    prereqs) cmd_prereqs ;;
    *) echo "Unknown command: $1" ;;
esac
```

## Adding New Targets

Template for new targets:

```makefile
##@ Category Name

.PHONY: new-target
new-target: dependency  ## What this target does
	@command here
```

For complex targets:

```makefile
.PHONY: complex-target
complex-target:  ## What this target does
	@./makehelp.sh complex-target $(ARGS)
```

---

# Part 2: Warden Application

## Target Reference

| Category | Target | Description |
|----------|--------|-------------|
| **General** | `help` | Display organized help message |
| | `prereqs` | Check/install prerequisites (OS-aware) |
| **Build** | `build` | Build CLI + hostserver (debug mode) |
| | `build-production` | Cross-compile release binaries (stripped) |
| | `build-daemons` | Build hostserver + cellserver for all platforms |
| | `proto` | Generate Go code from protobuf definitions |
| **Test** | `test` | Run all tests (unit + integration + e2e) |
| | `test-unit` | Unit tests only (fast, no external deps) |
| | `test-race` | Unit tests with race detector |
| | `test-integration` | Integration tests (requires infrastructure) |
| | `test-docker` | Docker provider tests |
| | `test-e2e` | End-to-end workflow tests |
| | `test-coverage` | Tests with coverage report |
| | `test-coverage-html` | Generate HTML coverage report |
| | `test-pkg` | Run tests for specific package |
| **Quality** | `lint` | Run golangci-lint |
| | `fmt` | Format code with gofmt/goimports |
| | `check` | Run lint + test together |
| **Install** | `install` | Alias for install-dev |
| | `install-dev` | Build and install as symlinks (development mode) |
| | `install-production` | Build and install as copies (production mode) |
| | `uninstall` | Remove all installed files |
| **Development** | `dev` | fmt → test → build workflow |
| | `cycle` | uninstall → clean → build → install |
| | `run` | Run development version |
| | `watch` | Watch for changes and rebuild |
| **Cleanup** | `clean` | Remove build artifacts |
| | `clean-all` | Deep clean including vendor |

## Install Modes

Two install modes serve different purposes. `make install` is an alias for `make install-dev`.

### `make install-dev` (Development)

Creates **symlinks** pointing to binaries in the build output directory:

```
~/.local/bin/warden → <project>/bin/warden-darwin-arm64
```

**Benefits:**
- Rebuild without reinstall (`make build` updates the binary, symlink still works)
- Only re-run `make install-dev` when adding new outputs (new binary, new completions)
- Easy to identify: `ls -la ~/.local/bin/warden` shows it's a symlink

**When to use:** Daily development, testing local changes.

### `make install-production` (Production)

**Copies** actual binaries into place, overwriting any symlinks:

```
~/.local/bin/warden    (actual file, not symlink)
```

**Benefits:**
- Self-contained installation
- Works after source directory is removed
- What end users get

**When to use:** Testing production behavior, preparing releases, installing on non-dev machines.

### Quick Reference

| Scenario | Command |
|----------|---------|
| First-time dev setup | `make install` |
| After adding new binary | `make install` (re-run to create new symlink) |
| After code changes | `make build` (no reinstall needed) |
| Testing production behavior | `make install-production` |
| Return to dev mode | `make install` (recreates symlinks) |

See [guide-install.md](guide-install.md) for full details on file locations and layout.

## Delegated to `warden` CLI

These operations belong in the CLI, not the Makefile:

```bash
# Daemon lifecycle
warden daemon status              # Show daemon status
warden daemon start               # Start daemon
warden daemon stop                # Stop daemon
warden daemon restart             # Restart daemon
warden daemon logs                # View logs
warden daemon logs -f             # Follow logs
warden daemon install             # Register as system service
warden daemon uninstall           # Remove system service

# Information
warden version                    # Show version info
```

## makehelp.sh Functions

| Function | Purpose |
|----------|---------|
| `cmd_prereqs` | OS-aware prerequisite checking (Go, Docker, lint tools) |
| `cmd_build_production` | Cross-platform production builds with proper LDFLAGS |
| `cmd_build_daemons` | Multi-platform daemon builds (CGO handling for clipboard) |
| `cmd_install_completions` | Shell completion installation (bash, zsh, fish) |
| `cmd_uninstall_completions` | Shell completion removal |
| `cmd_install_cellserver` | Install cellserver payloads to share directory |
| `cmd_install_snippets` | Install snippet templates to share directory |

## Common Workflows

### First-time setup
```bash
make prereqs          # Check environment
make build            # Build debug binaries
make install          # Install to ~/.local/bin (symlinks)
warden daemon install # Register hostserver service
warden daemon start   # Start hostserver
```

### Daily development
```bash
make dev              # Format, test, build
# or
make cycle            # Full clean rebuild + install
```

### Running tests
```bash
make test             # All tests
make test-unit        # Fast unit tests only
make test-pkg PKG=internal/config  # Single package
make test-docker      # Docker integration tests
```

### Release build
```bash
make build-production # Cross-compile for all platforms
```
