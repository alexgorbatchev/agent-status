---
created_on: 2026-10-14 22:41
last_modified: 2026-10-14 22:41
status: current
---

# Agent Status workspace

This repository preserves the Agent Status source monorepo boundary. Its six Cargo packages live under `packages/`; CLI usage skills live under repository-root `skills/`. Consult each package's nested instructions before editing it.

## Commands

- Discover modules: `just --list --list-submodules`.
- Test, lint, build, or check all packages: `just test`, `just lint`, `just build`, `just check`.
- Select a package module: `just debug test`, `just observer lint`, `just ingester build`, `just server check`, `just common test`, or `just store test`.
- Cargo commands must select the intended package with `--package <name>`, or explicitly use `--workspace` for workspace-wide work. Keep the root Cargo.lock current; do not add member lockfiles or nested Git repositories.

## Contracts

- Keep native shared types and transactional schema operations in the owning workspace packages. Preserve the distinction between ingester writes and server read-only queries.
- Keep each CLI's repository-root operational skill synchronized with changes to its live command interface. Preserve installation-pointer behavior.
- Keep devhost's API and dashboard on the same hostname. Use the `DEVHOST_DEV_EXT` environment variable for its suffix and keep machine-specific overrides in the ignored `.envrc.local`.
- Scope formatters to the intended package; `cargo fmt --all` may also visit external path dependencies.
- Runtime changes require behavioral red/green evidence, genuine implementation-disable/restoration proof, and at least 90% measured line coverage. Structural path-only changes use relevant interface tests and Cargo validation without redundant coverage runs.
- Use `.tmp/` for evidence and temporary files. Retain original source provenance, licenses, binaries, regression fixtures, and recorded verification limits.
- For ticket changes, use the issue-tracker, programming, and github-pr skills. Work in an isolated worktree and require independent PR review and passing CI before fast-forward integration.
- Inherit the parent Rust instructions for build-storage coordination, skills consultation, isolation, and sensitive information. Never contact live services, read unrelated credentials, commit, push, or publish without explicit authorization.
