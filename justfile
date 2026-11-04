# Workspace orchestration for agent-status
mod common 'packages/agent-common/justfile'
mod debug 'packages/agent-debug/justfile'
mod ingester 'packages/agent-ingester/justfile'
mod observer 'packages/agent-observer/justfile'
mod server 'packages/agent-server/justfile'
mod ui 'packages/ui-web/justfile'

# List workspace and package tasks
default:
    @just --list --list-submodules

# Start the devhost-managed stack and shared HTTPS proxy
run *args:
    devhost caddy start
    devhost start {{ args }}

# Start the full stack
start *args:
    @just run {{ args }}

# Stop this devhost stack
stop:
    devhost stop

# Show whether this checkout's devhost stack is running
status:
    devhost stack find "{{ justfile_directory() }}/devhost.toml"

# Tail rotating daemon logs on disk
logs:
    @tail -f .tmp/dev-stack/logs/*.log

# Run with token-conservative output
run-ai *args:
    AGENT=1 just run {{ args }}

# Build only dev stack binaries and bundle ui-web (omitting agent-debug)
build-stack:
    just build-daemons
    just ui build

# Build the observer, ingester, and server binaries the E2E rig spawns
build-daemons:
    just observer build
    just ingester build
    just server build

# Build all Go binaries and bundle ui-web
build-all:
    just build-stack
    just debug build

# Run all test suites across Go and Bun
test-all:
    just check-coverage
    just ui test

# Verify statement coverage meets >= 90% threshold across all Go packages
check-coverage:
    just common test
    just observer test
    just ingester test
    just server test
    just debug test

# Run tests with short output / test summary
test:
    @just test-all

# Lint all packages (Go vet + golangci-lint, Bun typecheck + linter)
lint-all:
    just common lint
    just observer lint
    just ingester lint
    just server lint
    just debug lint
    just ui lint

# Typecheck and lint
lint:
    @just lint-all

# Clean all build and test artifacts
clean:
    @just stop
    just common clean
    just observer clean
    just ingester clean
    just server clean
    just debug clean
    just ui clean
    rm -rf .tmp/test-* .tmp/dev-stack

# Prepare dependencies for local development
setup:
    just common setup
    just observer setup
    just ingester setup
    just server setup
    just debug setup
    just ui setup
