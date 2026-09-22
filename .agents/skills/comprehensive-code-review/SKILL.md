---
name: comprehensive-code-review
description: >-
  REQUIRED when conducting an in-depth codebase audit to find bugs, DRY/modularization
  opportunities, accessibility gaps, test deficiencies, or standard violations and filing
  GitHub issues for the findings. Trigger whenever asked to "review code", "find opportunities
  to modularize and dry", "audit the codebase", or "file issues for review findings".
author: alexgorbatchev
metadata:
  created_on: 2026-09-20 04:00
  last_modified: 2026-09-20 04:00
  status: current
---

## Review & Audit Phase

### 1. Codebase Discovery & Baseline Verification

- Map the target codebase structure, component hierarchies, layouts, hooks, utilities, and configurations.
- Inspect test suites: unit tests, component tests, Storybook stories, and E2E specs.
- Run typecheck (`tsc --noEmit`), linter/policy check (`bun run check` or `npm run lint`), and test commands (`bun test`, `playwright test`) to discover existing compiler errors, linter violations, or test runner conflicts.

### 2. Multi-Dimensional Code Inspection

Audit the code systematically across six core dimensions:

1. **Defects & State Synchronization**:
   - Trace cross-component predicate consistency (e.g., matching filter criteria between headers, summaries, and lists).
   - Check lifecycle hooks (`useEffect`) for missing cleanup, unreset state when switching IDs, or missing dependencies.
   - Identify invalid CSS classes or broken template expressions.
2. **DRY & Utility Consolidation**:
   - Find duplicated metric aggregations, currency/number formatting, and date utilities across multiple call sites.
   - Identify duplicated domain transformations (grouping, filtering, sorting) embedded inside components.
   - Spot hardcoded configuration defaults (ports, tokens, fallback URLs) duplicated in multiple files.
3. **Component Modularization & Declarative Mapping**:
   - Identify near-identical layout components sharing >80% markup and unify them into composable base components.
   - Decompose monolithic components combining stream subscriptions with complex inline render blocks into dedicated sub-components.
   - Refactor repetitive multi-branch `switch`/`case` render blocks into declarative configuration lookup tables.
4. **Accessibility & Semantic Primitives**:
   - Find interactive non-semantic elements (`div`, `span` with `onClick`) lacking `role="button"`, `tabIndex={0}`, or keyboard listeners (`Enter`, `Space`).
   - Check interactive controls (`button`, `a`, `input`) for explicit `type="button"` and `aria-label` attributes.
5. **Language & Framework Standards**:
   - Check for missing explicit return types on public functions and React components (`: React.JSX.Element | null`).
   - Identify forbidden patterns (e.g., `any`, unwarranted type assertions, calling compiler runtime `jsx()` directly in `.ts` files instead of standard JSX in `.tsx`).
6. **Testing Gaps & Suite Robustness**:
   - Check for unhandled test runner collisions (e.g., unit test runners picking up E2E spec files).
   - Check for superficial tests (only checking if a component renders without verifying visual state or dynamic updates).
   - Check for missing scenarios: stream disconnection/reconnect, item deletion, empty states, and all status matrix permutations.

---

## Issue Formulation & Ticketing Phase

Use the `github-issue` skill to create GitHub issues.
