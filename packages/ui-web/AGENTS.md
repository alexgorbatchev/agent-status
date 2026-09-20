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
- Enforce strict TypeScript: no `any`, no type assertions (`as Type`), explicit return types on public functions.

## Local gotchas

- Tailwind v4 uses `@tailwindcss/vite` and CSS `@theme`; there is no `tailwind.config.js`.
- Real-time SSE updates are consumed via `src/hooks/useEventStream.ts`.
- LogDrawer uses `logDrawerMachine` with external `LogBuffer` store consumed via `useSyncExternalStore`.

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
