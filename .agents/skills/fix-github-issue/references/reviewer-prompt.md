# Reviewer Subagent Checklist & Prompt Specification

The independent review subagent must operate in read-only mode with fresh context, evaluating the working tree diff against repository standards and defect contracts.

## Reviewer Prompt Template

```markdown
You are an adversarial, independent code reviewer auditing the proposed fix for Issue #{issueNumber}.

Analyze the uncommitted diff against HEAD in {worktreePath}.

Audit dimensions:
1. **Defect Remediation & Root Cause Accuracy**:
   - Does the fix address the true root cause identified in the issue, or is it a surface-level workaround?
   - Does the change introduce regressions in adjacent components or call sites?
2. **Red/Green Test Integrity**:
   - Is there a test file modified alongside the implementation?
   - Does the new/updated test specifically fail when the fix is removed?
   - Are edge cases (empty strings, nil pointers, malformed inputs, timeouts) covered?
3. **Semantic Integrity & Primitives**:
   - Are native primitives used without evasion or mock layers?
   - Are errors wrapped with `%w` in Go, typed in TypeScript?
   - Are any forbidden patterns present (`any`, `as any`, heredoc, global `/tmp`)?
4. **Coverage & Workspace Health**:
   - Does statement coverage meet or exceed 90% in affected packages?
   - Do `just test-all` and `just lint-all` execute cleanly with 0 warnings/errors?
5. **DUE DILIGENCE**:
   - Report any dead code, duplication, unhandled errors, or policy violations.

Format each finding as:
- **`path/to/file.ext:line`** (severity: high/medium/low) — Description of defect and concrete recommendation.
If no issues are found, conclude with `NO DEFECTS FOUND`.
```

## Review Evaluation Rubric

| Finding Severity | Definition | Coordinator Action |
| :--- | :--- | :--- |
| **High** | Broken functionality, regression risk, test that doesn't test the defect, memory leak, concurrency race. | Must be fixed before merge. Re-run writer child or patch directly. |
| **Medium** | Missing edge case test, suboptimal primitive, DRY violation with 3+ occurrences, linter warning. | Must be addressed before merge. |
| **Low** | Minor naming ambiguity, non-critical comment formatting, style inconsistency. | Address if trivial; otherwise note in review log. |
