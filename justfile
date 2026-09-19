# Root Justfile for agent-status

# Default recipe: list available tasks
default:
    @just --list

# Run in human-facing interactive mode
run *args:
    @echo "Specify component: just run-observer, just run-ingester, just run-server, or just run-ui"

# Run in agent-facing token-conservative mode
run-ai *args:
    @AGENT=1 just run {{args}}

# Build all Go binaries and bundle ui-web
build-all:
    @mkdir -p packages/agent-observer/bin packages/agent-ingester/bin packages/agent-server/bin
    cd packages/agent-observer && just build
    cd packages/agent-ingester && just build
    cd packages/agent-server && just build
    cd packages/ui-web && just build

# Run all test suites across Go and Bun
test-all:
    cd packages/agent-observer && just test
    cd packages/agent-ingester && just test
    cd packages/agent-server && just test
    cd packages/ui-web && just test

# Run tests with short output / test summary
test:
    @just test-all

# Lint all packages (Go vet + golangci-lint, Bun typecheck + linter)
lint-all:
    cd packages/agent-observer && just lint
    cd packages/agent-ingester && just lint
    cd packages/agent-server && just lint
    cd packages/ui-web && just lint

# Typecheck and lint
lint:
    @just lint-all

# Clean all build and test artifacts
clean:
    rm -rf packages/*/bin packages/ui-web/dist .tmp/test-*
