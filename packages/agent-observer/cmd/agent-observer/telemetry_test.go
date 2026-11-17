package main

import (
	"context"
	"reflect"
	"testing"
	"time"

	parser "github.com/alexgorbatchev/agent-parser"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-observer/internal/git"
	watcher "github.com/alexgorbatchev/agent-watcher"
)

func TestTelemetryFramePreservesWatcherEvent(t *testing.T) {
	cwd := t.TempDir()
	subagent := "worker"
	text := "working"
	env := types.IEnvironmentInfo{ID: "machine", Label: "workstation"}
	event := watcher.Event{
		Harness: watcher.HarnessCodex, HarnessVersion: "1.2", SessionID: "session",
		SubagentID: &subagent, PID: 42,
		ParsedEvent: parser.ParsedEvent{EventType: "thinking", Timestamp: 123,
			CWD: cwd, GitBranch: "task", Data: parser.EventData{Content: &text}},
	}
	frame := telemetryFrame(context.Background(), env, git.NewResolver(time.Minute), event)
	if frame.AgentID != "machine:codex:session" || frame.EnvID != env.ID || !reflect.DeepEqual(frame.Environment, env) {
		t.Fatalf("incorrect environment identity: %+v", frame)
	}
	if frame.SessionID != event.SessionID || frame.SubagentID != event.SubagentID || frame.PID != event.PID || string(frame.Harness) != string(event.Harness) || frame.HarnessVersion != event.HarnessVersion {
		t.Fatalf("incorrect session identity: %+v", frame)
	}
	if frame.EventType != event.EventType || frame.Timestamp != event.Timestamp || !reflect.DeepEqual(frame.Data, event.Data) {
		t.Fatalf("parser event changed: %+v", frame)
	}
	if frame.Workspace.CWD != cwd || frame.Workspace.Branch != "task" {
		t.Fatalf("workspace metadata lost: %+v", frame.Workspace)
	}
}
