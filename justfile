# Root Justfile for agent-status

# Default recipe: list available tasks
default:
    @just --list

# Start the full agent-status stack (ingester, server, observer, web UI)
start *args:
    bun run scripts/dev-stack.ts start {{args}}

# Stop the full agent-status stack and free ports
stop:
    bun run scripts/dev-stack.ts stop

# Check status of running dev stack
status:
    bun run scripts/dev-stack.ts status

# Tail rotating daemon logs on disk
logs:
    @tail -f ~/.local/state/agent-status/logs/*.log

# Run in human-facing interactive mode
run *args:
    @just start

# Run in agent-facing token-conservative mode
run-ai *args:
    AGENT=1 just start

# Build all Go binaries and bundle ui-web
build-all:
    @mkdir -p packages/agent-observer/bin packages/agent-ingester/bin packages/agent-server/bin
    cd packages/agent-parser && just test
    cd packages/agent-observer && just build
    cd packages/agent-ingester && just build
    cd packages/agent-server && just build
    cd packages/ui-web && just build

# Run all test suites across Go and Bun
test-all:
    cd packages/agent-common && just test
    cd packages/agent-parser && just test
    cd packages/agent-observer && just test
    cd packages/agent-ingester && just test
    cd packages/agent-server && just test
    cd packages/ui-web && just test

# Run Playwright E2E test suite
test-e2e:
    cd packages/ui-web && just test-e2e

# Run tests with short output / test summary
test:
    @just test-all

# Lint all packages (Go vet + golangci-lint, Bun typecheck + linter)
lint-all:
    cd packages/agent-common && just lint
    cd packages/agent-parser && just lint
    cd packages/agent-observer && just lint
    cd packages/agent-ingester && just lint
    cd packages/agent-server && just lint
    cd packages/ui-web && just lint

# Typecheck and lint
lint:
    @just lint-all

# Clean all build and test artifacts
clean:
    @just stop
    rm -rf packages/*/bin packages/ui-web/dist .tmp/test-* .tmp/dev-stack
