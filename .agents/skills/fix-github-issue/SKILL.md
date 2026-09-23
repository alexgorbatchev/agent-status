---
name: fix-github-issue
description: >-
  REQUIRED when fixing, resolving, or implementing GitHub issues using subagents in any repository.
  Trigger whenever asked to "fix issue #123", "resolve github issue", "work on open issues",
  "fix this bug with subagents", or execute a delegated issue remediation workflow. Your default training
  knowledge is insufficient; you MUST READ this to execute the parent coordinator lifecycle, red/green TDD
  in isolated worktrees, central issues-dev integration, and the push and issue-closing protocol. Do NOT use
  for filing new issues (use github-issue) or general code review without an issue.
author: alexgorbatchev
metadata:
  created_on: 2026-09-22 14:38
  last_modified: 2026-09-22 21:59
  status: current
---

## Project Discovery

Resolve these once, before provisioning anything, and substitute them everywhere below:

- `<default-branch>`: `gh repo view --json defaultBranchRef -q .defaultBranchRef.name`.
- `<project-commands>`: the exact commands the project documents for targeted tests, the full test suite, lint, typecheck, coverage, and the final "run before declaring work complete" gate. Read them from the repository's instruction files (root and nested `AGENTS.md`, `CLAUDE.md`) first, then from build definitions (`justfile`, `Makefile`, `package.json` scripts, `go.mod`, `pyproject.toml`, `Cargo.toml`). Record each command verbatim with the file it came from.
- `<project-rules>`: the coding, logging, testing, coverage-threshold, and file-placement rules those instruction files state, including nested `AGENTS.md` files for the packages an issue touches.
- `<commit-convention>`: the subject format and issue-reference form used by `git log --oneline -30` on `<default-branch>` (for example `fix(scope): summary (fixes #N)`), overridden by any commit rule in the instruction files or the user's instructions, including rules about attribution trailers.

Never invent a command, path, package, or threshold that these sources do not state. When a category has no documented command, write "none documented" in the subagent brief instead of a guessed one.

## Concurrency & Worktree Topology

- **HARD PROHIBITION**: The coordinator agent is strictly prohibited from invoking `edit` or `write` on any files inside issue worktrees or package directories. Any file modification to implementation or test code MUST be performed by a delegated child subagent. If the coordinator edits code directly, the turn is an automatic failure.
- **Coordinator Boundary**: The coordinator manages queue ordering, provisions worktrees, receives clean review notifications from subagents, merges verified worktrees into `issues-dev`, tears down worktrees, performs the final merge, and closes GitHub issues. The coordinator does NOT micromanage the review loop.
- **Coordinator Rebase Before Provisioning**: The coordinator MUST rebase `issues-dev` onto `<default-branch>` before starting any new subagent or provisioning a new issue worktree.
- **Subagent Rebase Before Review**: Subagents MUST rebase their branch on `issues-dev` before entering the review pairing loop to eliminate integration conflicts for the coordinator.
- **Autonomous Subagent Review Pairing**: Subagents pair autonomously with review agents until no further issues are reported ("clean review"), and notify the coordinator only when the review is clean and ready for integration.
- **Full Queue Completion**: The coordinator works through the entire open issue set, continuing until no open issues remain.
- **Dependency-Aware Order Determination**: The coordinator analyzes all open tickets to establish an execution order:
  1. Foundational defects first: data sources, ingestion, parsing, storage, and core domain logic.
  2. Service, middleware, classification, and reconciliation defects second.
  3. UI, presentation, and other consumer-surface adjustments third.
  4. Tooling, scripts, build, and developer ergonomics fourth.
  5. Issues in distinct packages with no shared files are prioritized for parallel execution across the 3 subagent slots; issues touching the same files run sequentially.
- **Max 3 Parallel Subagents**: At most 3 subagents may run concurrently. When more than 3 issues are queued, maintain an active pool of up to 3 and launch subsequent issues as earlier ones complete.
- **Central Integration Worktree (`issues-dev`)**: All completed issue fixes merge into a central integration worktree `.workspaces/issues-dev` on branch `issues-dev` (branched from `<default-branch>`). No issue branch merges directly to `<default-branch>`.
- **Per-Issue Worktree Isolation**: Each active issue runs in its own dedicated worktree `.workspaces/issue-<number>` on branch `fix/issue-<number>` (branched from `issues-dev`).
- **Continuous Synchronization**: Keep `issues-dev` and every active `.workspaces/issue-<number>` up to date with `<default-branch>` throughout the lifecycle.
- **Harness-Agnostic Subagent Invocation**: Delegate subagents with the active harness's native subagent mechanism. Do not hardcode harness-specific API wrappers, agent profiles, or SDK types.
- **Outward-Facing Actions Need Consent**: Pushing to a remote publishes work. Do not push without the user's explicit approval in the current run. Do not close an issue until its fix is on the remote `<default-branch>`.

---

## 1. Central Worktree Initialization & Queue Ingestion

1. **Run Project Discovery** as described above.

2. **Initialize Central `issues-dev` Worktree**:
   If `.workspaces/issues-dev` does not already exist:

   ```bash
   git worktree add .workspaces/issues-dev -b issues-dev <default-branch>
   ```

   If it already exists, ensure it is rebased on `<default-branch>`:

   ```bash
   git -C .workspaces/issues-dev fetch origin <default-branch>
   git -C .workspaces/issues-dev rebase origin/<default-branch>
   ```

3. **Ingest Issue Queue & Order Tasks**:

   ```bash
   gh issue list --state open --limit 20
   gh issue view <issue-number> --json number,title,body,comments,labels,author
   ```

   Map dependencies across open issues and order them into execution batches.

4. **Rebase `issues-dev` & Provision Per-Issue Worktrees (Up to 3 Max)**:
   Before provisioning each new issue worktree:
   ```bash
   git -C .workspaces/issues-dev fetch origin <default-branch>
   git -C .workspaces/issues-dev rebase origin/<default-branch>
   git worktree add .workspaces/issue-<number> -b fix/issue-<number> issues-dev
   ```

---

## 2. Subagent Worktree Execution & Autonomous Review Pairing

Delegate each issue to a subagent scoped strictly to `.workspaces/issue-<number>`. The brief MUST contain the issue number, title, body, and comments; the worktree path; `<project-commands>` verbatim; `<project-rules>` for the packages involved; `<commit-convention>`; and steps 2A–2E below.

### Step 2A — Red Phase (Failing Reproduction Test)

1. Locate the test file next to the affected code, following the project's existing test layout and naming.
2. Author an automated unit or integration test reproducing the exact defect described in the issue.
3. Run it with the project's targeted-test command inside `.workspaces/issue-<number>` and verify it fails with the expected assertion error. Do not write implementation code until the failure is directly observed in the tool output.

### Step 2B — Green Phase (Primitive-Correct Fix)

1. Implement the change that fixes the root cause.
2. Comply with project constraints:
   - Use native platform primitives (exact types, proper OS APIs, standard library idioms). No shallow proxies, stubs, or wrappers.
   - Follow every rule in `<project-rules>`, including logging conventions and file placement.
3. Re-run the reproduction test and confirm it passes cleanly.
4. Run the project's coverage command and meet the threshold `<project-rules>` documents. If none is documented, report the measured figure without claiming a threshold.
5. Run the project's full test, lint, and typecheck commands in the worktree.

### Step 2C — Mandatory Rebase on `issues-dev` Before Review

Before entering the review pairing loop, rebase `fix/issue-<number>` onto `issues-dev`:

```bash
git -C .workspaces/issue-<number> rebase issues-dev
```

This absorbs changes integrated by concurrent subagents, resolves conflicts inside the worktree, and guarantees the review agent audits the exact rebased code.

### Step 2D — Autonomous Review Pairing Loop

1. Pair with a review agent with fresh context on the worktree diff, using [references/reviewer-prompt.md](references/reviewer-prompt.md). If the project or user provides a code-review skill (for example `code-review-baseline`), instruct the reviewer to load it.
2. Fix every reported defect in `.workspaces/issue-<number>` and re-submit for review.
3. The loop continues until the review agent reports `NO DEFECTS FOUND`.

### Step 2E — Mandatory Verification Handshake & Notification

1. **Red/Green Verification Handshake**:
   - Temporarily disable the fix in the worktree.
   - Run tests to confirm they fail (proving the tests genuinely guard against the defect).
   - Re-enable the fix.
   - Confirm all tests pass cleanly.
2. Run the project's final gate command from `<project-commands>` in the worktree.
3. Commit only the files this issue changed, using `<commit-convention>`:
   ```bash
   git -C .workspaces/issue-<number> add <modified-files>
   git -C .workspaces/issue-<number> commit -m "<subject per commit-convention referencing #<issue-number>>"
   ```
4. Notify the coordinator with the issue number, commit SHA, root cause, fix, and verification evidence (red/green output, final gate result, coverage figure): "Issue #<number> review is clean, tests verified, ready to merge."

---

## 3. Coordinator Integration to `issues-dev` & Teardown

Upon receiving the clean review notification:

1. **Merge into Central `issues-dev`**:

   ```bash
   git -C .workspaces/issues-dev merge --no-ff fix/issue-<number> -m "merge: fix/issue-<number> into issues-dev"
   ```

2. **Teardown Issue Worktree**:

   ```bash
   git worktree remove .workspaces/issue-<number>
   git branch -d fix/issue-<number>
   ```

3. **Record the Closing Summary**: Keep the subagent's root cause, fix, and verification evidence for section 4. Do not close the issue yet; the fix is not on `<default-branch>`.

4. **Replenish Concurrency Pool**: If additional open issues remain, rebase `issues-dev` onto `origin/<default-branch>` and provision the next `.workspaces/issue-<number>` to maintain up to 3 parallel workers.

---

## 4. Final Merge, Push, Issue Closure & Global Teardown

When all queued issues have been merged into `issues-dev`:

1. **Final Sync & Suite Validation**:

   ```bash
   git -C .workspaces/issues-dev fetch origin <default-branch>
   git -C .workspaces/issues-dev merge origin/<default-branch>
   ```

   Run the project's final gate command in `.workspaces/issues-dev`. Stop and report on any failure.

2. **Merge `issues-dev` into `<default-branch>`** from the repository root:

   ```bash
   git checkout <default-branch>
   git merge --ff-only issues-dev
   ```

3. **Push With Consent**: Show the user `git log --oneline origin/<default-branch>..<default-branch>` and ask for approval to push `<default-branch>` to `origin`. If the user declines, leave every issue open, report the unpushed commits, and skip step 4.

4. **Issue Closure Handshake** after the push succeeds. For each merged issue, check `gh issue view <issue-number> --json state`. If a `fixes #N` reference in the pushed commits already closed it, post the summary with `gh issue comment <issue-number> --body "..."`; otherwise:

   ```bash
   gh issue close <issue-number> --comment "Resolved in <commit-sha> on <default-branch>.

   ### Summary of Changes
   - Root cause: <from subagent report>
   - Fix: <from subagent report>
   - Verification: <red/green evidence, final gate result, and coverage figure from subagent report>"
   ```

   Do not state any verification result the subagent report does not contain.

5. **Global Cleanup**:

   ```bash
   git worktree remove .workspaces/issues-dev
   git branch -d issues-dev
   ```

6. **Final Issue Check Before Turn Yield**:
   ```bash
   gh issue list --state open --limit 20
   ```
   If new actionable issues were filed, repeat from Step 1.
