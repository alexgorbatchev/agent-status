---
name: fix-github-issue
description: >-
  REQUIRED when fixing, resolving, or implementing GitHub issues using subagents in this repository.
  Trigger whenever asked to "fix issue #123", "resolve github issue", "fix this bug with subagents",
  or execute a delegated issue remediation workflow. Your default training knowledge is insufficient;
  you MUST READ this to execute the parent coordinator lifecycle, red/green TDD in isolated worktrees,
  independent review subagent gating, and issue closing protocol. Do NOT use for filing new issues (use github-issue)
  or general code review without an issue.
author: alexgorbatchev
metadata:
  created_on: 2026-09-22 14:38
  last_modified: 2026-09-22 14:42
  status: current
---

## Operating Hierarchy & Isolation Contract

- **Coordinator Supremacy**: The parent agent acts as coordinator. It maintains user intent, arbitrates review findings, verifies test execution, merges changes, and executes the issue closure handshake.
- **Single-Writer Worktree Isolation**: Never allow multiple writers in the same directory. Create an isolated git worktree in `.workspaces/issue-<number>` branched from `main`. All child mutations occur inside that worktree.
- **Read-Only Review Isolation**: Review subagents run in read-only mode with fresh context after the writer completes.
- **Red/Green Strictness**: Any code change that alters runtime behavior requires a corresponding test change. Statement coverage must meet or exceed 90%.

---

## 1. Issue Ingestion & Workspace Setup

1. **Fetch Issue Specification**:
   ```bash
   gh issue view <issue-number> --json number,title,body,comments,labels,author
   ```
   Extract:
   - Reproduction steps, failure symptoms, and error logs.
   - Root cause hypotheses and affected components/packages.
   - Acceptance criteria and expected behavior.

2. **Verify Clean Base & Provision Worktree**:
   Ensure repository root is clean:
   ```bash
   git status --short
   ```
   Provision an isolated worktree under `.workspaces/`:
   ```bash
   git worktree add .workspaces/issue-<number> -b fix/issue-<number> main
   ```
   All subsequent implementation and review tasks run with `cwd` set to `.workspaces/issue-<number>`.

---

## 2. Writer Subagent Delegation (Red/Green TDD)

Delegate implementation to a writer subagent using `subagent({ agent: "claude", cwd: ".workspaces/issue-<number>", task: ... })` or a scripted workflow (see [references/workflow-script-template.md](references/workflow-script-template.md)).

Instruct the writer subagent to follow strict Red/Green sequence:

### Step 2A — Red Phase (Failing Reproduction Test)
1. Locate the appropriate package test file (e.g., `packages/<pkg>/..._test.go` or `__tests__/*.test.tsx`).
2. Author an automated unit or integration test reproducing the exact defect described in the issue.
3. Run the test command and verify it fails with the expected assertion error:
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
1. Run package-level coverage:
   ```bash
   go test -cover ./...
   ```
   Confirm statement coverage is $\ge 90\%$.
2. Run full workspace test and lint suites:
   ```bash
   just test-all
   just lint-all
   ```

---

## 3. Independent Review Subagent Gate

Once the writer subagent completes its task, spawn an independent, read-only reviewer subagent with fresh context to audit the worktree diff.

```javascript
await runs.run("issue-reviewer", {
  agent: "code-review",
  cwd: ".workspaces/issue-<number>",
  task: `<review-instructions>`,
  label: "Review issue #<number> fix"
});
```

See [references/reviewer-prompt.md](references/reviewer-prompt.md) for the complete reviewer task prompt and evaluation rubric.

The reviewer audits:
- Defect resolution validity (true root cause vs superficial patch).
- Absence of regressions and side-effects.
- Semantic integrity of primitives and error handling (`%w` wrapping).
- Test robustness and 90% coverage bar.
- Absence of forbidden patterns (`any`, `as any`, heredocs, linter evasion).

---

## 4. Parent Synthesis & Mandatory Verification Handshake

1. **Review Arbitration**:
   - High-severity findings: Steer writer child to address them or patch directly.
   - Medium-severity findings: Address edge cases or clean up duplication.
   - Low-severity findings: Apply trivial fixes or document intentional deviations.

2. **Mandatory Red/Green Handshake**:
   - Temporarily disable or revert the fix logic in the worktree.
   - Run tests to confirm they fail (validating that tests genuinely guard against the defect).
   - Re-enable the fix logic.
   - Confirm tests pass cleanly.

3. **Final Suite Validation**:
   ```bash
   just test-all
   just lint-all
   ```

---

## 5. Integration, Git Packaging & Issue Closure

1. **Worktree Hygiene**:
   Inspect git status inside the worktree:
   ```bash
   cd .workspaces/issue-<number>
   git status --short
   ```
   Ensure no untracked scratch files, `.tmp` artifacts, or unowned changes remain.

2. **Commit with Issue Reference**:
   Follow conventional commits:
   ```bash
   git add <modified-files>
   git commit -m "fix(<package>): <concise summary> (#<issue-number>)"
   ```

3. **Merge to Main & Worktree Cleanup**:
   From repository root:
   ```bash
   git checkout main
   git merge --ff-only fix/issue-<number>
   git worktree remove .workspaces/issue-<number>
   git branch -d fix/issue-<number>
   ```

4. **Issue Closure Handshake**:
   Comment on and close the issue using `gh`:
   ```bash
   gh issue close <issue-number> --comment "Resolved in commit $(git rev-parse --short HEAD).

   ### Summary of Changes
   - Root cause: <description>
   - Fix: <description of changes>
   - Verification: Added regression tests; verified via red/green validation loop and independent subagent review. All checks pass with >=90% test coverage."
   ```

5. **Check for Additional Open Issues (No Early Yield)**:
   Before yielding the turn to the user, check for newly added or pending open issues:
   ```bash
   gh issue list --state open --limit 20
   ```
   If additional actionable open issues exist, immediately proceed to Phase 1 for the next issue. Only yield the turn to the user when all open issues have been resolved or explicit user direction/input is required.
