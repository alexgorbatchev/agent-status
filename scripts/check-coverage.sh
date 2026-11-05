#!/usr/bin/env bash
set -euo pipefail

# Required statement coverage threshold
THRESHOLD="${THRESHOLD:-90.0}"

process_stream() {
  local exit_code=0
  local low_coverage=()

  while IFS= read -r line || [[ -n "$line" ]]; do
    echo "$line"
    if [[ "$line" == *"[no tests to run]"* ]] || [[ "$line" == *"[no test files]"* ]]; then
      continue
    fi
    if [[ "$line" =~ ^FAIL([[:space:]]|$) ]] || [[ "$line" =~ ^---[[:space:]]FAIL: ]]; then
      exit_code=1
    fi
    if [[ "$line" =~ coverage:[[:space:]]*([0-9.]+)%[[:space:]]*of[[:space:]]*statements ]]; then
      local pct="${BASH_REMATCH[1]}"
      local is_low
      is_low=$(awk -v p="$pct" -v t="$THRESHOLD" 'BEGIN { print (p < t) ? 1 : 0 }')
      if [ "$is_low" -eq 1 ]; then
        low_coverage+=("$line")
        exit_code=1
      fi
    fi
  done

  if [ ${#low_coverage[@]} -gt 0 ]; then
    echo "" >&2
    echo "FAIL: statement coverage below ${THRESHOLD}% threshold:" >&2
    for entry in "${low_coverage[@]}"; do
      echo "  $entry" >&2
    done
    return 1
  fi

  return $exit_code
}

if [ $# -gt 0 ]; then
  has_cover=0
  for arg in "$@"; do
    if [[ "$arg" == -cover* ]]; then
      has_cover=1
      break
    fi
  done
  if [ $has_cover -eq 0 ]; then
    set -- -cover "$@"
  fi
  scratch_root=${GOTMPDIR:-${TMPDIR:-/tmp}}
  mkdir -p "$scratch_root"
  scratch_root=$(cd "$scratch_root" && pwd)
  run_dir=$(mktemp -d "$scratch_root/coverage.XXXXXX")
  test_pid=
  registry=
  groups=()
  scratch_dirs=()
  collect_runs() {
    local record group scratch
    groups=()
    scratch_dirs=()
    [[ -n "$test_pid" ]] && groups+=("$test_pid")
    [[ -d "$registry" ]] || return 0
    while IFS= read -r -d '' record; do
      group=$(cat "$record/group" 2>/dev/null) || group=
      [[ "$group" =~ ^[0-9]+$ ]] && groups+=("$group")
      scratch=$(cat "$record/scratch" 2>/dev/null) || scratch=
      [[ -n "$scratch" ]] && scratch_dirs+=("$scratch")
    done < <(find "$registry" -depth -type d -print0 2>/dev/null)
    return 0
  }
  signal_groups() {
    local signal=$1 group
    for group in ${groups[@]+"${groups[@]}"}; do
      kill -"$signal" -- "-$group" 2>/dev/null || true
    done
  }
  groups_alive() {
    local group
    for group in ${groups[@]+"${groups[@]}"}; do
      kill -0 -- "-$group" 2>/dev/null && return 0
    done
    return 1
  }
  cleanup() {
    local result=$? scratch
    trap - EXIT
    # An enclosing cancelled runner may close our output pipes while cleanup
    # is still running. Job notifications must not abort descendant cleanup.
    trap '' HUP INT TERM PIPE
    set +m
    collect_runs
    signal_groups TERM
    # Nested runners register their groups outside their own scratch. The
    # enclosing runner can kill them even if their supervisors exit first.
    for ((attempt = 0; attempt < 50; attempt++)); do
      groups_alive || break
      sleep 0.1
    done
    collect_runs
    signal_groups KILL
    [[ -z "$test_pid" ]] || wait "$test_pid" 2>/dev/null || true
    for scratch in ${scratch_dirs[@]+"${scratch_dirs[@]}"}; do
      rm -rf -- "$scratch" || result=1
    done
    [[ -z "$registry" ]] || rm -rf -- "$registry" || result=1
    rm -rf -- "$run_dir" || result=1
    exit "$result"
  }
  trap cleanup EXIT
  trap 'exit 129' HUP
  trap 'exit 130' INT
  trap 'exit 143' TERM

  if [[ -n "${COVERAGE_RUN_REGISTRY:-}" ]]; then
    registry=$(mktemp -d "$COVERAGE_RUN_REGISTRY/run.XXXXXX")
  else
    registry="$run_dir/runners"
    mkdir "$registry"
  fi
  printf '%s' "$run_dir" > "$registry/scratch"
  export COVERAGE_RUN_REGISTRY="$registry"

  export TMPDIR="$run_dir" GOTMPDIR="$run_dir"
  # A non-repository fixture must not discover this checkout as its Git root.
  export GIT_CEILING_DIRECTORIES="$run_dir${GIT_CEILING_DIRECTORIES:+:$GIT_CEILING_DIRECTORIES}"
  # One process group owns Go, test binaries, and the coverage reader. Keep
  # nested job control off so the pipeline does not create separate groups.
  set -m
  (
    set +m
    go test "$@" 2>&1 | process_stream
  ) &
  test_pid=$!
  printf '%s\n' "$test_pid" > "$registry/group"
  wait "$test_pid"
else
  process_stream
fi
