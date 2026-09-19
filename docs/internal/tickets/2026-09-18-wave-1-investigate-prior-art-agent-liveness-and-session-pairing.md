---
created_on: 2026-09-18 22:14
last_modified: 2026-09-18 22:14
status: current
ticket_status: open
---

# Wave 1: Investigate prior art for agent liveness and process-to-session pairing

## Problem

Three of the observer's load-bearing rules were chosen from first principles against a single machine's on-disk evidence, with no comparison against how existing tools solve the same problems. Each is a guess that happens to fit the data available at the time of writing.

1. **Pi process-to-session pairing.** Pi writes no pid file, and `--session-id` exists only on an experimental subcommand, so nothing directly links a running `pi` process to the session file it is writing. `docs/prd.md` §4.1.3 pairs them by matching each process's start time against the earliest session record at or after that time. Concurrent Pi Agents in one directory are routine — three overlapping pairs were observed in `--Users-agorbatchev-development-dotfiles-installer--` on 2026-09-18 alone — so the rule is exercised constantly and a wrong pairing silently attributes one Agent's activity to another.

2. **Permission-prompt detection.** Recorded as an unresolved gap in `docs/prd.md` §11. No signal for a *pending* permission prompt has been found: the Claude Code descriptor's `status` field shows only `busy` and `idle`, and transcript `permission-mode` records describe mode changes rather than pending prompts. An Agent blocked on a permission check currently reports as working.

3. **Darwin working-directory resolution.** `docs/prd.md` §4.1.3 shells out to `lsof -a -p <pid> -d cwd -Fn` once per discovered Pi process per scan, because a pure-Go path was not confirmed at the time of writing. `docs/refs.md` §1.2 asserts that `gopsutil/v4/process` resolves this natively on Darwin through `libproc` (`proc_pidinfo` with `PROC_PIDVNODEPATHINFO`). If that holds, the subprocess disappears.

## Why this matters

Pairing correctness determines whether Pi support can be trusted at all. A mispaired Agent does not fail loudly — it shows one engineer's work under another Agent's card, with a summary describing the wrong task, which is worse than showing nothing.

Permission-prompt detection is a stated user wish (`docs/user-spec.md` Wish 4 and §6). An Agent stalled on a permission check is exactly as blocked as one stalled on a question, and it is the single most common way an unattended Agent silently stops making progress. Shipping without it leaves a hole where the product's core value is supposed to be.

Every one of these problems has been solved, or deliberately not solved, by at least a dozen tools already catalogued in `docs/refs.md`. Reading their solutions before implementing costs a few hours; discovering their failure modes ourselves costs a phase.

## Observed context

- Design decisions under investigation: `docs/prd.md` §4.1.2 (Claude Code discovery), §4.1.3 (Pi discovery and the pairing rule), §4.2.3 (question detection), §11 (known gaps 1 and 2).
- Domain terms these rules implement: `CONTEXT.md` — **Agent**, **Run**, **Lifecycle**, **Activity**.
- Reference catalog: `docs/refs.md` §1.2 (process introspection), §2.1 (Pi), §2.2 (Claude Code parsers), §3.1 (dashboard projects).
- Architectural Decision Record: none yet.
- No implementation exists; this ticket precedes Phase 1.

## Desired outcome

A written comparison of how existing multi-agent monitors solve liveness, pairing, and blocked-state detection, with each of the three rules above either confirmed, replaced, or explicitly retained with a recorded reason. Findings land in `docs/refs.md` as an annotated section, and any rule that changes is applied to `docs/prd.md` in the same pass.

The deliverable is a decision, not a survey. A reference that was read and found irrelevant is recorded as such so the next person does not re-read it.

## Acceptance criteria

- [ ] Read `github.com/neochoon/agenthud` (`docs/refs.md` §3.1) and record how it establishes agent liveness across Claude Code, Codex, OpenCode, and Kiro, and how its `agenthud follow` lifecycle event stream decides an agent has exited rather than gone quiet.
- [ ] Read `github.com/KyleAMathews/claude-code-ui` and `github.com/joe-re/eyes-on-claude-code` (`docs/refs.md` §3.1) and record exactly which on-disk or IPC signal each uses to produce its "Needs Approval" / "Waiting" / pending-user-input state. These are the two references most likely to have already solved known gap 1.
- [ ] Read `github.com/markojak/harness` and `github.com/onikan27/claude-code-monitor` (`docs/refs.md` §3.1) and record how each handles two concurrent sessions in one working directory, including whether either attempts process-to-session pairing at all.
- [ ] Read `github.com/janekbaraniewski/openusage` `internal/detect/` and `internal/providers/claude_code/` (`docs/refs.md` §1.1) and record its discovery and liveness approach in Go specifically.
- [ ] Search the four Pi-adjacent sources — `github.com/earendil-works/pi` (`docs/refs.md` §2.1), the local `harnesses/pi/.tmp/pi-source` checkout, Pi's `SessionManager.list` / `listAll` API, and Pi's own CLI surface — for any supported way to enumerate live sessions or map a process to a session, and record the answer as found or confirmed absent.
- [ ] Verify empirically whether `gopsutil/v4/process` `Cwd()` returns a working directory on darwin without cgo, using a throwaway Go program against a live `pi` process. Record the measured per-call cost against the `lsof` subprocess it would replace.
- [ ] Determine whether Pi logs delegated work the way Claude Code does. Claude Code writes each Subagent to `projects/<slug>/<sessionId>/subagents/agent-<agentId>.jsonl`, and on 2026-09-18 delegated work accounted for 87% of the busiest Agent's spend, so the equivalent question for Pi is not cosmetic. No `.pi-subagents` directory, no nested session directory, and no subagent reference in the vendored Pi source were found on 2026-09-18 — confirm that this is genuinely absent rather than merely unused on this machine, and record which.
- [ ] For each of the three rules in **Problem**, record one of: confirmed by prior art (naming the tool), replaced (stating the new rule and the evidence), or retained despite prior art (stating why the alternative was rejected).
- [ ] Add an annotated subsection to `docs/refs.md` capturing the findings, including references that were read and found irrelevant.
- [ ] Apply every rule change to `docs/prd.md` in the same pass, updating §4.1.3, §4.2.3, and §11 as needed, and update the `last_modified` frontmatter on both documents.
- [ ] If permission-prompt detection is solved by this investigation, remove it from `docs/prd.md` §11 and from the Deferred list in `docs/user-spec.md` §6, and add the detection rule to §4.2.3.
- [ ] Run a separate review pass on this ticket using an independent review workflow or review subagent, and resolve all identified feedback/issues until a completely clean review is returned.
