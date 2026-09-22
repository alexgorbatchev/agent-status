# Subagent Workflow Script Template

Use this script template inside `subagent({ workflowScript: ... })` when orchestrating a multi-stage issue fix programmatically.

```javascript
// Step 1: Writer Subagent in isolated worktree
const writerOutput = await runs.run("issue-writer", {
  agent: "claude",
  cwd: args.worktreePath,
  task: `You are fixing GitHub Issue #${args.issueNumber}: "${args.issueTitle}".

Issue description:
${args.issueBody}

MANDATORY RULES:
1. Work strictly in Red/Green TDD:
   a. Write a reproducing test first in the affected test file.
   b. Run the test to observe failure.
   c. Implement the fix using semantically correct native primitives (no shallow wrappers or linter evasion).
   d. Run the test to observe it passing.
2. 90% statement test coverage is required for changed packages.
3. Run 'just test-all' and 'just lint-all' before concluding.
4. Return a summary of root cause, files changed, and test results.`,
  label: `Fix issue #${args.issueNumber}`
});

// Step 2: Independent Review Subagent (read-only, fresh context)
const reviewOutput = await runs.run("issue-reviewer", {
  agent: "code-review",
  cwd: args.worktreePath,
  task: `Perform an independent, adversarial code review of the diff in ${args.worktreePath} fixing Issue #${args.issueNumber}.

Review against:
1. Semantic correctness and edge cases.
2. Red/Green test verification: does the new test genuinely guard against the defect?
3. Code standards: no 'any', explicit return types, structured logging, no heredocs, DRY.
4. Coverage: verify 90% statement coverage is maintained.

Format findings with clickable file:line references. If clean, state 'NO DEFECTS FOUND'.`,
  label: `Review issue #${args.issueNumber} fix`
});

return {
  writerOutput: writerOutput.output,
  reviewOutput: reviewOutput.output
};
```
