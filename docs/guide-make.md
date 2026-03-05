# Makefile Guide

This guide covers the 2-layer Makefile system. Part 1 describes universal principles applicable to any project. Part 2 is a template to customize for your specific application.

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

# Part 2: Project-Specific Template

This section is a template. Update it to customize it for your specific project.

---

## Target Reference

<!-- Replace <appname> with your binary name -->

| Category | Target | Description |
|----------|--------|-------------|
| **General** | `help` | Display organized help message |
| | `prereqs` | Check/install prerequisites (OS-aware) |
| **Build** | `build` | Build binary for local platform (debug mode) |
| | `build-production` | Cross-compile release binary (stripped) |
| | `build-all` | Build for all target platforms |
| **Test** | `test` | Run all tests |
| | `test-unit` | Unit tests only (fast, no external deps) |
| | `test-integration` | Integration tests |
| | `test-coverage` | Tests with coverage report |
| | `test-pkg` | Run tests for specific package |
| **Quality** | `lint` | Run linter |
| | `fmt` | Format code |
| | `check` | Run lint + test together |
| **Install** | `install` | Alias for install-dev |
| | `install-dev` | Build and install as symlinks (development mode) |
| | `install-production` | Build and install as copies (production mode) |
| | `uninstall` | Remove all installed files |
| **Development** | `dev` | fmt → test → build workflow |
| | `cycle` | uninstall → clean → build → install |
| | `run` | Run development version |
| **Cleanup** | `clean` | Remove build artifacts |
| | `clean-all` | Deep clean including caches/vendor |

---

## Install Modes

Two install modes serve different purposes. `make install` is an alias for `make install-dev`.

### `make install-dev` (Development)

Creates **symlinks** pointing to binaries in the build output directory:

```
~/.local/bin/<appname> → <project>/bin/<appname>-<os>-<arch>
```

**Benefits:**
- Rebuild without reinstall (`make build` updates the binary, symlink still works)
- Only re-run `make install-dev` when adding new outputs
- Easy to identify: `ls -la ~/.local/bin/<appname>` shows it's a symlink

**When to use:** Daily development, testing local changes.

### `make install-production` (Production)

**Copies** actual binaries into place, overwriting any symlinks:

```
~/.local/bin/<appname>    (actual file, not symlink)
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

---

## Delegated to CLI (Runtime Operations)

These operations belong in the application CLI, not the Makefile:

```bash
# Example runtime commands (customize for your app)
<appname> version                 # Show version info
<appname> config show             # Display configuration
<appname> daemon start            # Start background service (if applicable)
<appname> daemon stop             # Stop background service
```

---

## makehelp.sh Functions

| Function | Purpose |
|----------|---------|
| `cmd_prereqs` | OS-aware prerequisite checking |
| `cmd_build_production` | Cross-platform production builds with LDFLAGS |
| `cmd_install_completions` | Shell completion installation (bash, zsh, fish) |
| `cmd_uninstall_completions` | Shell completion removal |

---

## Common Workflows

### First-time setup
```bash
make prereqs          # Check environment
make build            # Build debug binary
make install          # Install to ~/.local/bin (symlinks)
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
```

### Release build
```bash
make build-production # Cross-compile for release
```
