package app

import (
	"os"
	"path/filepath"
	"testing"

	"charm.land/bubbles/v2/table"
	"github.com/chmouel/lazyworktree/internal/config"
	"github.com/chmouel/lazyworktree/internal/models"
)

func TestDetermineCurrentWorktreePrefersSelection(t *testing.T) {
	cfg := &config.AppConfig{
		WorktreeDir: t.TempDir(),
	}
	m := NewModel(cfg, "")

	main := &models.WorktreeInfo{Path: "/tmp/main", Branch: "main", IsMain: true}
	feature := &models.WorktreeInfo{Path: "/tmp/feature", Branch: "feature"}
	m.state.data.worktrees = []*models.WorktreeInfo{main, feature}
	m.state.data.filteredWts = m.state.data.worktrees

	rows := []table.Row{
		{"main"},
		{"feature"},
	}
	m.state.ui.worktreeTable.SetRows(rows)
	m.state.ui.worktreeTable.SetCursor(1)

	got := m.determineCurrentWorktree()
	if got != feature {
		t.Fatalf("expected selected worktree, got %v", got)
	}
}

func TestDetermineCurrentWorktreeAvoidsSiblingPrefixMatch(t *testing.T) {
	cfg := &config.AppConfig{
		WorktreeDir: t.TempDir(),
	}
	m := NewModel(cfg, "")

	parent := normalizePathForTest(t, t.TempDir())
	feature := filepath.Join(parent, "feature")
	featureTwo := filepath.Join(parent, "feature-2")
	requireDir(t, feature)
	requireDir(t, featureTwo)

	main := &models.WorktreeInfo{Path: filepath.Join(parent, "main"), Branch: "main", IsMain: true}
	featureWt := &models.WorktreeInfo{Path: feature, Branch: "feature"}
	featureTwoWt := &models.WorktreeInfo{Path: featureTwo, Branch: "feature-2"}
	m.state.data.worktrees = []*models.WorktreeInfo{main, featureWt, featureTwoWt}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(featureTwo); err != nil {
		t.Fatalf("failed to change working directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	got := m.determineCurrentWorktree()
	if got != featureTwoWt {
		t.Fatalf("expected feature-2 worktree, got %v", got)
	}
}

func requireDir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil { //nolint:gosec
		t.Fatalf("failed to create directory %q: %v", path, err)
	}
}

func normalizePathForTest(t *testing.T, path string) string {
	t.Helper()

	normalized, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path
	}
	return normalized
}
