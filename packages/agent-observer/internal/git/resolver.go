package git

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-observer/internal/types"
)

const DefaultCacheTTL = 30 * time.Second

type CacheEntry struct {
	Project    types.IProjectInfo
	Workspace  types.IWorkspaceInfo
	ResolvedAt time.Time
}

type Resolver struct {
	mu       sync.RWMutex
	cache    map[string]CacheEntry
	cacheTTL time.Duration
}

func NewResolver(ttl time.Duration) *Resolver {
	if ttl <= 0 {
		ttl = DefaultCacheTTL
	}
	return &Resolver{
		cache:    make(map[string]CacheEntry),
		cacheTTL: ttl,
	}
}

// Invalidate clears the cache for a specific CWD or all if empty.
func (r *Resolver) Invalidate(cwd string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cwd == "" {
		r.cache = make(map[string]CacheEntry)
		return
	}
	delete(r.cache, filepath.Clean(cwd))
}

// Resolve resolves git workspace and project metadata for a given working directory.
// If branchHint is non-empty, it is used for the branch name (e.g. from Claude Code transcript records).
func (r *Resolver) Resolve(ctx context.Context, cwd string, branchHint string) (types.IProjectInfo, types.IWorkspaceInfo, error) {
	cleanCWD := filepath.Clean(cwd)

	r.mu.RLock()
	entry, exists := r.cache[cleanCWD]
	r.mu.RUnlock()

	if exists && time.Since(entry.ResolvedAt) < r.cacheTTL {
		// If branchHint is provided and differs from cached branch, we invalidate and re-resolve
		if branchHint == "" || branchHint == entry.Workspace.Branch {
			return entry.Project, entry.Workspace, nil
		}
	}

	proj, ws, err := r.resolveGit(ctx, cleanCWD, branchHint)
	if err != nil {
		// Fallback for non-git directories
		base := filepath.Base(cleanCWD)
		if base == "" || base == "." || base == "/" {
			base = "default"
		}
		fallbackProj := types.IProjectInfo{
			ID:            "local/" + base,
			Name:          base,
			CanonicalRoot: cleanCWD,
		}
		fallbackWS := types.IWorkspaceInfo{
			IsWorktree: false,
			Branch:     branchHint,
			IsDetached: false,
			RelPath:    nil,
			CWD:        cleanCWD,
		}

		r.mu.Lock()
		r.cache[cleanCWD] = CacheEntry{
			Project:    fallbackProj,
			Workspace:  fallbackWS,
			ResolvedAt: time.Now(),
		}
		r.mu.Unlock()

		return fallbackProj, fallbackWS, nil
	}

	r.mu.Lock()
	r.cache[cleanCWD] = CacheEntry{
		Project:    proj,
		Workspace:  ws,
		ResolvedAt: time.Now(),
	}
	r.mu.Unlock()

	return proj, ws, nil
}

func (r *Resolver) resolveGit(ctx context.Context, cwd string, branchHint string) (types.IProjectInfo, types.IWorkspaceInfo, error) {
	// 1. git rev-parse --path-format=absolute --git-common-dir
	commonDirCmd := exec.CommandContext(ctx, "git", "rev-parse", "--path-format=absolute", "--git-common-dir")
	commonDirCmd.Dir = cwd
	commonDirOut, err := commonDirCmd.Output()
	if err != nil {
		return types.IProjectInfo{}, types.IWorkspaceInfo{}, fmt.Errorf("git common dir: %w", err)
	}
	gitCommonDir := filepath.Clean(strings.TrimSpace(string(commonDirOut)))

	// Canonical root is the directory containing the shared .git directory
	canonicalRoot := gitCommonDir
	if strings.HasSuffix(canonicalRoot, string(filepath.Separator)+".git") {
		canonicalRoot = filepath.Dir(canonicalRoot)
	} else if filepath.Base(canonicalRoot) == ".git" {
		canonicalRoot = filepath.Dir(canonicalRoot)
	}
	if eval, err := filepath.EvalSymlinks(canonicalRoot); err == nil {
		canonicalRoot = eval
	}

	// 2. git rev-parse --show-toplevel (worktree top-level)
	topLevelCmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	topLevelCmd.Dir = cwd
	topLevelOut, err := topLevelCmd.Output()
	if err != nil {
		return types.IProjectInfo{}, types.IWorkspaceInfo{}, fmt.Errorf("git show toplevel: %w", err)
	}
	topLevel := filepath.Clean(strings.TrimSpace(string(topLevelOut)))
	if eval, err := filepath.EvalSymlinks(topLevel); err == nil {
		topLevel = eval
	}

	// 3. Linked worktree detection
	isWorktree := false
	var relPath *string
	if topLevel != canonicalRoot {
		isWorktree = true
		if rel, err := filepath.Rel(canonicalRoot, topLevel); err == nil {
			relPath = &rel
		} else {
			relPath = &topLevel
		}
	}

	// 4. Branch and detached HEAD detection
	branch := branchHint
	isDetached := false

	if branch == "" {
		symCmd := exec.CommandContext(ctx, "git", "symbolic-ref", "--short", "HEAD")
		symCmd.Dir = cwd
		symOut, err := symCmd.Output()
		if err == nil {
			branch = strings.TrimSpace(string(symOut))
		} else {
			// Detached HEAD: get short SHA
			isDetached = true
			revCmd := exec.CommandContext(ctx, "git", "rev-parse", "--short", "HEAD")
			revCmd.Dir = cwd
			revOut, rErr := revCmd.Output()
			if rErr == nil {
				branch = strings.TrimSpace(string(revOut))
			} else {
				branch = "HEAD"
			}
		}
	}

	// 5. Origin URL and Project ID
	var rawOrigin string
	originCmd := exec.CommandContext(ctx, "git", "remote", "get-url", "origin")
	originCmd.Dir = cwd
	originOut, err := originCmd.Output()
	if err == nil {
		rawOrigin = strings.TrimSpace(string(originOut))
	}

	baseName := filepath.Base(canonicalRoot)
	if baseName == "" || baseName == "." || baseName == "/" {
		baseName = "default"
	}

	normalizedOrigin := NormalizeOriginURL(rawOrigin)
	var projID string
	var projName string

	if normalizedOrigin != "" {
		projID = normalizedOrigin
		projName = ExtractRepoName(normalizedOrigin)
		if projName == "" {
			projName = baseName
		}
	} else {
		projID = "local/" + baseName
		projName = baseName
	}

	proj := types.IProjectInfo{
		ID:            projID,
		Name:          projName,
		CanonicalRoot: canonicalRoot,
	}

	ws := types.IWorkspaceInfo{
		IsWorktree: isWorktree,
		Branch:     branch,
		IsDetached: isDetached,
		RelPath:    relPath,
		CWD:        cwd,
	}

	return proj, ws, nil
}

var scpLikeRegex = regexp.MustCompile(`^(?:[\w\.\-]+@)?([\w\.\-]+):(.+)$`)

// NormalizeOriginURL normalizes a remote git URL into "host/owner/repo".
// If invalid or empty, returns empty string.
func NormalizeOriginURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	// Check SCP-like syntax: git@github.com:owner/repo.git
	if matches := scpLikeRegex.FindStringSubmatch(raw); len(matches) == 3 && !strings.Contains(raw, "://") {
		host := matches[1]
		path := strings.TrimPrefix(matches[2], "/")
		path = strings.TrimSuffix(path, ".git")
		path = strings.TrimRight(path, "/")
		if host != "" && path != "" {
			return host + "/" + path
		}
	}

	// Standard URL parsing
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}

	host := parsed.Hostname()
	if host == "" {
		host = parsed.Host
	}
	if host == "" {
		return ""
	}

	path := strings.TrimPrefix(parsed.Path, "/")
	path = strings.TrimSuffix(path, ".git")
	path = strings.TrimRight(path, "/")

	if path == "" {
		return ""
	}

	return host + "/" + path
}

// ExtractRepoName extracts the repository name slug from normalized origin or path.
func ExtractRepoName(normalized string) string {
	if normalized == "" {
		return ""
	}
	parts := strings.Split(normalized, "/")
	return parts[len(parts)-1]
}
