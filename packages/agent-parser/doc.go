// Package parser provides log parsers and database readers for AI agent harnesses (Claude Code, Pi, OpenCode).
//
// Attributions & Prior Art:
//   - Claude Code JSONL log parsing models and buffer sizing are derived from and inspired by:
//     Chitragupt (https://github.com/sonnes/chitragupt) - reader/claude/claude.go
//     OpenUsage (https://github.com/janekbaraniewski/openusage) - internal/providers/claude_code/
//     Claude Agent SDK Golang (https://github.com/ProjAnvil/claude-agent-sdk-golang)
//   - Pi session discovery, tree structure, and log models are derived from and inspired by:
//     Nick Nisi Sessions (https://github.com/nicknisi/sessions) - src/pi-tree.ts
//     OpenUsage (https://github.com/janekbaraniewski/openusage) - internal/providers/pi/session.go
//   - OpenCode SQLite storage reader, schema mappings, and turn reconstruction are derived from and inspired by:
//     OpenCode Storage Engine (https://github.com/anomalyco/opencode) - packages/opencode/src/storage/db.ts
//     agenthud (https://github.com/neochoon/agenthud) - src/data/providers/opencode.ts
//     opencode-db-search (https://github.com/rmk40/opencode-db-search) - src/db.ts
//     Nick Nisi Sessions (https://github.com/nicknisi/sessions) - src/opencode.ts
package parser
