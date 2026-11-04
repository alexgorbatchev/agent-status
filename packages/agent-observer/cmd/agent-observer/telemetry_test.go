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

func TestTelemetryFramePreservesCodexToolOutput(t *testing.T) {
	for _, kind := range []string{"function_call_output", "custom_tool_call_output"} {
		for _, output := range []string{`"completed"`, `[{"type":"input_text","text":"first"},{"type":"input_text","text":"second"}]`} {
			t.Run(kind+"/"+output, func(t *testing.T) {
				line := []byte(`{"timestamp":"2026-10-29T23:08:21Z","type":"response_item","payload":{"type":"` + kind + `","call_id":"call-1","output":` + output + `}}`)
				events, err := parser.NewCodexParser().ParseLine(line)
				if err != nil {
					t.Fatalf("parsing Codex tool output: %v", err)
				}
				if len(events) != 1 {
					t.Fatalf("got %d events, want one tool result", len(events))
				}
				frame := telemetryFrame(context.Background(), types.IEnvironmentInfo{ID: "machine"}, git.NewResolver(time.Minute), watcher.Event{
					Harness: watcher.HarnessCodex, SessionID: "session", ParsedEvent: events[0],
				})
				want := output
				if output == `"completed"` {
					want = "completed"
				}
				if frame.EventType != "tool_result" || frame.Data.ToolCallID == nil || *frame.Data.ToolCallID != "call-1" || frame.Data.ToolOutput == nil || *frame.Data.ToolOutput != want {
					t.Fatalf("tool result lost or changed: %+v", frame.Data)
				}
			})
		}
	}
}
