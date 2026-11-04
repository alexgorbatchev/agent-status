#!/usr/bin/env bash
set -euo pipefail

# Report unformatted Go source in the current directory. Lists tracked files and
# new untracked ones, but never ignored output such as compiler scratch under
# .tmp or nested worktrees, which `gofmt -l .` would walk.
# NUL delimiters keep filenames with whitespace intact.
output=$(
  git ls-files --cached --others --exclude-standard -z -- '*.go' |
    while IFS= read -r -d '' path; do
      # A tracked file may have been deleted in the working tree.
      if [[ -f "$path" ]]; then
        printf './%s\0' "$path"
      fi
    done |
    xargs -0 gofmt -l
)

if [[ -n "$output" ]]; then
  printf 'Unformatted Go files:\n%s\n' "$output" >&2
  exit 1
fi
