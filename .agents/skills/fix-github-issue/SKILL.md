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
  last_modified: 2026-09-22 15:02
  status: current
---

## Concurrency & Worktree Topology

- **Coordinator Supremacy**: The parent agent acts as the coordinator. It manages the issue queue, arbitrates review findings, performs all merges to integration and main branches, and executes the issue closure handshake.
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

2. **Ingest Issue Queue**:
   Query open issues:
   ```bash
   gh issue list --state open --limit 20
   ```
   For each target issue, fetch details:
   ```bash
   gh issue view <issue-number> --json number,title,body,comments,labels,author
   ```
   Extract reproduction criteria, root cause hypotheses, affected packages, and acceptance criteria.

3. **Provision Per-Issue Worktrees (Up to 3 Max)**:
   For up to 3 issues simultaneously, create dedicated worktrees branched from `issues-dev`:
   ```bash
   git worktree add .workspaces/issue-<number> -b fix/issue-<number> issues-dev
   ```

---

## 2. Writer Subagent Execution (Red/Green TDD)

Delegate each issue implementation to a writer subagent scoped strictly to `.workspaces/issue-<number>` using the harness's native mechanism.

Instruct the writer subagent to follow the strict Red/Green sequence:

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

### Step 2C — Package & Workspace Verification
1. Confirm statement coverage meets or exceeds 90%:
   ```bash
   go test -cover ./...
   ```
2. Run workspace test and lint suites in the worktree:
   ```bash
   just test-all
   just lint-all
   ```

---

## 3. Independent Review Subagent Gate

Once the writer subagent finishes, delegate a read-only review to an independent reviewer subagent with fresh context in `.workspaces/issue-<number>`.

Pass the prompt template and checklist from [references/reviewer-prompt.md](references/reviewer-prompt.md).

The reviewer audits:
- Defect resolution validity (true root cause vs superficial patch).
- Absence of regressions and side-effects.
- Semantic integrity of primitives and error handling (`%w` wrapping).
- Test robustness and 90% coverage bar.
- Absence of forbidden patterns (`any`, `as any`, heredocs, linter evasion).

---

## 4. Parent Synthesis & Mandatory Verification Handshake

1. **Arbitrate Review Findings**:
   Address high and medium findings in `.workspaces/issue-<number>` before integrating.

2. **Sync with Main (Best Effort)**:
   Before final verification, pull latest changes from `main` into the issue worktree to prevent divergence:
   ```bash
   git -C .workspaces/issue-<number> merge issues-dev
   ```

3. **Mandatory Red/Green Handshake**:
   - Temporarily disable or revert the fix logic in `.workspaces/issue-<number>`.
   - Run tests to confirm they fail (validating that tests genuinely guard against the defect).
   - Re-enable the fix logic.
   - Confirm tests pass cleanly.

4. **Final Worktree Suite Validation**:
   ```bash
   cd .workspaces/issue-<number> && just test-all && just lint-all
   ```

---

## 5. Integration to `issues-dev`, Worktree Teardown & Issue Closure

1. **Commit in Issue Worktree**:
   Follow conventional commits:
   ```bash
   cd .workspaces/issue-<number>
   git add <modified-files>
   git commit -m "fix(<package>): <concise summary> (#<issue-number>)"
   ```

2. **Merge into Central `issues-dev`**:
   In `.workspaces/issues-dev`:
   ```bash
   cd .workspaces/issues-dev
   git merge --no-ff fix/issue-<number> -m "merge: fix/issue-<number> into issues-dev"
   ```

3. **Teardown Issue Worktree**:
   Remove the individual issue worktree and branch:
   ```bash
   git worktree remove .workspaces/issue-<number>
   git branch -d fix/issue-<number>
   ```

4. **Issue Closure Handshake**:
   Comment on and close the issue:
   ```bash
   gh issue close <issue-number> --comment "Resolved in issues-dev.

   ### Summary of Changes
   - Root cause: <description>
   - Fix: <description of changes>
   - Verification: Added regression tests; verified via red/green validation loop and independent review. Checks pass with >=90% test coverage."
   ```

5. **Replenish Concurrency Pool**:
   If additional open issues remain in the queue, immediately provision the next `.workspaces/issue-<number>` from `issues-dev` to maintain up to 3 parallel workers.

---

## 6. Final Main Merge & Global Teardown

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
