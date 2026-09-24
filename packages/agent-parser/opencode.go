package parser

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/models"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	_ "github.com/ncruces/go-sqlite3/driver"
)

// DefaultOpencodeDBPath is the standard relative path to the OpenCode SQLite database within $HOME.
const DefaultOpencodeDBPath = ".local/share/opencode/opencode.db"

// OpencodeModelConfig represents the parsed JSON model configuration in session.model.
type OpencodeModelConfig struct {
	ID         string `json:"id"`
	ProviderID string `json:"providerID,omitempty"`
	Variant    string `json:"variant,omitempty"`
}

// OpencodeCacheTokens represents cache read/write token counts.
type OpencodeCacheTokens struct {
	Read  int64 `json:"read"`
	Write int64 `json:"write"`
}

// OpencodeTokens represents token usage metrics for a turn or session.
type OpencodeTokens struct {
	Total     int64               `json:"total"`
	Input     int64               `json:"input"`
	Output    int64               `json:"output"`
	Reasoning int64               `json:"reasoning,omitempty"`
	Cache     OpencodeCacheTokens `json:"cache"`
}

// OpencodeMessageTiming tracks turn start and completion timestamps.
type OpencodeMessageTiming struct {
	Created   int64  `json:"created"`
	Completed *int64 `json:"completed,omitempty"`
}

// OpencodeMessagePath describes the workspace and current working directory for a turn.
type OpencodeMessagePath struct {
	CWD  string `json:"cwd"`
	Root string `json:"root"`
}

// OpencodeMessageData represents the deserialized JSON payload from the message.data column.
type OpencodeMessageData struct {
	Role       string                 `json:"role"`
	ModelID    string                 `json:"modelID,omitempty"`
	ProviderID string                 `json:"providerID,omitempty"`
	Agent      string                 `json:"agent,omitempty"`
	Mode       string                 `json:"mode,omitempty"`
	Path       *OpencodeMessagePath   `json:"path,omitempty"`
	Cost       *float64               `json:"cost,omitempty"`
	Tokens     *OpencodeTokens        `json:"tokens,omitempty"`
	Time       *OpencodeMessageTiming `json:"time,omitempty"`
	Finish     *string                `json:"finish,omitempty"`
	Error      interface{}            `json:"error,omitempty"`
}

// OpencodeToolPartTime holds execution start and end timestamps.
type OpencodeToolPartTime struct {
	Start int64  `json:"start"`
	End   *int64 `json:"end,omitempty"`
}

// OpencodeToolPartState holds execution state, arguments, output, and status of a tool invocation.
type OpencodeToolPartState struct {
	Status   string                 `json:"status"` // "pending" | "running" | "completed" | "error"
	Title    string                 `json:"title,omitempty"`
	Input    map[string]interface{} `json:"input,omitempty"`
	Output   string                 `json:"output,omitempty"`
	Error    string                 `json:"error,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	Time     *OpencodeToolPartTime  `json:"time,omitempty"`
}

// OpencodePartData represents the deserialized JSON payload from the part.data column.
type OpencodePartData struct {
	Type     string                  `json:"type"` // "text" | "tool" | "reasoning" | "step-start" | "step-finish" | "file" | "patch"
	Text     string                  `json:"text,omitempty"`
	Tool     string                  `json:"tool,omitempty"`
	CallID   string                  `json:"callID,omitempty"`
	State    *OpencodeToolPartState  `json:"state,omitempty"`
	Reason   string                  `json:"reason,omitempty"`
	Cost     *float64                `json:"cost,omitempty"`
	Tokens   *OpencodeTokens         `json:"tokens,omitempty"`
	Path     string                  `json:"path,omitempty"`
	Content  string                  `json:"content,omitempty"`
	Snapshot map[string]interface{}  `json:"snapshot,omitempty"`
	Metadata map[string]interface{}  `json:"metadata,omitempty"`
	Time     *OpencodeToolPartTime   `json:"time,omitempty"`
	Error    interface{}             `json:"error,omitempty"`
	Message  string                  `json:"message,omitempty"`
}

// OpencodeSession represents a fully parsed OpenCode session.
type OpencodeSession struct {
	ID          string                 `json:"id"`
	ProjectID   *string                `json:"projectId,omitempty"`
	ParentID    *string                `json:"parentId,omitempty"`
	Slug        *string                `json:"slug,omitempty"`
	Directory   string                 `json:"directory"`
	Title       string                 `json:"title"`
	Version     string                 `json:"version"`
	Agent       string                 `json:"agent"`
	Model       string                 `json:"model"`
	ModelConfig *OpencodeModelConfig   `json:"modelConfig,omitempty"`
	Cost        float64                `json:"cost"`
	Tokens      OpencodeTokens         `json:"tokens"`
	TimeCreated int64                  `json:"timeCreated"`
	TimeUpdated int64                  `json:"timeUpdated"`
	TimeArchived *int64                `json:"timeArchived,omitempty"`
	IsArchived  bool                   `json:"isArchived"`
	IsSubagent  bool                   `json:"isSubagent"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// OpencodeMessage represents a row from the message table.
type OpencodeMessage struct {
	ID          string              `json:"id"`
	SessionID   string              `json:"sessionId"`
	TimeCreated int64               `json:"timeCreated"`
	Data        OpencodeMessageData `json:"data"`
}

// OpencodePart represents a row from the part table.
type OpencodePart struct {
	ID          string           `json:"id"`
	MessageID   string           `json:"messageId"`
	SessionID   string           `json:"sessionId"`
	TimeCreated int64            `json:"timeCreated"`
	Data        OpencodePartData `json:"data"`
}

// OpencodeProject represents a row from the project table.
type OpencodeProject struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Worktree *string `json:"worktree,omitempty"`
	VCS      *string `json:"vcs,omitempty"`
}

// OpencodePermission represents a row from the permission table.
type OpencodePermission struct {
	ID          string                 `json:"id"`
	SessionID   string                 `json:"sessionId"`
	Type        string                 `json:"type"`
	Data        map[string]interface{} `json:"data"`
	TimeCreated int64                  `json:"timeCreated"`
}

// ParseOpencodeModel parses the raw session.model text into a model ID and optional model config.
func ParseOpencodeModel(raw string) (string, *OpencodeModelConfig) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "unknown", nil
	}
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
		var cfg OpencodeModelConfig
		if err := json.Unmarshal([]byte(trimmed), &cfg); err == nil && cfg.ID != "" {
			return cfg.ID, &cfg
		}
	}
	return trimmed, nil
}

// ParseOpencodeMessageData unmarshals JSON text from message.data.
func ParseOpencodeMessageData(raw string) (*OpencodeMessageData, error) {
	var data OpencodeMessageData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, fmt.Errorf("unmarshaling opencode message data: %w", err)
	}
	return &data, nil
}

// ParseOpencodePartData unmarshals JSON text from part.data.
func ParseOpencodePartData(raw string) (*OpencodePartData, error) {
	var data OpencodePartData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, fmt.Errorf("unmarshaling opencode part data: %w", err)
	}
	return &data, nil
}

// OpencodeDB manages read-only queries against an OpenCode SQLite database.
type OpencodeDB struct {
	db *sql.DB
}

// OpenOpencodeDB opens an OpenCode SQLite database in read-only mode with a 5000ms busy timeout.
func OpenOpencodeDB(dbPath string) (*OpencodeDB, error) {
	var dsn string
	if dbPath == ":memory:" {
		dsn = "file::memory:?cache=shared"
	} else {
		dsn = "file:" + dbPath + "?mode=ro&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)"
	}

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening opencode database %q: %w", dbPath, err)
	}

	return NewOpencodeDB(db), nil
}

// NewOpencodeDB wraps an existing sql.DB handle.
func NewOpencodeDB(db *sql.DB) *OpencodeDB {
	return &OpencodeDB{db: db}
}

// DB returns the underlying database handle.
func (o *OpencodeDB) DB() *sql.DB {
	return o.db
}

// Close closes the database connection.
func (o *OpencodeDB) Close() error {
	if o.db != nil {
		return o.db.Close()
	}
	return nil
}

// GetSessions queries all sessions ordered by last updated timestamp descending.
func (o *OpencodeDB) GetSessions(ctx context.Context) ([]OpencodeSession, error) {
	const query = `
		SELECT
			id, project_id, parent_id, slug, directory, title,
			version, agent, model, cost, tokens_input, tokens_output,
			tokens_reasoning, tokens_cache_read, tokens_cache_write,
			time_created, time_updated, time_archived, metadata
		FROM session
		ORDER BY time_updated DESC;
	`
	rows, err := o.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying opencode sessions: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var sessions []OpencodeSession
	for rows.Next() {
		sess, err := scanOpencodeSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, *sess)
	}
	return sessions, rows.Err()
}

// GetSessionByID queries a single session by its unique ID.
func (o *OpencodeDB) GetSessionByID(ctx context.Context, id string) (*OpencodeSession, error) {
	const query = `
		SELECT
			id, project_id, parent_id, slug, directory, title,
			version, agent, model, cost, tokens_input, tokens_output,
			tokens_reasoning, tokens_cache_read, tokens_cache_write,
			time_created, time_updated, time_archived, metadata
		FROM session
		WHERE id = ?;
	`
	row := o.db.QueryRowContext(ctx, query, id)
	return scanOpencodeSessionRow(row)
}

// GetChildSessions queries all subagent sessions spawned by a parent session ID.
func (o *OpencodeDB) GetChildSessions(ctx context.Context, parentID string) ([]OpencodeSession, error) {
	const query = `
		SELECT
			id, project_id, parent_id, slug, directory, title,
			version, agent, model, cost, tokens_input, tokens_output,
			tokens_reasoning, tokens_cache_read, tokens_cache_write,
			time_created, time_updated, time_archived, metadata
		FROM session
		WHERE parent_id = ?
		ORDER BY time_created ASC;
	`
	rows, err := o.db.QueryContext(ctx, query, parentID)
	if err != nil {
		return nil, fmt.Errorf("querying opencode child sessions: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var sessions []OpencodeSession
	for rows.Next() {
		sess, err := scanOpencodeSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, *sess)
	}
	return sessions, rows.Err()
}

// GetMessages queries all messages for a session ordered by time created ascending.
func (o *OpencodeDB) GetMessages(ctx context.Context, sessionID string) ([]OpencodeMessage, error) {
	const query = `
		SELECT id, session_id, time_created, data
		FROM message
		WHERE session_id = ?
		ORDER BY time_created ASC;
	`
	rows, err := o.db.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying opencode messages: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var messages []OpencodeMessage
	for rows.Next() {
		var id, sessID, rawData string
		var timeCreated int64
		if err := rows.Scan(&id, &sessID, &timeCreated, &rawData); err != nil {
			return nil, fmt.Errorf("scanning opencode message: %w", err)
		}

		data, err := ParseOpencodeMessageData(rawData)
		if err != nil {
			return nil, err
		}

		messages = append(messages, OpencodeMessage{
			ID:          id,
			SessionID:   sessID,
			TimeCreated: timeCreated,
			Data:        *data,
		})
	}
	return messages, rows.Err()
}

// GetParts queries all parts for a session ordered by time created ascending.
func (o *OpencodeDB) GetParts(ctx context.Context, sessionID string) ([]OpencodePart, error) {
	const query = `
		SELECT id, message_id, session_id, time_created, data
		FROM part
		WHERE session_id = ?
		ORDER BY time_created ASC;
	`
	rows, err := o.db.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying opencode parts: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var parts []OpencodePart
	for rows.Next() {
		var id, messageID, sessID, rawData string
		var timeCreated int64
		if err := rows.Scan(&id, &messageID, &sessID, &timeCreated, &rawData); err != nil {
			return nil, fmt.Errorf("scanning opencode part: %w", err)
		}

		data, err := ParseOpencodePartData(rawData)
		if err != nil {
			return nil, err
		}

		parts = append(parts, OpencodePart{
			ID:          id,
			MessageID:   messageID,
			SessionID:   sessID,
			TimeCreated: timeCreated,
			Data:        *data,
		})
	}
	return parts, rows.Err()
}

// GetPartsByMessage queries all parts for a single message.
func (o *OpencodeDB) GetPartsByMessage(ctx context.Context, messageID string) ([]OpencodePart, error) {
	const query = `
		SELECT id, message_id, session_id, time_created, data
		FROM part
		WHERE message_id = ?
		ORDER BY time_created ASC;
	`
	rows, err := o.db.QueryContext(ctx, query, messageID)
	if err != nil {
		return nil, fmt.Errorf("querying opencode message parts: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var parts []OpencodePart
	for rows.Next() {
		var id, msgID, sessID, rawData string
		var timeCreated int64
		if err := rows.Scan(&id, &msgID, &sessID, &timeCreated, &rawData); err != nil {
			return nil, fmt.Errorf("scanning opencode message part: %w", err)
		}

		data, err := ParseOpencodePartData(rawData)
		if err != nil {
			return nil, err
		}

		parts = append(parts, OpencodePart{
			ID:          id,
			MessageID:   msgID,
			SessionID:   sessID,
			TimeCreated: timeCreated,
			Data:        *data,
		})
	}
	return parts, rows.Err()
}

// GetProjects queries all project records.
func (o *OpencodeDB) GetProjects(ctx context.Context) ([]OpencodeProject, error) {
	const query = `
		SELECT id, name, worktree, vcs
		FROM project
		ORDER BY id ASC;
	`
	rows, err := o.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying opencode projects: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var projects []OpencodeProject
	for rows.Next() {
		var id, name string
		var worktree, vcs sql.NullString
		if err := rows.Scan(&id, &name, &worktree, &vcs); err != nil {
			return nil, fmt.Errorf("scanning opencode project: %w", err)
		}
		p := OpencodeProject{
			ID:   id,
			Name: name,
		}
		if worktree.Valid {
			p.Worktree = &worktree.String
		}
		if vcs.Valid {
			p.VCS = &vcs.String
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// GetPendingPermissions queries pending permission requests for a session.
func (o *OpencodeDB) GetPendingPermissions(ctx context.Context, sessionID string) ([]OpencodePermission, error) {
	const query = `
		SELECT id, session_id, type, data, time_created
		FROM permission
		WHERE session_id = ?
		ORDER BY time_created ASC;
	`
	rows, err := o.db.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying opencode permissions: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var perms []OpencodePermission
	for rows.Next() {
		var id, sessID, permType, rawData string
		var timeCreated int64
		if err := rows.Scan(&id, &sessID, &permType, &rawData, &timeCreated); err != nil {
			return nil, fmt.Errorf("scanning opencode permission: %w", err)
		}

		var dataMap map[string]interface{}
		if rawData != "" {
			_ = json.Unmarshal([]byte(rawData), &dataMap)
		}

		perms = append(perms, OpencodePermission{
			ID:          id,
			SessionID:   sessID,
			Type:        permType,
			Data:        dataMap,
			TimeCreated: timeCreated,
		})
	}
	return perms, rows.Err()
}

// ReconstructSessionEvents queries session, message, and part rows and reconstructs them into chronological ParsedEvents.
func (o *OpencodeDB) ReconstructSessionEvents(ctx context.Context, sessionID string) ([]ParsedEvent, error) {
	session, err := o.GetSessionByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	messages, err := o.GetMessages(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	parts, err := o.GetParts(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	return ReconstructSessionEvents(session, messages, parts), nil
}

// ReconstructSessionEvents converts an OpenCode session, its messages, and parts into normalized ParsedEvents.
func ReconstructSessionEvents(session *OpencodeSession, messages []OpencodeMessage, parts []OpencodePart) []ParsedEvent {
	var events []ParsedEvent
	if session == nil {
		return events
	}

	// 1. Initial run_started event
	var titlePtr *string
	if session.Title != "" {
		titlePtr = stringPtr(session.Title)
	}
	var modelPtr *string
	var cwPtr *int64
	if session.Model != "" {
		modelPtr = stringPtr(session.Model)
		cw := models.ResolveContextWindow(session.Model)
		cwPtr = &cw
	}

	events = append(events, ParsedEvent{
		EventType: "run_started",
		Timestamp: session.TimeCreated,
		CWD:       session.Directory,
		Data: types.TelemetryFrameData{
			Title:              titlePtr,
			Model:              modelPtr,
			ModelContextWindow: cwPtr,
		},
	})

	// Index parts by message ID
	partsByMessage := make(map[string][]OpencodePart)
	for _, p := range parts {
		partsByMessage[p.MessageID] = append(partsByMessage[p.MessageID], p)
	}

	// 2. Process messages in chronological order
	for _, msg := range messages {
		cwd := session.Directory
		if msg.Data.Path != nil && msg.Data.Path.CWD != "" {
			cwd = msg.Data.Path.CWD
		}

		msgParts := partsByMessage[msg.ID]

		switch msg.Data.Role {
		case "user":
			var textParts []string
			for _, p := range msgParts {
				if p.Data.Type == "text" && p.Data.Text != "" {
					textParts = append(textParts, p.Data.Text)
				}
			}
			userText := strings.Join(textParts, "\n")
			if userText != "" {
				events = append(events, ParsedEvent{
					EventType: "user_prompt",
					Timestamp: msg.TimeCreated,
					CWD:       cwd,
					Data: types.TelemetryFrameData{
						Role:       stringPtr("user"),
						Content:    stringPtr(userText),
						LastPrompt: stringPtr(userText),
					},
				})
			}

		default:
			msgModel := msg.Data.ModelID
			if msgModel == "" {
				msgModel = session.Model
			}

			var errorPartTexts []string

			for _, p := range msgParts {
				partTs := p.TimeCreated
				switch p.Data.Type {
				case "reasoning":
					if p.Data.Text != "" {
						events = append(events, ParsedEvent{
							EventType: "thinking",
							Timestamp: partTs,
							CWD:       cwd,
							Data: types.TelemetryFrameData{
								Role:    stringPtr("assistant"),
								Content: stringPtr(p.Data.Text),
								Model:   stringPtr(msgModel),
							},
						})
					}

				case "text":
					if p.Data.Text != "" {
						events = append(events, ParsedEvent{
							EventType: "text",
							Timestamp: partTs,
							CWD:       cwd,
							Data: types.TelemetryFrameData{
								Role:    stringPtr("assistant"),
								Content: stringPtr(p.Data.Text),
								Model:   stringPtr(msgModel),
							},
						})
					}

				case "tool":
					if p.Data.Tool != "" || p.Data.CallID != "" {
						var toolInput map[string]interface{}
						var toolTitle *string
						startTs := partTs

						if p.Data.State != nil {
							toolInput = p.Data.State.Input
							if p.Data.State.Title != "" {
								toolTitle = stringPtr(p.Data.State.Title)
							}
							if p.Data.State.Time != nil && p.Data.State.Time.Start > 0 {
								startTs = p.Data.State.Time.Start
							}
						}

						fileMod := NormalizeFileModification(p.Data.Tool, toolInput)
						events = append(events, ParsedEvent{
							EventType: "tool_use",
							Timestamp: startTs,
							CWD:       cwd,
							Data: types.TelemetryFrameData{
								Role:             stringPtr("assistant"),
								ToolName:         stringPtr(p.Data.Tool),
								ToolCallID:       stringPtr(p.Data.CallID),
								ToolInput:        toolInput,
								FileModification: fileMod,
								Content:          toolTitle,
							},
						})

						// Emit tool_result if execution state is finalized or output is available
						if p.Data.State != nil && (p.Data.State.Status == "completed" || p.Data.State.Status == "error" || p.Data.State.Output != "" || p.Data.State.Error != "") {
							endTs := partTs
							if p.Data.State.Time != nil && p.Data.State.Time.End != nil && *p.Data.State.Time.End > 0 {
								endTs = *p.Data.State.Time.End
							}
							isError := p.Data.State.Status == "error"
							toolOutput := p.Data.State.Output
							if toolOutput == "" && p.Data.State.Error != "" {
								toolOutput = p.Data.State.Error
							}
							events = append(events, ParsedEvent{
								EventType: "tool_result",
								Timestamp: endTs,
								CWD:       cwd,
								Data: types.TelemetryFrameData{
									Role:       stringPtr("toolResult"),
									ToolCallID: stringPtr(p.Data.CallID),
									ToolOutput: stringPtr(toolOutput),
									IsError:    &isError,
								},
							})
						}
					}

				case "error":
					txt := p.Data.Text
					if txt == "" {
						txt = p.Data.Message
					}
					if txt == "" && p.Data.Error != nil {
						txt = extractOpencodeError(p.Data.Error)
					}
					if txt != "" {
						errorPartTexts = append(errorPartTexts, txt)
					}

				default:
					if p.Data.Error != nil {
						txt := extractOpencodeError(p.Data.Error)
						if txt != "" {
							errorPartTexts = append(errorPartTexts, txt)
						}
					}
				}
			}

			// Assistant turn end
			var usage *types.ITokenUsage
			if msg.Data.Tokens != nil {
				u := msg.Data.Tokens
				total := u.Total
				if total == 0 {
					total = u.Input + u.Output + u.Cache.Read + u.Cache.Write
				}
				usage = &types.ITokenUsage{
					Input:      u.Input,
					Output:     u.Output,
					CacheRead:  u.Cache.Read,
					CacheWrite: u.Cache.Write,
					Reasoning:  u.Reasoning,
					Total:      total,
				}
			}

			var stopReason *string
			if msg.Data.Finish != nil && *msg.Data.Finish != "" {
				stopReason = msg.Data.Finish
			}

			endTs := msg.TimeCreated
			if msg.Data.Time != nil && msg.Data.Time.Completed != nil && *msg.Data.Time.Completed > 0 {
				endTs = *msg.Data.Time.Completed
			} else {
				for _, p := range msgParts {
					if p.TimeCreated > endTs {
						endTs = p.TimeCreated
					}
				}
			}

			var contextWindowPtr *int64
			var fullnessPtr *float64
			if msgModel != "" {
				cw := models.ResolveContextWindow(msgModel)
				contextWindowPtr = &cw
				if usage != nil && cw > 0 {
					fn := models.CalculateContextFullness(usage.Total, cw)
					fullnessPtr = &fn
				}
			}

			if usage != nil || stopReason != nil || msg.Data.Cost != nil {
				events = append(events, ParsedEvent{
					EventType: "turn_end",
					Timestamp: endTs,
					CWD:       cwd,
					Data: types.TelemetryFrameData{
						Role:               stringPtr("assistant"),
						Model:              stringPtr(msgModel),
						ModelContextWindow: contextWindowPtr,
						ContextFullness:    fullnessPtr,
						StopReason:         stopReason,
						Usage:              usage,
						RawHarnessCost:     msg.Data.Cost,
					},
				})
			}

			hasError := (msg.Data.Finish != nil && *msg.Data.Finish == "error") ||
				msg.Data.Error != nil ||
				len(errorPartTexts) > 0

			if hasError {
				errText := ""
				if msg.Data.Error != nil {
					errText = extractOpencodeError(msg.Data.Error)
				}
				if errText == "" && len(errorPartTexts) > 0 {
					errText = strings.Join(errorPartTexts, "\n")
				}
				if errText == "" && msg.Data.Finish != nil && *msg.Data.Finish == "error" {
					errText = "error"
				}
				if errText == "" {
					errText = "error"
				}

				events = append(events, ParsedEvent{
					EventType: "error",
					Timestamp: endTs,
					CWD:       cwd,
					Data: types.TelemetryFrameData{
						Role:               stringPtr("assistant"),
						Content:            stringPtr(errText),
						Model:              stringPtr(msgModel),
						ModelContextWindow: contextWindowPtr,
						IsError:            boolPtr(true),
					},
				})
			}
		}
	}

	return events
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanOpencodeSession(s rowScanner) (*OpencodeSession, error) {
	var (
		id, directory                                string
		projectID, parentID, slug, title, version    sql.NullString
		agent, model, metadata                       sql.NullString
		cost                                         sql.NullFloat64
		tokensInput, tokensOutput, tokensReasoning   int64
		tokensCacheRead, tokensCacheWrite            int64
		timeCreated, timeUpdated                     int64
		timeArchived                                 sql.NullInt64
	)

	err := s.Scan(
		&id, &projectID, &parentID, &slug, &directory, &title,
		&version, &agent, &model, &cost, &tokensInput, &tokensOutput,
		&tokensReasoning, &tokensCacheRead, &tokensCacheWrite,
		&timeCreated, &timeUpdated, &timeArchived, &metadata,
	)
	if err != nil {
		return nil, fmt.Errorf("scanning opencode session: %w", err)
	}

	sess := &OpencodeSession{
		ID:          id,
		Directory:   directory,
		TimeCreated: timeCreated,
		TimeUpdated: timeUpdated,
		Tokens: OpencodeTokens{
			Total:     tokensInput + tokensOutput + tokensCacheRead + tokensCacheWrite,
			Input:     tokensInput,
			Output:    tokensOutput,
			Reasoning: tokensReasoning,
			Cache: OpencodeCacheTokens{
				Read:  tokensCacheRead,
				Write: tokensCacheWrite,
			},
		},
	}

	if projectID.Valid {
		sess.ProjectID = &projectID.String
	}
	if parentID.Valid {
		sess.ParentID = &parentID.String
		sess.IsSubagent = true
	}
	if slug.Valid {
		sess.Slug = &slug.String
	}
	if title.Valid {
		sess.Title = title.String
	}
	if version.Valid {
		sess.Version = version.String
	}
	if agent.Valid {
		sess.Agent = agent.String
	}
	if model.Valid {
		modelID, modelCfg := ParseOpencodeModel(model.String)
		sess.Model = modelID
		sess.ModelConfig = modelCfg
	} else {
		sess.Model = "unknown"
	}
	if cost.Valid {
		sess.Cost = cost.Float64
	}
	if timeArchived.Valid {
		sess.TimeArchived = &timeArchived.Int64
		sess.IsArchived = true
	}
	if metadata.Valid && metadata.String != "" {
		var meta map[string]interface{}
		if err := json.Unmarshal([]byte(metadata.String), &meta); err == nil {
			sess.Metadata = meta
		}
	}

	return sess, nil
}

func scanOpencodeSessionRow(row *sql.Row) (*OpencodeSession, error) {
	sess, err := scanOpencodeSession(row)
	if err != nil {
		return nil, err
	}
	return sess, nil
}

// extractOpencodeError extracts a human-readable error message from arbitrary error payloads
// serialized in OpenCode message data or part data (e.g. strings, NamedError maps, error objects).
func extractOpencodeError(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return strings.TrimSpace(val)
	case error:
		return strings.TrimSpace(val.Error())
	case map[string]interface{}:
		// 1. Check data field (e.g. NamedError: {name: "...", data: {message: "..."}})
		if dataVal, ok := val["data"]; ok && dataVal != nil {
			if dataMap, ok := dataVal.(map[string]interface{}); ok {
				if msg, ok := dataMap["message"].(string); ok && strings.TrimSpace(msg) != "" {
					return strings.TrimSpace(msg)
				}
				if exc, ok := dataMap["exception"].(string); ok && strings.TrimSpace(exc) != "" {
					return strings.TrimSpace(exc)
				}
				if errStr, ok := dataMap["error"].(string); ok && strings.TrimSpace(errStr) != "" {
					return strings.TrimSpace(errStr)
				}
			} else if dataStr, ok := dataVal.(string); ok && strings.TrimSpace(dataStr) != "" {
				return strings.TrimSpace(dataStr)
			}
		}
		// 2. Check message field
		if msg, ok := val["message"].(string); ok && strings.TrimSpace(msg) != "" {
			return strings.TrimSpace(msg)
		}
		// 3. Check error field
		if errVal, ok := val["error"]; ok && errVal != nil {
			if sub := extractOpencodeError(errVal); sub != "" {
				return sub
			}
		}
		// 4. Check exception field
		if excVal, ok := val["exception"]; ok && excVal != nil {
			if sub := extractOpencodeError(excVal); sub != "" {
				return sub
			}
		}
		// 5. Check text field
		if txt, ok := val["text"].(string); ok && strings.TrimSpace(txt) != "" {
			return strings.TrimSpace(txt)
		}
		// 6. Check details field
		if det, ok := val["details"].(string); ok && strings.TrimSpace(det) != "" {
			return strings.TrimSpace(det)
		}
		// 7. Check name field
		if name, ok := val["name"].(string); ok && strings.TrimSpace(name) != "" {
			return strings.TrimSpace(name)
		}
		// Fallback to JSON serialization if non-empty
		b, err := json.Marshal(val)
		if err == nil && string(b) != "{}" {
			return string(b)
		}
	case []interface{}:
		var parts []string
		for _, item := range val {
			if s := extractOpencodeError(item); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "\n")
	}
	return fmt.Sprintf("%v", v)
}

