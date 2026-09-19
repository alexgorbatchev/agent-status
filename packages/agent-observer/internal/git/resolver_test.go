package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestNormalizeOriginURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "empty",
			raw:  "",
			want: "",
		},
		{
			name: "https with .git",
			raw:  "https://github.com/alexgorbatchev/agent-status.git",
			want: "github.com/alexgorbatchev/agent-status",
		},
		{
			name: "https without .git",
			raw:  "https://gitlab.com/group/subgroup/project",
			want: "gitlab.com/group/subgroup/project",
		},
		{
			name: "ssh with scp syntax",
			raw:  "git@github.com:alexgorbatchev/agent-status.git",
			want: "github.com/alexgorbatchev/agent-status",
		},
		{
			name: "ssh with custom host in scp syntax",
			raw:  "git@ssh.dev.azure.com:v3/Company/Project/Repo",
			want: "ssh.dev.azure.com/v3/Company/Project/Repo",
		},
		{
			name: "ssh url scheme",
			raw:  "ssh://git@github.com/alexgorbatchev/agent-status.git",
			want: "github.com/alexgorbatchev/agent-status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeOriginURL(tt.raw)
			if got != tt.want {
				t.Fatalf("NormalizeOriginURL(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestExtractRepoName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"single", "repo", "repo"},
		{"github path", "github.com/owner/repo", "repo"},
		{"nested path", "gitlab.com/group/subgroup/repo", "repo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractRepoName(tt.input)
			if got != tt.want {
				t.Fatalf("ExtractRepoName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolverNonGit(t *testing.T) {
	tmpDir := t.TempDir()
	r := NewResolver(10 * time.Second)

	proj, ws, err := r.Resolve(context.Background(), tmpDir, "")
	if err != nil {
		t.Fatalf("expected non-git dir to fallback gracefully, got error: %v", err)
	}

	base := filepath.Base(tmpDir)
	if proj.ID != "local/"+base {
		t.Errorf("proj.ID = %q, want %q", proj.ID, "local/"+base)
	}
	if proj.Name != base {
		t.Errorf("proj.Name = %q, want %q", proj.Name, base)
	}
	if ws.IsWorktree {
		t.Errorf("ws.IsWorktree = true, want false")
	}
}

func TestResolverGitAndWorktree(t *testing.T) {
	// Create main git repository in a temp dir
	mainRepo := t.TempDir()

	runGit := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\nOutput: %s", args, err, string(out))
		}
	}

	runGit(mainRepo, "init", "-b", "main")
	runGit(mainRepo, "config", "user.email", "test@example.com")
	runGit(mainRepo, "config", "user.name", "Test User")
	runGit(mainRepo, "remote", "add", "origin", "git@github.com:org/test-repo.git")

	// Create a commit so we can add worktrees
	dummyFile := filepath.Join(mainRepo, "README.md")
	if err := os.WriteFile(dummyFile, []byte("# Test"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(mainRepo, "add", "README.md")
	runGit(mainRepo, "commit", "-m", "initial commit")

	// Create linked worktree
	worktreeDir := filepath.Join(t.TempDir(), "feat-wt")
	runGit(mainRepo, "worktree", "add", "-b", "feat-wt", worktreeDir)

	r := NewResolver(10 * time.Second)
	ctx := context.Background()

	// 1. Resolve main repository
	proj1, ws1, err := r.Resolve(ctx, mainRepo, "")
	if err != nil {
		t.Fatalf("Resolve mainRepo error: %v", err)
	}
	if proj1.ID != "github.com/org/test-repo" {
		t.Errorf("proj1.ID = %q, want %q", proj1.ID, "github.com/org/test-repo")
	}
	if proj1.Name != "test-repo" {
		t.Errorf("proj1.Name = %q, want %q", proj1.Name, "test-repo")
	}
	if ws1.IsWorktree {
		t.Errorf("ws1.IsWorktree = true, want false")
	}
	if ws1.Branch != "main" {
		t.Errorf("ws1.Branch = %q, want 'main'", ws1.Branch)
	}

	// 2. Resolve worktree
	proj2, ws2, err := r.Resolve(ctx, worktreeDir, "")
	if err != nil {
		t.Fatalf("Resolve worktreeDir error: %v", err)
	}
	if proj2.ID != "github.com/org/test-repo" {
		t.Errorf("proj2.ID = %q, want %q", proj2.ID, "github.com/org/test-repo")
	}
	if !ws2.IsWorktree {
		t.Errorf("ws2.IsWorktree = false, want true")
	}
	if ws2.Branch != "feat-wt" {
		t.Errorf("ws2.Branch = %q, want 'feat-wt'", ws2.Branch)
	}
	if ws2.RelPath == nil {
		t.Errorf("ws2.RelPath is nil, want relative path")
	}

	// 3. Test caching and branch hint invalidation
	projCached, wsCached, err := r.Resolve(ctx, mainRepo, "")
	if err != nil {
		t.Fatalf("Resolve cached error: %v", err)
	}
	if projCached.ID != proj1.ID {
		t.Errorf("cached proj ID mismatch")
	}
	if wsCached.Branch != ws1.Branch {
		t.Errorf("cached branch mismatch: got %q, want %q", wsCached.Branch, ws1.Branch)
	}

	// Branch hint mismatch should update branch
	_, wsHint, err := r.Resolve(ctx, mainRepo, "branch-hint-override")
	if err != nil {
		t.Fatalf("Resolve with branch hint error: %v", err)
	}
	if wsHint.Branch != "branch-hint-override" {
		t.Errorf("wsHint.Branch = %q, want 'branch-hint-override'", wsHint.Branch)
	}
}
