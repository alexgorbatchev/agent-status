package testenv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsolateHarnessHomeClearsHarnessLocations(t *testing.T) {
	t.Setenv("HOME", "/real/home")
	t.Setenv("XDG_DATA_HOME", "/real/data")
	for _, name := range harnessEnv {
		t.Setenv(name, "/real/"+name)
	}

	home := IsolateHarnessHome(t)

	if got := os.Getenv("HOME"); got != home {
		t.Errorf("HOME = %q, want %q", got, home)
	}
	if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
		t.Errorf("isolated HOME is not an empty directory: entries=%v err=%v", entries, err)
	}
	for _, name := range append([]string{"XDG_DATA_HOME"}, harnessEnv...) {
		if got := os.Getenv(name); got != "" {
			t.Errorf("%s = %q, want it cleared", name, got)
		}
	}
}

func TestIsolateHarnessEnvClearsHarnessLocations(t *testing.T) {
	t.Setenv("HOME", os.Getenv("HOME")) // restored after the test
	for _, name := range harnessEnv {
		t.Setenv(name, "/real/"+name)
	}
	home := filepath.Join(t.TempDir(), "home")

	if err := IsolateHarnessEnv(home); err != nil {
		t.Fatal(err)
	}

	if got := os.Getenv("HOME"); got != home {
		t.Errorf("HOME = %q, want %q", got, home)
	}
	if info, err := os.Stat(home); err != nil || !info.IsDir() {
		t.Errorf("isolated HOME was not created: %v", err)
	}
	for _, name := range harnessEnv {
		if value, set := os.LookupEnv(name); set {
			t.Errorf("%s = %q, want it unset", name, value)
		}
	}
}

func TestIsolateHarnessEnvReportsUncreatableHome(t *testing.T) {
	t.Setenv("HOME", os.Getenv("HOME"))
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := IsolateHarnessEnv(filepath.Join(file, "home")); err == nil {
		t.Fatal("expected an error when HOME cannot be created")
	}
	if got := os.Getenv("HOME"); got == filepath.Join(file, "home") {
		t.Error("HOME was changed although the directory could not be created")
	}
}

func TestRequireHarnessIsolated(t *testing.T) {
	t.Setenv("HOME", os.Getenv("HOME"))
	for _, name := range harnessEnv {
		t.Setenv(name, os.Getenv(name))
	}
	origHome := isolatedHome
	defer func() { isolatedHome = origHome }()

	failsRequire := func() bool {
		rec := &fatalRecorder{}
		RequireHarnessIsolated(rec)
		return rec.failed
	}

	isolatedHome = ""
	if !failsRequire() {
		t.Error("RequireHarnessIsolated passed although IsolateHarnessEnv never ran")
	}

	home := filepath.Join(t.TempDir(), "home")
	if err := IsolateHarnessEnv(home); err != nil {
		t.Fatal(err)
	}
	if failsRequire() {
		t.Error("RequireHarnessIsolated failed right after IsolateHarnessEnv")
	}

	t.Setenv("HOME", filepath.Join(home, "nested"))
	if failsRequire() {
		t.Error("RequireHarnessIsolated failed for a HOME inside the isolated home")
	}

	t.Setenv("HOME", t.TempDir())
	if !failsRequire() {
		t.Error("RequireHarnessIsolated passed although HOME left the isolated home")
	}

	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", "/real/codex")
	if !failsRequire() {
		t.Error("RequireHarnessIsolated passed although CODEX_HOME relocates harness storage")
	}
}
