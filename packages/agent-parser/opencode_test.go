package parser

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
)

func TestParseOpencodeModel(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantID      string
		wantHasConf bool
	}{
		{
			name:        "empty string",
			input:       "",
			wantID:      "unknown",
			wantHasConf: false,
		},
		{
			name:        "plain string model ID",
			input:       "claude-3-7-sonnet",
			wantID:      "claude-3-7-sonnet",
			wantHasConf: false,
		},
		{
			name:        "json object model config",
			input:       `{"id":"gpt-5.5","providerID":"openai","variant":"high"}`,
			wantID:      "gpt-5.5",
			wantHasConf: true,
		},
		{
			name:        "invalid json fallback",
			input:       `{"id":`,
			wantID:      `{"id":`,
			wantHasConf: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, cfg := ParseOpencodeModel(tt.input)
			if id != tt.wantID {
				t.Errorf("ParseOpencodeModel(%q) id = %q, want %q", tt.input, id, tt.wantID)
			}
			if (cfg != nil) != tt.wantHasConf {
				t.Errorf("ParseOpencodeModel(%q) config presence = %v, want %v", tt.input, cfg != nil, tt.wantHasConf)
			}
		})
	}
}

func TestParseOpencodeMessageData(t *testing.T) {
	rawUser := `{
		"role": "user",
		"agent": "build",
		"mode": "code",
		"path": {
			"cwd": "/workspace/project",
			"root": "/workspace"
		}
	}`

	msg, err := ParseOpencodeMessageData(rawUser)
	if err != nil {
		t.Fatalf("ParseOpencodeMessageData user error = %v", err)
	}
	if msg.Role != "user" {
		t.Errorf("expected role user, got %q", msg.Role)
	}
	if msg.Path == nil || msg.Path.CWD != "/workspace/project" {
		t.Errorf("expected cwd /workspace/project, got %+v", msg.Path)
	}

	rawAssistant := `{
		"role": "assistant",
		"modelID": "claude-3-7-sonnet",
		"providerID": "anthropic",
		"cost": 0.0152,
		"tokens": {
			"total": 1250,
			"input": 1000,
			"output": 250,
			"reasoning": 100,
			"cache": {
				"read": 500,
				"write": 0
			}
		},
		"time": {
			"created": 1700000000000,
			"completed": 1700000005000
		},
		"finish": "tool_use"
	}`

	asstMsg, err := ParseOpencodeMessageData(rawAssistant)
	if err != nil {
		t.Fatalf("ParseOpencodeMessageData assistant error = %v", err)
	}
	if asstMsg.Role != "assistant" || asstMsg.ModelID != "claude-3-7-sonnet" {
		t.Errorf("unexpected role or model: %+v", asstMsg)
	}
	if asstMsg.Cost == nil || *asstMsg.Cost != 0.0152 {
		t.Errorf("expected cost 0.0152, got %v", asstMsg.Cost)
	}
	if asstMsg.Tokens == nil || asstMsg.Tokens.Total != 1250 || asstMsg.Tokens.Reasoning != 100 {
		t.Errorf("unexpected tokens: %+v", asstMsg.Tokens)
	}
	if asstMsg.Finish == nil || *asstMsg.Finish != "tool_use" {
		t.Errorf("expected finish tool_use, got %v", asstMsg.Finish)
	}
}

func TestParseOpencodePartData(t *testing.T) {
	rawTool := `{
		"type": "tool",
		"tool": "edit",
		"callID": "call_12345",
		"state": {
			"status": "completed",
			"title": "src/main.ts",
			"input": {
				"path": "src/main.ts",
				"command": "update"
			},
			"output": "File updated successfully",
			"time": {
				"start": 1700000001000,
				"end": 1700000002000
			}
		}
	}`

	part, err := ParseOpencodePartData(rawTool)
	if err != nil {
		t.Fatalf("ParseOpencodePartData error = %v", err)
	}
	if part.Type != "tool" || part.Tool != "edit" || part.CallID != "call_12345" {
		t.Errorf("unexpected part data: %+v", part)
	}
	if part.State == nil || part.State.Status != "completed" || part.State.Output != "File updated successfully" {
		t.Errorf("unexpected state: %+v", part.State)
	}

	rawReasoning := `{
		"type": "reasoning",
		"text": "Analyzing the build structure to identify root cause"
	}`
	reasoningPart, err := ParseOpencodePartData(rawReasoning)
	if err != nil {
		t.Fatalf("ParseOpencodePartData reasoning error = %v", err)
	}
	if reasoningPart.Type != "reasoning" || reasoningPart.Text != "Analyzing the build structure to identify root cause" {
		t.Errorf("unexpected reasoning part: %+v", reasoningPart)
	}
}

func setupTestOpencodeDB(t *testing.T) *OpencodeDB {
	db, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("opening in-memory sqlite: %v", err)
	}

	schema := `
		CREATE TABLE project (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			worktree TEXT,
			vcs TEXT
		);

		CREATE TABLE session (
			id TEXT PRIMARY KEY,
			project_id TEXT,
			parent_id TEXT,
			slug TEXT,
			directory TEXT NOT NULL,
			title TEXT,
			version TEXT,
			agent TEXT,
			model TEXT,
			cost REAL,
			tokens_input INTEGER NOT NULL DEFAULT 0,
			tokens_output INTEGER NOT NULL DEFAULT 0,
			tokens_reasoning INTEGER NOT NULL DEFAULT 0,
			tokens_cache_read INTEGER NOT NULL DEFAULT 0,
			tokens_cache_write INTEGER NOT NULL DEFAULT 0,
			time_created INTEGER NOT NULL,
			time_updated INTEGER NOT NULL,
			time_archived INTEGER,
			metadata TEXT
		);

		CREATE TABLE message (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			time_created INTEGER NOT NULL,
			data TEXT NOT NULL
		);

		CREATE TABLE part (
			id TEXT PRIMARY KEY,
			message_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			time_created INTEGER NOT NULL,
			data TEXT NOT NULL
		);

		CREATE TABLE permission (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			type TEXT NOT NULL,
			data TEXT NOT NULL,
			time_created INTEGER NOT NULL
		);
	`
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		t.Fatalf("creating test schema: %v", err)
	}

	return NewOpencodeDB(db)
}

func TestOpencodeDB_QueriesAndReconstruction(t *testing.T) {
	odb := setupTestOpencodeDB(t)
	defer func() {
		_ = odb.Close()
	}()

	ctx := context.Background()

	// 1. Insert project
	_, err := odb.DB().Exec(`
		INSERT INTO project (id, name, worktree, vcs)
		VALUES ('proj_1', 'agent-status', '/workspace/agent-status', 'git');
	`)
	if err != nil {
		t.Fatalf("inserting project: %v", err)
	}

	// 2. Insert parent session
	_, err = odb.DB().Exec(`
		INSERT INTO session (
			id, project_id, parent_id, slug, directory, title,
			version, agent, model, cost, tokens_input, tokens_output,
			tokens_reasoning, tokens_cache_read, tokens_cache_write,
			time_created, time_updated, time_archived, metadata
		) VALUES (
			'ses_parent', 'proj_1', NULL, 'fix-session-parser', '/workspace/agent-status', 'Fix Session Parser',
			'0.4.1', 'build', '{"id":"claude-3-7-sonnet","providerID":"anthropic"}', 0.045, 2000, 500,
			200, 1000, 0, 1700000000000, 1700000010000, NULL, '{"priority":"high"}'
		);
	`)
	if err != nil {
		t.Fatalf("inserting parent session: %v", err)
	}

	// 3. Insert child subagent session
	_, err = odb.DB().Exec(`
		INSERT INTO session (
			id, project_id, parent_id, slug, directory, title,
			version, agent, model, cost, tokens_input, tokens_output,
			tokens_reasoning, tokens_cache_read, tokens_cache_write,
			time_created, time_updated, time_archived, metadata
		) VALUES (
			'ses_child', 'proj_1', 'ses_parent', 'subagent-task', '/workspace/agent-status', 'Subagent Task',
			'0.4.1', 'default', 'gpt-5.5', 0.012, 800, 200,
			0, 0, 0, 1700000005000, 1700000008000, NULL, NULL
		);
	`)
	if err != nil {
		t.Fatalf("inserting child session: %v", err)
	}

	// 4. Insert messages for parent session
	userMsgData := `{"role":"user","mode":"code","path":{"cwd":"/workspace/agent-status","root":"/workspace"}}`
	_, err = odb.DB().Exec(`
		INSERT INTO message (id, session_id, time_created, data)
		VALUES ('msg_u1', 'ses_parent', 1700000001000, ?);
	`, userMsgData)
	if err != nil {
		t.Fatalf("inserting user message: %v", err)
	}

	asstMsgData := `{
		"role":"assistant",
		"modelID":"claude-3-7-sonnet",
		"cost":0.045,
		"tokens":{"total":2700,"input":2000,"output":500,"reasoning":200,"cache":{"read":1000,"write":0}},
		"time":{"created":1700000002000,"completed":1700000008000},
		"finish":"stop"
	}`
	_, err = odb.DB().Exec(`
		INSERT INTO message (id, session_id, time_created, data)
		VALUES ('msg_a1', 'ses_parent', 1700000002000, ?);
	`, asstMsgData)
	if err != nil {
		t.Fatalf("inserting assistant message: %v", err)
	}

	// 5. Insert parts for user and assistant messages
	userPartData := `{"type":"text","text":"Please fix the parser tests"}`
	_, err = odb.DB().Exec(`
		INSERT INTO part (id, message_id, session_id, time_created, data)
		VALUES ('part_u1', 'msg_u1', 'ses_parent', 1700000001050, ?);
	`, userPartData)
	if err != nil {
		t.Fatalf("inserting user text part: %v", err)
	}

	reasoningPartData := `{"type":"reasoning","text":"Let me read the test failure logs first."}`
	_, err = odb.DB().Exec(`
		INSERT INTO part (id, message_id, session_id, time_created, data)
		VALUES ('part_r1', 'msg_a1', 'ses_parent', 1700000002100, ?);
	`, reasoningPartData)
	if err != nil {
		t.Fatalf("inserting reasoning part: %v", err)
	}

	toolPartData := `{
		"type":"tool",
		"tool":"read",
		"callID":"call_read_1",
		"state":{
			"status":"completed",
			"title":"packages/agent-parser/pi_test.go",
			"input":{"path":"packages/agent-parser/pi_test.go"},
			"output":"package parser\n\nfunc TestPi(t *testing.T) {}",
			"time":{"start":1700000003000,"end":1700000004000}
		}
	}`
	_, err = odb.DB().Exec(`
		INSERT INTO part (id, message_id, session_id, time_created, data)
		VALUES ('part_t1', 'msg_a1', 'ses_parent', 1700000003000, ?);
	`, toolPartData)
	if err != nil {
		t.Fatalf("inserting tool part: %v", err)
	}

	asTextPartData := `{"type":"text","text":"I have reviewed the test file and identified the issue."}`
	_, err = odb.DB().Exec(`
		INSERT INTO part (id, message_id, session_id, time_created, data)
		VALUES ('part_text1', 'msg_a1', 'ses_parent', 1700000005000, ?);
	`, asTextPartData)
	if err != nil {
		t.Fatalf("inserting assistant text part: %v", err)
	}

	// 6. Insert pending permission
	_, err = odb.DB().Exec(`
		INSERT INTO permission (id, session_id, type, data, time_created)
		VALUES ('perm_1', 'ses_parent', 'tool_execution', '{"tool":"bash","command":"rm -rf /tmp/junk"}', 1700000009000);
	`)
	if err != nil {
		t.Fatalf("inserting permission: %v", err)
	}

	// Verify Projects Query
	projects, err := odb.GetProjects(ctx)
	if err != nil {
		t.Fatalf("GetProjects error = %v", err)
	}
	if len(projects) != 1 || projects[0].Name != "agent-status" {
		t.Errorf("unexpected projects: %+v", projects)
	}

	// Verify Sessions Query
	sessions, err := odb.GetSessions(ctx)
	if err != nil {
		t.Fatalf("GetSessions error = %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	// Verify Session By ID
	parentSess, err := odb.GetSessionByID(ctx, "ses_parent")
	if err != nil {
		t.Fatalf("GetSessionByID(ses_parent) error = %v", err)
	}
	if parentSess.Title != "Fix Session Parser" || parentSess.Model != "claude-3-7-sonnet" || parentSess.IsSubagent {
		t.Errorf("unexpected parent session: %+v", parentSess)
	}
	if parentSess.Cost != 0.045 || parentSess.Tokens.Total != 3500 { // 2000 input + 500 output + 1000 cacheRead + 0 cacheWrite = 3500
		t.Errorf("unexpected tokens/cost: tokens=%+v, cost=%f", parentSess.Tokens, parentSess.Cost)
	}

	// Verify Child Sessions Query
	children, err := odb.GetChildSessions(ctx, "ses_parent")
	if err != nil {
		t.Fatalf("GetChildSessions error = %v", err)
	}
	if len(children) != 1 || children[0].ID != "ses_child" || !children[0].IsSubagent || children[0].ParentID == nil || *children[0].ParentID != "ses_parent" {
		t.Errorf("unexpected child sessions: %+v", children)
	}

	// Verify Messages Query
	messages, err := odb.GetMessages(ctx, "ses_parent")
	if err != nil {
		t.Fatalf("GetMessages error = %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	// Verify Parts Query
	parts, err := odb.GetParts(ctx, "ses_parent")
	if err != nil {
		t.Fatalf("GetParts error = %v", err)
	}
	if len(parts) != 4 {
		t.Fatalf("expected 4 parts, got %d", len(parts))
	}

	// Verify Pending Permissions Query
	perms, err := odb.GetPendingPermissions(ctx, "ses_parent")
	if err != nil {
		t.Fatalf("GetPendingPermissions error = %v", err)
	}
	if len(perms) != 1 || perms[0].Type != "tool_execution" {
		t.Errorf("unexpected permissions: %+v", perms)
	}

	// Verify Event Reconstruction
	events, err := odb.ReconstructSessionEvents(ctx, "ses_parent")
	if err != nil {
		t.Fatalf("ReconstructSessionEvents error = %v", err)
	}

	expectedEventTypes := []string{
		"run_started",
		"user_prompt",
		"thinking",
		"tool_use",
		"tool_result",
		"text",
		"turn_end",
	}

	if len(events) != len(expectedEventTypes) {
		t.Fatalf("expected %d events, got %d:\n%+v", len(expectedEventTypes), len(events), events)
	}

	for i, expectedType := range expectedEventTypes {
		if events[i].EventType != expectedType {
			t.Errorf("event[%d] type = %q, want %q", i, events[i].EventType, expectedType)
		}
	}

	// Inspect specific reconstructed event data
	runStarted := events[0]
	if runStarted.Data.Title == nil || *runStarted.Data.Title != "Fix Session Parser" {
		t.Errorf("run_started title = %v, want 'Fix Session Parser'", runStarted.Data.Title)
	}
	if runStarted.Data.Model == nil || *runStarted.Data.Model != "claude-3-7-sonnet" {
		t.Errorf("run_started model = %v, want 'claude-3-7-sonnet'", runStarted.Data.Model)
	}

	userPrompt := events[1]
	if userPrompt.Data.Content == nil || *userPrompt.Data.Content != "Please fix the parser tests" {
		t.Errorf("user_prompt content = %v", userPrompt.Data.Content)
	}

	thinking := events[2]
	if thinking.Data.Content == nil || *thinking.Data.Content != "Let me read the test failure logs first." {
		t.Errorf("thinking content = %v", thinking.Data.Content)
	}

	toolUse := events[3]
	if toolUse.Data.ToolName == nil || *toolUse.Data.ToolName != "read" || toolUse.Data.ToolCallID == nil || *toolUse.Data.ToolCallID != "call_read_1" {
		t.Errorf("tool_use data = %+v", toolUse.Data)
	}

	toolResult := events[4]
	if toolResult.Data.ToolCallID == nil || *toolResult.Data.ToolCallID != "call_read_1" || toolResult.Data.ToolOutput == nil || *toolResult.Data.ToolOutput != "package parser\n\nfunc TestPi(t *testing.T) {}" {
		t.Errorf("tool_result data = %+v", toolResult.Data)
	}
	if toolResult.Data.IsError == nil || *toolResult.Data.IsError {
		t.Errorf("tool_result isError = %v, want false", toolResult.Data.IsError)
	}

	textEvent := events[5]
	if textEvent.Data.Content == nil || *textEvent.Data.Content != "I have reviewed the test file and identified the issue." {
		t.Errorf("text event content = %v", textEvent.Data.Content)
	}

	turnEnd := events[6]
	if turnEnd.Data.Usage == nil || turnEnd.Data.Usage.Total != 2700 {
		t.Errorf("turn_end usage = %+v", turnEnd.Data.Usage)
	}
	if turnEnd.Data.RawHarnessCost == nil || *turnEnd.Data.RawHarnessCost != 0.045 {
		t.Errorf("turn_end cost = %v", turnEnd.Data.RawHarnessCost)
	}
	if turnEnd.Data.StopReason == nil || *turnEnd.Data.StopReason != "stop" {
		t.Errorf("turn_end stopReason = %v", turnEnd.Data.StopReason)
	}
}

func TestOpencodeDB_ToolResultErrorStatus(t *testing.T) {
	session := &OpencodeSession{
		ID:          "ses_err",
		Directory:   "/test",
		Title:       "Error Test",
		Model:       "gpt-5.5",
		TimeCreated: time.Now().UnixMilli(),
	}

	messages := []OpencodeMessage{
		{
			ID:          "msg_1",
			SessionID:   "ses_err",
			TimeCreated: time.Now().UnixMilli(),
			Data: OpencodeMessageData{
				Role: "assistant",
			},
		},
	}

	errOutput := "command not found: foobar"
	parts := []OpencodePart{
		{
			ID:          "part_err",
			MessageID:   "msg_1",
			SessionID:   "ses_err",
			TimeCreated: time.Now().UnixMilli(),
			Data: OpencodePartData{
				Type:   "tool",
				Tool:   "bash",
				CallID: "call_bash_err",
				State: &OpencodeToolPartState{
					Status: "error",
					Output: errOutput,
				},
			},
		},
	}

	events := ReconstructSessionEvents(session, messages, parts)
	var foundResult *ParsedEvent
	for i := range events {
		if events[i].EventType == "tool_result" {
			foundResult = &events[i]
			break
		}
	}

	if foundResult == nil {
		t.Fatalf("expected tool_result event")
	}
	if foundResult.Data.IsError == nil || !*foundResult.Data.IsError {
		t.Errorf("expected isError = true for failed tool execution, got %v", foundResult.Data.IsError)
	}
}
