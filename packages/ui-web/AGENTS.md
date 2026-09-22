# ui-web

Web frontend dashboard for agent-status built with React 19, Tailwind CSS v4, Lucide icons, and Bun.

## Commands

- Dev: `just run`
- Build: `just build`
- Test: `just test`
- Test E2E: `just test-e2e`
- Lint: `just lint`
- Guidance: `bun x typescript-ai-policy guidance`

## Local conventions

- Components live in `src/components/ComponentName.tsx` with Storybook stories in `src/components/stories/ComponentName.stories.tsx`.
- Centralized TypeScript types live in `src/types/types.ts` and re-export via `src/types/index.ts`.
- State machines live in `src/machines/` with XState v5 actors and strict event typing.
- Spec-first UI workflow: when modifying or designing UI components (`AgentCard`, `AttentionBanner`, `LogDrawer`), follow the upstream specification established by `air-debug session view` (#50) using `[buttons]` and `(badges)` notation.
- Accessibility tree snapshot testing: write semantic accessibility role-tree snapshot tests in `src/__tests__/*.a11y.test.tsx` using `formatA11yTree` (`src/utils/a11ySnapshot.ts`) to verify component accessible hierarchies, accessible names, headings, badges, and buttons.
- Sessions can be hidden/unhidden via `PATCH /api/v1/agents/{id}`, filtered out by default with a UI toggle to inspect and unhide them.
- Enforce strict TypeScript: no `any`, no type assertions (`as Type`), explicit return types on public functions.

## Local gotchas

- Tailwind v4 uses `@tailwindcss/vite` and CSS `@theme`; there is no `tailwind.config.js`.
- Real-time SSE updates are consumed via `src/hooks/useEventStream.ts` with exponential backoff and random jitter (`src/utils/calculateReconnectDelay.ts`), resetting attempt count on successful open.
- LogDrawer uses `logDrawerMachine` with external `LogBuffer` store consumed via `useSyncExternalStore`.
- ANSI color escape sequences (16-color, 256-color, 24-bit RGB truecolor) and text styles are parsed and rendered via custom `src/utils/ansiUtils.ts`, `src/utils/rehypeAnsi.ts`, and `src/components/AnsiText.tsx` rather than external packages (`ansi-to-react` / `anser`) because off-the-shelf libraries cannot maintain ANSI color state across `react-markdown` AST blocks (e.g. thinking frames wrapping multi-paragraph markdown in truecolor spans) and fail to handle stripped/bare escape codes (`[38;2;...m` without `\x1b`).
- LogDrawer renders log events via markdown using `remark-breaks` to preserve soft line breaks, and includes a debug button copying `just debug session replay --harness <harness> <session-id>` (`getAgentDebugCommand`).

## Boundaries

- Always: read typescript-ai-policy guidelines (`bun x typescript-ai-policy guidance`) before making changes
- Always (code-based projects only): any time code is changed such that results from running that code are changed, a test file must be changed as well; 90% code coverage is required (scripts/ folder is excluded from this rule)
- Ask first: adding UI state management libraries or changing top-level component architecture
- Never: use inline `import("...").Type` or unsafe type casts
- Never: publish releases automatically without explicit user authorization

## References

- App layout: `src/layouts/App.tsx`
- Types: `src/types/types.ts`
- SSE hook: `src/hooks/useEventStream.ts`
- State machine: `src/machines/logDrawerMachine.ts`
- Log buffer store: `src/utils/LogBuffer.ts`
- Relative time hook: `src/hooks/useRelativeTime.ts`
- Dev server: `src/server.ts`
- Policy guidance: `bun x typescript-ai-policy guidance`
