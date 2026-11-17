#!/bin/sh
set -eu

stack_dir="$PWD/.tmp/dev-stack"
notify_sock="$stack_dir/run/agent-status/notify.sock"
if [ "$(printf '%s' "$notify_sock" | wc -c)" -gt 103 ]; then
    printf '%s\n' 'action=dev_service_failed error=notify_socket_path_exceeds_103_bytes' >&2
    exit 1
fi
mkdir -p "$stack_dir/data" "$stack_dir/cache" "$stack_dir/logs" "$stack_dir/run"

unset AGENT_LOG_FILE AGENT_SESSION_STATE_PATH
export AGENT_LOG_DIR="$stack_dir/logs"
export AGENT_STATUS_TOKEN=dev-secret-token

case "$DEVHOST_SERVICE_NAME" in
    ingester)
        export XDG_DATA_HOME="$stack_dir/data"
        export XDG_RUNTIME_DIR="$stack_dir/run"
        export AGENT_BEARER_TOKEN=dev-secret-token
        exec ./packages/agent-ingester/bin/agent-ingester \
            --addr "$DEVHOST_BIND_HOST:$PORT" \
            --log-level "${AGENT_DEV_LOG_LEVEL:-info}"
        ;;
    server)
        export XDG_DATA_HOME="$stack_dir/data"
        export XDG_RUNTIME_DIR="$stack_dir/run"
        exec ./packages/agent-server/bin/agent-server \
            --addr "$DEVHOST_BIND_HOST:$PORT" \
            --cors-origin "$AGENT_DEV_ORIGIN" \
            --log-level "${AGENT_DEV_LOG_LEVEL:-info}"
        ;;
    observer)
        # Preserve XDG_DATA_HOME: the observer discovers harness sessions there.
        export XDG_CACHE_HOME="$stack_dir/cache"
        exec ./packages/agent-observer/bin/agent-observer \
            --log-level "${AGENT_DEV_LOG_LEVEL:-info}"
        ;;
    *)
        printf '%s\n' 'action=dev_service_failed error=unknown_service' >&2
        exit 1
        ;;
esac
