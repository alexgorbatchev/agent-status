---
created_on: 2026-09-21 13:56
last_modified: 2026-09-21 13:56
status: current
ticket_status: open
---

# Wave 1: Improve activity summarization with specialized prompting, semantic deduplication, and serialized execution

## Problem

The current activity summarizer in `air-ingester` (`packages/agent-ingester/internal/summarizer/summarizer.go`) has several structural and operational limitations that affect summarization quality and update stability:

1. **Monolithic Prompting Across Diverse Turn Contexts**: The summarizer uses a single static prompt template for all events, concatenating user prompt history, assistant responses, recent tool lists, raw thinking excerpts (up to 1,200 chars), and error messages into a single prompt. This produces noisy summaries when only a specific tool call or thinking chunk is relevant, and increases LLM latency on fast intermediate turns.
2. **Lack of Semantic / Token-Overlap Deduplication**: When successive agent events or thinking chunks produce minor phrasing variations describing the exact same underlying action (e.g., `"Editing file auth.go"` vs. `"Updating auth.go"`), the classifier updates the state and marks it fresh, causing unnecessary SSE broadcast churn and visual flicker in the UI dashboard.
3. **Potential Out-of-Order Completion Races**: The classifier currently executes summarization synchronously during ingestion (`c.summarizer.Summarize(context.Background(), input)` inside `updateSummarization`), but to avoid blocking ingestion handlers under heavy event streams, asynchronous summarization requests must execute sequentially per agent without racing or applying out-of-order state overwrites.
4. **Tool Argument Payload Slicing**: Large tool inputs (such as file write payloads or ast-grep patterns) are not cleanly sanitized or bounded before passing into the summarization context, occasionally inflating prompt tokens.

## Why this matters

The activity summary (`action`, `phase`, `activeTarget`) is the primary real-time status signal displayed on Agent cards across the web dashboard and CLI monitoring views. 

Improving prompt specialization generates sharper, concise, punchy present-continuous status updates (under 100 characters). Adding semantic word-overlap deduplication eliminates UI flickering from redundant re-phrasings. Serializing async summarization per agent ensures deterministic state updates without blocking high-throughput log ingestion.

## Observed context

- `packages/agent-ingester/internal/summarizer/summarizer.go`: Core LLM summarization engine, multi-provider failover (`openai`/LiteLLM, `gemini`, `anthropic`), rate limiting, and fallback rule generator.
- `packages/agent-ingester/internal/classifier/classifier.go`: Frame ingestion classifier mapping log frames (`tool_call`, `thinking`, `question_prompt`, `error`) to agent states and triggering `updateSummarization`.
- `packages/agent-common/types/types.go`: Canonical data models (`IAgentState`, `IAgentActivity`, `AgentPhase`, `ActivityState`).
- `packages/agent-ingester/internal/receiver/receiver.go`: WebSocket ingestion pipeline delivering parsed frames.

## Desired outcome

A refined summarization engine in `air-ingester` that:
1. Employs specialized, compact prompts based on available context (Thinking + Tool, Thinking only, or Tool execution only).
2. Uses token-based Jaccard similarity / word-overlap deduplication to skip near-duplicate summaries.
3. Provides a sequential, per-agent async work queue so summarization runs in background without blocking WebSocket ingestion or creating race conditions.
4. Maintains clean target extraction and robust fallback summaries when no LLM is configured or when calls time out.

## Acceptance criteria

- [ ] Refactor `packages/agent-ingester/internal/summarizer/summarizer.go` to support specialized prompt templates tailored to the specific active context:
  - Combined thinking excerpt and active tool execution with sanitized argument preview (max 300 chars).
  - Thinking excerpt only (concise status from agent reasoning).
  - Tool execution only (concise status from tool name and target/args).
- [ ] Implement a normalized word-overlap / Jaccard similarity deduplication helper (rejecting new summaries with $\ge 60\%$ word overlap with the last published summary) to prevent redundant state updates.
- [ ] Ensure `air-ingester` processes LLM summarization asynchronously without blocking the WebSocket receiver or tailer handlers, utilizing a serialized per-agent queue or channel to prevent out-of-order state overwrites.
- [ ] Ensure deterministic fallback summaries in `FallbackSummary` continue to handle all tool types, user questions, errors, and idle states correctly when external LLMs are disabled (`--no-summarize`) or fail.
- [ ] Update unit tests in `packages/agent-ingester/internal/summarizer/summarizer_test.go` and `packages/agent-ingester/internal/classifier/classifier_test.go` verifying specialized prompt generation, deduplication filtering, and async queue ordering with $\ge 90\%$ code coverage.
- [ ] Verify `just test-all` and `just lint-all` pass with zero errors.
- [ ] Run a separate review pass on this ticket using an independent review workflow or review subagent, and resolve all identified feedback/issues until a completely clean review is returned.
