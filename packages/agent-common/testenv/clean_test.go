package testenv

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestWorkspaceCleanPreservesRuntimeAndScratch(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	fixture := t.TempDir()
	write := func(name string, data []byte) {
		t.Helper()
		path := filepath.Join(fixture, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	recipes, err := filepath.Glob(filepath.Join(root, "packages", "*", "justfile"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range append(recipes, filepath.Join(root, "justfile")) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		name, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}
		write(name, data)
	}
	outputs := []string{
		"packages/agent-common/bin/output", "packages/agent-observer/bin/output",
		"packages/agent-ingester/bin/output", "packages/agent-server/bin/output",
		"packages/agent-debug/bin/output", "packages/ui-web/dist/output",
	}
	preserved := []string{
		".tmp/test-other-session/active", ".tmp/test-other-session.log",
		".tmp/dev-stack/data/agents.db", ".tmp/dev-stack/logs/observer.log",
		".tmp/unrelated/keep", "packages/agent-common/source.go",
	}
	for _, path := range append(outputs, preserved...) {
		write(path, []byte(path))
	}
	// Record attempted lifecycle changes without contacting the real dev stack.
	write("tools/devhost", []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$DEVHOST_CALLS\"\n"))
	for range 2 {
		cmd := exec.Command("just", "clean")
		cmd.Dir = fixture
		cmd.Env = append(os.Environ(), "PATH="+filepath.Join(fixture, "tools")+string(os.PathListSeparator)+os.Getenv("PATH"), "DEVHOST_CALLS="+filepath.Join(fixture, "devhost.calls"))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("clean: %v\n%s", err, out)
		}
		for _, path := range outputs {
			if _, err := os.Stat(filepath.Join(fixture, path)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("build output survived: %s: %v", path, err)
			}
		}
		for _, path := range preserved {
			data, err := os.ReadFile(filepath.Join(fixture, path))
			if err != nil || string(data) != path {
				t.Errorf("clean changed unrelated file %s: %q, %v", path, data, err)
			}
		}
	}
	if calls, err := os.ReadFile(filepath.Join(fixture, "devhost.calls")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("clean changed dev stack lifecycle: %q, %v", calls, err)
	}
}
