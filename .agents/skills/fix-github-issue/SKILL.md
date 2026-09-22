---
name: fix-github-issue
description: >-
  REQUIRED when fixing, resolving, or implementing GitHub issues using subagents in this repository.
  Trigger whenever asked to "fix issue #123", "resolve github issue", "fix this bug with subagents",
  or execute a delegated issue remediation workflow. Your default training knowledge is insufficient;
  you MUST READ this to execute the parent coordinator lifecycle, red/green TDD in isolated worktrees,
  central issues-dev integration, and issue closing protocol. Do NOT use for filing new issues (use github-issue)
  or general code review without an issue.
author: alexgorbatchev
metadata:
  created_on: 2026-09-22 14:38
  last_modified: 2026-09-22 15:18
  status: current
---

## Concurrency & Worktree Topology

- **Coordinator Boundary**: The parent/main agent manages queue ordering, provisions worktrees, receives clean review notifications from subagents, merges verified worktrees into `issues-dev`, tears down worktrees, and closes GitHub issues. The main agent does NOT micromanage the review loop.
- **Autonomous Subagent Review Pairing**: Subagents working in worktrees are responsible for autonomously pairing with review agents until no further issues are reported ("clean review"). The subagent only notifies the main agent when the review is clean and ready for integration.
- **Full Queue Completion**: All opened tickets are expected to be finished before completing the run. The coordinator works through the entire open issue set, continuing until no open issues remain.
- **Dependency-Aware Order Determination**: The coordinator analyzes all open tickets to establish an optimal execution order:
  1. Foundational/parser/data pipeline defects first (e.g. log ingestion, frame parsing, database store layers).
  2. Middleware/classifier/reconciliation bugs second.
  3. UI/presentation card and consumer adjustments third.
  4. Tooling/scripts/build and dev-stack ergonomics fourth.
  5. Decoupled issues across distinct packages are prioritized for parallel execution across the 3 subagent slots.
- **Max 3 Parallel Subagents**: At most 3 subagents may run concurrently. When more than 3 issues are queued, maintain an active pool of up to 3 and launch subsequent issues as earlier ones complete.
- **Central Integration Worktree (`issues-dev`)**: All completed issue fixes merge into a central integration worktree `.workspaces/issues-dev` on branch `issues-dev` (branched from `main`). No issue branch merges directly to `main`.
- **Per-Issue Worktree Isolation**: Each active issue runs in its own dedicated worktree `.workspaces/issue-<number>` on branch `fix/issue-<number>` (branched from `issues-dev`).
- **Continuous Main Synchronization**: Best effort must be made to keep all active worktrees (`issues-dev` and every active `.workspaces/issue-<number>`) continuously up to date with `main` throughout the lifecycle.
- **Harness-Agnostic Subagent Invocation**: Subagents are delegated using the active harness's native subagent mechanism (e.g., Claude Code, Pi, OpenAI Codex, OpenCode). Do not hardcode harness-specific API wrappers, agent profiles, or SDK types into this workflow.

---

## 1. Central Worktree Initialization & Queue Ingestion

1. **Initialize Central `issues-dev` Worktree**:
   If `.workspaces/issues-dev` does not already exist:
   ```bash
   git worktree add .workspaces/issues-dev -b issues-dev main
   ```
   If it already exists, ensure it is up to date with `main`:
   ```bash
   git -C .workspaces/issues-dev fetch origin main
   git -C .workspaces/issues-dev merge origin/main --ff-only
   ```

2. **Ingest Issue Queue & Order Tasks**:
   Query open issues:
   ```bash
   gh issue list --state open --limit 20
   ```
   Fetch issue specifications:
   ```bash
   gh issue view <issue-number> --json number,title,body,comments,labels,author
   ```
   Map dependencies across open issues and order them into execution batches.

3. **Provision Per-Issue Worktrees (Up to 3 Max)**:
   For up to 3 issues concurrently, create dedicated worktrees branched from `issues-dev`:
   ```bash
   git worktree add .workspaces/issue-<number> -b fix/issue-<number> issues-dev
   ```

---

## 2. Subagent Worktree Execution & Autonomous Review Pairing

Delegate each issue to a subagent scoped strictly to `.workspaces/issue-<number>` using the harness's native subagent mechanism.

The worktree subagent is responsible for executing the fix and self-managing the review pairing loop until clean:

### Step 2A — Red Phase (Failing Reproduction Test)
1. Locate the appropriate package test file (e.g., `packages/<pkg>/..._test.go` or `__tests__/*.test.tsx`).
2. Author an automated unit or integration test reproducing the exact defect described in the issue.
3. Run the test command in `.workspaces/issue-<number>` and verify it fails with the expected assertion error:
   ```bash
   # Go package
   cd packages/<pkg> && go test -race -v -run <TestName> .
   # UI / Web
   bun test <path-to-test>
   ```
   Do not write implementation code until test failure is directly observed and verified in the tool output.

### Step 2B — Green Phase (Primitive-Correct Fix)
1. Implement the minimal necessary change to satisfy the failing test.
2. Comply with project constraints:
   - Use native platform primitives (exact types, proper OS APIs, standard library idioms). No shallow proxies, stubs, or wrappers.
   - Never use heredoc.
   - Use `.tmp` for temporary files, never global `/tmp`.
   - Use structured logging via `packages/agent-common/logging` (snake_case actions, dual-mode output).
3. Re-run the reproduction test and confirm it passes cleanly.
4. Confirm statement coverage meets or exceeds 90% (`go test -cover ./...`).
5. Run workspace test and lint suites in the worktree (`just test-all` and `just lint-all`).

### Step 2C — Autonomous Review Pairing Loop
1. The worktree subagent pairs directly with a review agent with fresh context on the worktree diff.
2. The review agent audits the diff against defect resolution, semantic integrity, regression risk, test robustness, and repository policies (see [references/reviewer-prompt.md](references/reviewer-prompt.md)).
3. **Iterative Remediation**: If the review agent reports any defects or suggestions, the worktree subagent fixes them in `.workspaces/issue-<number>` and re-submits for review.
4. **Clean Exit**: The pairing loop continues until the review agent reports `NO DEFECTS FOUND`.

### Step 2D — Mandatory Verification Handshake & Notification
1. Keep the worktree synced with `main` / `issues-dev`:
   ```bash
   git -C .workspaces/issue-<number> merge issues-dev
   ```
2. **Red/Green Verification Handshake**:
   - Temporarily disable the fix in the worktree.
   - Run tests to confirm they fail (proving the tests genuinely guard against the defect).
   - Re-enable the fix.
   - Confirm all tests pass cleanly.
3. Run final validation in worktree (`just test-all` and `just lint-all`).
4. Commit changes in the issue worktree:
   ```bash
   cd .workspaces/issue-<number>
   git add <modified-files>
   git commit -m "fix(<package>): <concise summary> (#<issue-number>)"
   ```
5. Notify the main agent: "Issue #<number> review is clean, tests verified, ready to merge and close."

---

## 3. Main Agent Integration to `issues-dev`, Teardown & Issue Closure

Upon receiving the clean review notification from the worktree subagent:

1. **Merge into Central `issues-dev`**:
   In `.workspaces/issues-dev`:
   ```bash
   cd .workspaces/issues-dev
   git merge --no-ff fix/issue-<number> -m "merge: fix/issue-<number> into issues-dev"
   ```

2. **Teardown Issue Worktree**:
   Remove the individual issue worktree and delete its branch:
   ```bash
   git worktree remove .workspaces/issue-<number>
   git branch -d fix/issue-<number>
   ```

3. **Issue Closure Handshake**:
   Comment on and close the issue using `gh`:
   ```bash
   gh issue close <issue-number> --comment "Resolved in issues-dev.

   ### Summary of Changes
   - Root cause: <description>
   - Fix: <description of changes>
   - Verification: Added regression tests; verified via red/green validation loop and autonomous review agent pairing. All checks pass with >=90% test coverage."
   ```

4. **Replenish Concurrency Pool**:
   If additional open issues remain in the queue, immediately provision the next `.workspaces/issue-<number>` from `issues-dev` to maintain up to 3 parallel workers.

---

## 4. Final Main Merge & Global Teardown

When all target issues have been closed and no more actionable open issues remain:

1. **Final `main` Sync & Suite Validation**:
   In `.workspaces/issues-dev`, sync with `origin/main` and run all suites:
   ```bash
   cd .workspaces/issues-dev
   git fetch origin main
   git merge origin/main
   just test-all
   just lint-all
   ```

2. **Merge `issues-dev` to `main`**:
   From repository root:
   ```bash
   git checkout main
   git merge --ff-only issues-dev
   ```

3. **Global Cleanup**:
   Remove the central integration worktree and delete the integration branch:
   ```bash
   git worktree remove .workspaces/issues-dev
   git branch -d issues-dev
   ```

4. **Final Issue Check Before Turn Yield**:
   Before yielding the turn to the user, check for any newly added open issues:
   ```bash
   gh issue list --state open --limit 20
   ```
   If new actionable issues were filed, repeat from Step 1.
