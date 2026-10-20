mod ui 'packages/ui-web/justfile'

# Root Justfile for agent-status

# Default recipe: list available tasks
default:
    @just --list

# Start the full agent-status stack (ingester, server, observer, web UI)
start *args:
    bun run scripts/dev-stack.ts start {{ args }}

# Stop the full agent-status stack and free ports
stop:
    bun run scripts/dev-stack.ts stop

# Check status of running dev stack
status:
    bun run scripts/dev-stack.ts status

# Tail rotating dev stack daemon logs on disk
logs:
    @tail -f .tmp/dev-stack/logs/*.log

# Run in human-facing interactive mode
run *args:
    @just start {{ args }}

# Run in agent-facing token-conservative mode
run-ai *args:
    AGENT=1 just start

# Run air-debug CLI utility via go run
debug *args:
    go run ./packages/agent-debug/cmd/air-debug {{ args }}

# Alias for debug
run-debug *args:
    @just debug {{ args }}

# Run print-session CLI utility via go run
print-session *args:
    go run github.com/alexgorbatchev/agent-print-session-cli/cmd/print-session@v1.0.0 {{ args }}

# Build only dev stack binaries and bundle ui-web (omitting agent-debug)
build-stack:
    @mkdir -p packages/agent-observer/bin packages/agent-ingester/bin packages/agent-server/bin
    cd packages/agent-observer && just build
    cd packages/agent-ingester && just build
    cd packages/agent-server && just build
    cd packages/ui-web && just build

# Build all Go binaries and bundle ui-web
build-all:
    @mkdir -p packages/agent-observer/bin packages/agent-ingester/bin packages/agent-server/bin packages/agent-debug/bin
    cd packages/agent-observer && just build
    cd packages/agent-ingester && just build
    cd packages/agent-server && just build
    cd packages/agent-debug && just build
    cd packages/ui-web && just build

# Run all test suites across Go and Bun
test-all:
    cd packages/agent-common && just test
    cd packages/agent-observer && just test
    cd packages/agent-ingester && just test
    cd packages/agent-server && just test
    cd packages/agent-debug && just test
    cd packages/ui-web && just test

# Verify statement coverage meets >= 90% threshold across all Go packages
check-coverage:
    cd packages/agent-common && just test
    cd packages/agent-observer && just test
    cd packages/agent-ingester && just test
    cd packages/agent-server && just test
    cd packages/agent-debug && just test

# Run Playwright E2E test suite
test-e2e:
    cd packages/ui-web && just test-e2e

# Start Storybook component explorer for ui-web
storybook-web *args:
    cd packages/ui-web && just storybook {{ args }}

# Run tests with short output / test summary
test:
    @just test-all

# Typecheck dev stack scripts
lint-scripts:
    bun x tsc -p scripts/tsconfig.json

# Lint all packages (Go vet + golangci-lint, Bun typecheck + linter)
lint-all:
    @out="$(gofmt -l packages/)"; if [ -n "$out" ]; then echo "Unformatted Go files:"; echo "$out"; exit 1; fi
    cd packages/agent-common && just lint
    cd packages/agent-observer && just lint
    cd packages/agent-ingester && just lint
    cd packages/agent-server && just lint
    cd packages/agent-debug && just lint
    cd packages/ui-web && just lint
    just lint-scripts

# Typecheck and lint
lint:
    @just lint-all

# Clean all build and test artifacts
clean:
    @just stop
    rm -rf packages/*/bin packages/ui-web/dist .tmp/test-* .tmp/dev-stack

# Replay a Claude Code session by session ID or file path
session-replay-claude session_id *args:
    just debug session replay --harness claude-code {{ session_id }} {{ args }}

# Alias for session-replay-claude
session-replay-claude-code session_id *args:
    just session-replay-claude {{ session_id }} {{ args }}

# Replay a Pi session by session ID or file path
session-replay-pi session_id *args:
    just debug session replay --harness pi {{ session_id }} {{ args }}

# Replay an OpenAI Codex session by session ID or file path
session-replay-codex session_id *args:
    just debug session replay --harness codex {{ session_id }} {{ args }}

# Replay an OpenCode session by session ID or database path
session-replay-opencode session_id *args:
    just debug session replay --harness opencode {{ session_id }} {{ args }}

# Prepare dependencies for local development.
setup:
    go mod download
    just ui setup
