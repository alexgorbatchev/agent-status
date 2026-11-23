package store

import (
	"database/sql"
	"strings"
	"testing"
)

func openV8Fixture(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	_, err = db.Exec(schemaV1 + `
ALTER TABLE agents ADD COLUMN is_hidden INTEGER NOT NULL DEFAULT 0;
ALTER TABLE agents ADD COLUMN hidden_at INTEGER;
ALTER TABLE agents ADD COLUMN active_tool_started_at INTEGER;
ALTER TABLE agents ADD COLUMN active_subagents TEXT;
ALTER TABLE agents ADD COLUMN model TEXT;
ALTER TABLE agents ADD COLUMN context_window INTEGER;
ALTER TABLE agents ADD COLUMN context_fullness REAL;
PRAGMA user_version = 5;
PRAGMA foreign_keys = ON;`)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateV6(db); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
ALTER TABLE agents ADD COLUMN rate_limit_reset_at INTEGER;
ALTER TABLE agents ADD COLUMN retry_after_seconds INTEGER;
ALTER TABLE agents ADD COLUMN has_unseen_completion INTEGER NOT NULL DEFAULT 0;
ALTER TABLE agents ADD COLUMN completed_at INTEGER;
PRAGMA user_version = 8;
INSERT INTO environments VALUES ('env', 'Local', 'host', 'linux', 'user', 0, 1000);
INSERT INTO runs VALUES ('run', 'agent', 42, '1.0', 1000, NULL);
INSERT INTO agents (id, session_id, environment_id, harness, project_id, project_name,
canonical_root, branch, cwd, current_run_id, activity, title, tokens_input,
first_started_at, updated_at, is_hidden, model, rate_limit_reset_at,
retry_after_seconds, has_unseen_completion, completed_at)
VALUES ('agent', 'session', 'env', 'codex', 'project', 'Project', '/repo', 'main',
'/repo', 'run', 'RATE_LIMITED', 'Existing session', 321, 1000, 2000, 1, 'model', 9000, 7, 1, 1500);
INSERT INTO telemetry_events (agent_id, event_type, timestamp, data_json) VALUES ('agent', 'message', 1200, '{}');
CREATE INDEX custom_agents_title ON agents(title);
CREATE TABLE title_updates (title TEXT);
CREATE TRIGGER capture_title AFTER UPDATE OF title ON agents BEGIN INSERT INTO title_updates VALUES (NEW.title); END;
`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestMigrateV9PreservesV8DataAndAdmitsAgy(t *testing.T) {
	db := openV8Fixture(t)
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("second migration: %v", err)
	}
	var version, fk int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatal(err)
	}
	if version != 9 || fk != 1 {
		t.Fatalf("version=%d foreign_keys=%d", version, fk)
	}
	var title, harness, run, model string
	var input, hidden, reset, retry, unseen, completed int
	err := db.QueryRow(`SELECT title,harness,current_run_id,model,tokens_input,is_hidden,rate_limit_reset_at,retry_after_seconds,has_unseen_completion,completed_at FROM agents WHERE id='agent'`).Scan(&title, &harness, &run, &model, &input, &hidden, &reset, &retry, &unseen, &completed)
	if err != nil {
		t.Fatal(err)
	}
	if title != "Existing session" || harness != "codex" || run != "run" || model != "model" || input != 321 || hidden != 1 || reset != 9000 || retry != 7 || unseen != 1 || completed != 1500 {
		t.Fatalf("migration changed existing agent: %s %s %s %s %d %d %d %d %d %d", title, harness, run, model, input, hidden, reset, retry, unseen, completed)
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM telemetry_events WHERE agent_id='agent'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("telemetry count=%d err=%v", count, err)
	}
	if _, err := db.Exec("UPDATE agents SET harness='agy', title='Antigravity' WHERE id='agent'"); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT title FROM title_updates").Scan(&title); err != nil || title != "Antigravity" {
		t.Fatalf("trigger title=%q err=%v", title, err)
	}
	if err := db.QueryRow("SELECT count(*) FROM sqlite_schema WHERE name='custom_agents_title'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("index count=%d err=%v", count, err)
	}
	if _, err := db.Exec("UPDATE agents SET harness='unknown'"); err == nil {
		t.Fatal("unknown harness accepted")
	}
	if _, err := db.Exec("UPDATE agents SET environment_id='missing'"); err == nil {
		t.Fatal("foreign key enforcement lost")
	}
	if _, err := db.Exec("DELETE FROM environments WHERE id='env'"); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT count(*) FROM telemetry_events").Scan(&count); err != nil || count != 0 {
		t.Fatalf("cascade count=%d err=%v", count, err)
	}
}

func TestMigrateV9RollsBackForeignKeyViolation(t *testing.T) {
	db := openV8Fixture(t)
	if _, err := db.Exec("PRAGMA foreign_keys=OFF; UPDATE agents SET environment_id='missing'; PRAGMA foreign_keys=ON;"); err != nil {
		t.Fatal(err)
	}
	if err := migrateV9(db); err == nil {
		t.Fatal("corrupt foreign key was accepted")
	}
	var version, count, fk int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT count(*) FROM telemetry_events").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatal(err)
	}
	if version != 8 || count != 1 || fk != 1 {
		t.Fatalf("rollback version=%d events=%d fk=%d", version, count, fk)
	}
}

func TestMigrateV9RejectsUnexpectedSchema(t *testing.T) {
	db := openV8Fixture(t)
	if _, err := db.Exec("DROP TRIGGER capture_title; DROP TABLE agents; CREATE TABLE agents(id TEXT)"); err != nil {
		t.Fatal(err)
	}
	if err := migrateV9(db); err == nil {
		t.Fatal("unexpected schema accepted")
	}
}

func TestMigrateV9StorageFailuresPreserveDatabase(t *testing.T) {
	for _, tc := range []struct{ name, setup, want string }{
		{"missing agents", "DROP TRIGGER capture_title; DROP TABLE agents", "reading agents schema"},
		{"conflicting table", "CREATE TABLE agents_new(id TEXT)", "rebuilding agents"},
		{"invalid foreign key schema", "CREATE TABLE invalid_reference(parent TEXT REFERENCES agents(title))", "checking foreign keys"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openV8Fixture(t)
			if _, err := db.Exec(tc.setup); err != nil {
				t.Fatal(err)
			}
			err := Migrate(db)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Migrate=%v, want %s", err, tc.want)
			}
			var version, fk, events int
			if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow("SELECT count(*) FROM telemetry_events").Scan(&events); err != nil {
				t.Fatal(err)
			}
			wantEvents := 1
			if tc.name == "missing agents" {
				wantEvents = 0
			}
			if version != 8 || fk != 1 || events != wantEvents {
				t.Fatalf("version=%d fk=%d events=%d", version, fk, events)
			}
		})
	}
}

func TestAgentListsReportUnreadableRows(t *testing.T) {
	for _, tc := range []struct {
		name, damage string
		running      bool
	}{
		{"active list null identity", "UPDATE agents SET id=NULL", false},
		{"running list null identity", "UPDATE agents SET id=NULL", true},
		{"active list invalid usage", "UPDATE agents SET tokens_input='invalid'", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openV8Fixture(t)
			if _, err := db.Exec("PRAGMA foreign_keys=OFF;" + tc.damage); err != nil {
				t.Fatal(err)
			}
			st := &Store{db: db}
			var err error
			if tc.running {
				_, err = st.GetRunningAgents("")
			} else {
				_, err = st.GetAllActiveAgents()
			}
			if err == nil {
				t.Fatal("corrupt row was silently accepted")
			}
		})
	}
}

func TestTableRebuildMigrationsRejectClosedDatabase(t *testing.T) {
	for _, migrate := range []func(*sql.DB) error{migrateV6, migrateV9} {
		db := openV8Fixture(t)
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		if err := migrate(db); err == nil || !strings.Contains(err.Error(), "getting connection") {
			t.Fatalf("closed database: %v", err)
		}
	}
}
