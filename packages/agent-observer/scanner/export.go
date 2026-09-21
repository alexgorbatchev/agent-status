package scanner

import (
	internalscanner "github.com/alexgorbatchev/agent-status/packages/agent-observer/internal/scanner"
)

type SessionDescriptor = internalscanner.SessionDescriptor
type SubagentTranscript = internalscanner.SubagentTranscript
type ClaudeCodeScanner = internalscanner.ClaudeCodeScanner

type PiSessionDescriptor = internalscanner.PiSessionDescriptor
type PiProcessInfo = internalscanner.PiProcessInfo
type PiHeader = internalscanner.PiHeader
type PiProcessFinder = internalscanner.PiProcessFinder
type PiScanner = internalscanner.PiScanner

type CodexSessionDescriptor = internalscanner.CodexSessionDescriptor
type CodexProcessInfo = internalscanner.CodexProcessInfo
type CodexSessionHeader = internalscanner.CodexSessionHeader
type CodexProcessFinder = internalscanner.CodexProcessFinder
type CodexScanner = internalscanner.CodexScanner

type OpencodeSessionDescriptor = internalscanner.OpencodeSessionDescriptor
type OpencodeProcessInfo = internalscanner.OpencodeProcessInfo
type OpencodeProcessFinder = internalscanner.OpencodeProcessFinder
type OpencodeScanner = internalscanner.OpencodeScanner

var (
	DefaultClaudeSessionDirs = internalscanner.DefaultClaudeSessionDirs
	NewClaudeCodeScanner    = internalscanner.NewClaudeCodeScanner

	DefaultPiSessionsDir = internalscanner.DefaultPiSessionsDir
	LivePiProcessFinder  = internalscanner.LivePiProcessFinder
	NewPiScanner         = internalscanner.NewPiScanner
	ReadPiSessionHeader  = internalscanner.ReadPiSessionHeader

	DefaultCodexSessionsDir = internalscanner.DefaultCodexSessionsDir
	LiveCodexProcessFinder  = internalscanner.LiveCodexProcessFinder
	NewCodexScanner         = internalscanner.NewCodexScanner
	ReadCodexSessionHeader  = internalscanner.ReadCodexSessionHeader

	DefaultOpencodeDBPath     = internalscanner.DefaultOpencodeDBPath
	LiveOpencodeProcessFinder = internalscanner.LiveOpencodeProcessFinder
	NewOpencodeScanner        = internalscanner.NewOpencodeScanner
)
