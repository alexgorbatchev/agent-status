# Agent Status

A pane of glass over the AI coding agents running across an engineer's machines. It observes agents, reports what they are doing, and signals when one needs attention.

## Language

**Agent**:
One continuous unit of work carried out by a coding agent on behalf of the engineer, from the moment it is first started to the moment it is abandoned. An Agent survives being resumed: stopping the program and picking the same work back up hours later continues the same Agent, with one unbroken history.
_Avoid_: using "agent" to mean the running program or the harness product. A single Agent may be carried by several program runs over its life, and the same harness carries many Agents.

**Harness**:
The coding agent product that carries an Agent — Claude Code or Pi. The harness is the software; the Agent is the work it is doing.
_Avoid_: "tool" (a tool is something an Agent invokes), "client", "CLI".

**Run**:
A single execution of the harness program carrying an Agent. An Agent has at least one Run; resuming starts another. Runs are how the Agent lives, not what it is.
_Avoid_: "process" when speaking about the domain, "instance", "session" (see below).

**Subagent**:
Work an Agent delegates to a helper it spawns. A Subagent acts on behalf of its Agent rather than on behalf of the engineer, so it is part of an Agent and never one itself. Its activity and its Usage belong to the Agent that spawned it.
_Avoid_: calling a Subagent an Agent, giving it its own card, or reporting an Agent's Usage without it — delegated work routinely dominates the total.

**Session**:
The harness's own word for what this project calls an Agent. Used only when describing harness behaviour or on-disk layout, never as a domain term.
_Avoid_: using "session" in product, UI, or API language. Say Agent.

**Lifecycle**:
Whether an Agent is being carried by a Run right now. An Agent is either Running or Exited. Exited is not an ending — a resumed Agent is Running again, and its history continues.
_Avoid_: "done", "finished", "complete", "dead". None of them are true: an Exited Agent may come back, and the work it was doing may have succeeded or failed.

**Activity**:
What an Agent is doing, independent of whether it is running: working, waiting on the engineer, errored, or idle between turns. Activity is remembered across an exit, so an Agent that died mid-error is still an errored Agent.
_Avoid_: collapsing Activity and Lifecycle into one word such as "status" or "state".

**Attention**:
The property of needing the engineer. It comes in two degrees: an Agent that is waiting on them right now, and an Agent that stopped in a state worth looking at. Derived from Lifecycle, Activity, and whether the Environment is Stale — never stored on its own.
_Avoid_: "blocked" (too narrow — an errored Agent needs attention and is not blocked), "alert" or "banner" (those are the UI treatment, not the property), and treating the two degrees as one list.

**Title**:
A short, stable name for what an Agent is working on, used to tell one Agent apart from another at a glance. Taken from the harness when it offers one, because the harness already knows. It changes rarely, only when the Agent's subject genuinely shifts.
_Avoid_: "name" (an Environment has a name; an Agent has a Title), and treating the Title as a description of current activity.

**Action**:
One sentence describing what an Agent is doing at this moment, derived from its current task and its recent tool use. It is expected to change constantly and is always about now, never about the Agent's overall purpose.
_Avoid_: "summary" — that word hides the difference between Title and Action and is why the two were conflated in the first place.

**Usage**:
The tokens an Agent has consumed, broken down by kind — fresh input, output, cache reads, cache writes, and reasoning. The breakdown is the unit, not a single number, because cache reads routinely outweigh fresh input several times over and a bare total hides that.
_Avoid_: "tokens" as a single figure, and any total that is only input plus output.

**Cost**:
What an Agent's Usage is worth in money, calculated the same way for every Agent from a published price list so that two Agents are always comparable. A harness's own cost figure is kept as recorded but is not what the dashboard reports. An Agent whose model is not in the price list has no Cost, which is shown as absent rather than as zero.
_Avoid_: presenting zero where the answer is unknown, and mixing a harness-reported figure into a fleet total alongside calculated ones.

**Stale**:
Said of an Environment that has stopped reporting. Its Agents are not known to have changed — they are simply no longer observed, and the dashboard says so rather than continuing to present their last known state as current.
_Avoid_: "offline" or "down" (the machine may be fine and the Agents still working), and applying Stale to an Agent — only an Environment goes Stale.

**Project**:
The codebase an Agent is working on, identified by where that codebase lives upstream rather than by where it happens to sit on any one machine. The same Project is one Project across every machine, every clone, and every Worktree.
_Avoid_: "repo" or "repository" when the machine-local copy is meant — that is a Worktree. Avoid naming a Project after a folder; folders lie, as with a checkout whose folder is called `.workspace`.

**Worktree**:
The particular checkout of a Project, on a particular machine, that an Agent is working in. Either a main clone or a git-linked worktree; the distinction matters to the engineer and is shown, but both are Worktrees.
_Avoid_: "workspace" (overloaded, and also a literal folder name in use), "directory", "branch" (a Worktree has a branch, it is not one).

**Environment**:
One person's Agents on one machine — the unit that observes and reports. Identified by the machine together with the user account, because an observer can only ever see the Agents belonging to its own account. Its name is a label the engineer chooses and may change at any time without the Environment becoming a different one.
_Avoid_: "host", "box", or "machine" as synonyms — one machine shared by two people is two Environments. Avoid treating the name as the identity.

**Working Directory**:
Where an Agent is operating at a given moment. It moves during an Agent's life and is a property of events, not of the Agent. It is never what an Agent is grouped by.
_Avoid_: treating the Working Directory as the Agent's location, or deriving the Project from it after the Agent has started.
