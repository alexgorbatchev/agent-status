# ui-web

Web frontend dashboard for agent-status built with React 19, Tailwind CSS v4, Lucide icons, and Bun.

## Commands

- Dev: `just run`
- Storybook: `just storybook`
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
- Agent card ordering and grouping: session cards maintain stable order without changing places based on recent activity, new sessions are appended, and sessions are grouped by busy/idle/finished in that order without subtitles, separated by double vertical gap (`gap-8`) with each category starting on its own row.
- Subagent chips in `AgentCard` render visual status indicators (`data-subagent-status` "running" vs "completed"/"settling") with smooth exit transitions during their dwell window (#98).
- Sessions can be hidden/unhidden via `PATCH /api/v1/agents/{id}`, filtered out by default with a UI toggle to inspect and unhide them.
- Finished sessions are hidden by default on load, with a UI toggle (`App--toggle-show-finished`) next to show/hide hidden to show/hide finished sessions without state persistence.
- ToolExecutionBlock multiline output container must constrain height (`max-h-64`) and handle overflow (`overflow-auto`) on the flex scroll container wrapping the line gutter and pre output, ensuring line numbers and output content scroll together without empty space.
- Custom scrollbars matching page colors (standard `scrollbar-width: thin`, `scrollbar-color: #334155 transparent` and WebKit `#334155` slate-700 thumb on transparent track) must be defined in `src/theme.css` across all scrollable elements.
- Hierarchical project nesting uses `IProjectTreeGroup` and `IWorktreeBranchGroup` (`src/types/types.ts`) via `groupAgentsByProjectHierarchy`, separating root checkout agents from linked worktree branches with dedicated headers and subgrids.
- Semantic color precedence strictly reserves green and red for lifecycle and activity states; tokens in `src/theme.css` guarantee WCAG AA contrast compliance (>= 4.5:1 text, >= 3:1 graphical) across dark card surfaces (`#020617`, `#0f172a`, `#1e293b`). Light-mode media query swapping is removed to prevent washed-out colors on dark surfaces (#106).
- Card border highlighting uses Tailwind utility classes directly; `src/theme.css` avoids `.status-*` classes or `[data-semantic-status]` selectors that collide with Tailwind card borders (#106).
- CLI parity and color gating: follow `air-debug session view` color gating conventions where color output respects `NO_COLOR` and terminal detection (`go-isatty`) (#106).
- Unacknowledged completion state: When `hasUnseenCompletion` is true, cards display a green `(Completed)` status badge and completion callout with an acknowledge action to dismiss the completion state.
- Idle freshness tiers classify idle sessions as `fresh` (<15m), `standard` (15m–2h), or `stale` (>2h, visually dimmed via opacity styling unless unacknowledged completion is active, #109).
- Redundant project prefixes in card titles are stripped with `trimProjectTitlePrefix` (`src/utils/trimProjectTitlePrefix.ts`) when displayed in project group context.
- Rate-limiting is rendered with a dedicated `RATE_LIMITED` status badge and callout detailing remaining wait time or reset timestamp (`rateLimitResetAt`, `retryAfterSeconds`).
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
