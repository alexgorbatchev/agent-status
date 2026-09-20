// Package parser provides streaming log parsers for AI agent harnesses (Claude Code, Pi).
//
// Attributions & Prior Art:
//   - Claude Code JSONL log parsing models and buffer sizing are derived from and inspired by:
//     Chitragupt (https://github.com/sonnes/chitragupt) - reader/claude/claude.go
//     OpenUsage (https://github.com/janekbaraniewski/openusage) - internal/providers/claude_code/
//     Claude Agent SDK Golang (https://github.com/ProjAnvil/claude-agent-sdk-golang)
//   - Pi session discovery, tree structure, and log models are derived from and inspired by:
//     Nick Nisi Sessions (https://github.com/nicknisi/sessions) - src/pi-tree.ts
//     OpenUsage (https://github.com/janekbaraniewski/openusage) - internal/providers/pi/session.go
package parser
