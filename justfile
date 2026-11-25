mod common 'packages/agent-common/justfile'
mod debug 'packages/agent-debug/justfile'
mod ingester 'packages/agent-ingester/justfile'
mod observer 'packages/agent-observer/justfile'
mod server 'packages/agent-server/justfile'
mod store 'packages/agent-status-store/justfile'
mod ui 'packages/ui-web/justfile'

default:
    @just --list --list-submodules

run *args:
    devhost caddy start
    devhost start --detach {{ args }}

start *args:
    @just run {{ args }}

stop:
    devhost stop

status:
    devhost stack find "{{ justfile_directory() }}/devhost.toml"

logs:
    @tail -f .tmp/dev-stack/logs/*.log

run-ai *args:
    AGENT=1 just run {{ args }}

build-daemons:
    just observer build
    just ingester build
    just server build

build:
    just common build
    just debug build
    just ingester build
    just observer build
    just server build
    just store build
    just ui build

test:
    just common test
    just debug test
    just ingester test
    just observer test
    just server test
    just store test
    just ui test

lint:
    just common lint
    just debug lint
    just ingester lint
    just observer lint
    just server lint
    just store lint
    just ui lint

coverage:
    just common coverage
    just debug coverage
    just ingester coverage
    just observer coverage
    just server coverage
    just store coverage

check:
    just common check
    just debug check
    just ingester check
    just observer check
    just server check
    just store check
    just ui lint
    just ui test
