package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallGitHook_FreshFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "post-merge")
	if err := installGitHook(path, postMergeHookBody); err != nil {
		t.Fatalf("installGitHook: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	content := string(data)
	if !strings.HasPrefix(content, "#!/bin/sh") {
		t.Errorf("fresh hook should start with shebang, got:\n%s", content)
	}
	if !strings.Contains(content, syncHookMarkerStart) || !strings.Contains(content, syncHookMarkerEnd) {
		t.Errorf("hook missing markers:\n%s", content)
	}
	if !strings.Contains(content, "knowing sync") {
		t.Errorf("hook missing sync invocation:\n%s", content)
	}
	// Must be executable.
	info, _ := os.Stat(path)
	if info.Mode()&0o111 == 0 {
		t.Errorf("hook not executable: mode %v", info.Mode())
	}
}

func TestInstallGitHook_AppendsToExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "post-merge")
	pre := "#!/bin/sh\necho 'user hook runs first'\n"
	if err := os.WriteFile(path, []byte(pre), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := installGitHook(path, postMergeHookBody); err != nil {
		t.Fatalf("installGitHook: %v", err)
	}
	content, _ := os.ReadFile(path)
	s := string(content)
	if !strings.Contains(s, "user hook runs first") {
		t.Errorf("pre-existing hook content was lost:\n%s", s)
	}
	if !strings.Contains(s, syncHookMarkerStart) {
		t.Errorf("knowing block not appended:\n%s", s)
	}
}

func TestInstallGitHook_ReplacesOwnBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "post-merge")
	// Install twice; the second must not duplicate the block.
	if err := installGitHook(path, postMergeHookBody); err != nil {
		t.Fatal(err)
	}
	if err := installGitHook(path, postMergeHookBody); err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(path)
	if n := strings.Count(string(content), syncHookMarkerStart); n != 1 {
		t.Errorf("expected exactly 1 knowing block after double install, got %d:\n%s", n, content)
	}
}

func TestUninstallGitHook_RemovesOwnOnlyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "post-merge")
	if err := installGitHook(path, postMergeHookBody); err != nil {
		t.Fatal(err)
	}
	removed, err := uninstallGitHook(path)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if !removed {
		t.Error("expected removed=true")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("hook that was ours alone should be deleted, still exists")
	}
}

func TestUninstallGitHook_PreservesUserContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "post-merge")
	pre := "#!/bin/sh\necho 'user hook'\n"
	if err := os.WriteFile(path, []byte(pre), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := installGitHook(path, postMergeHookBody); err != nil {
		t.Fatal(err)
	}
	removed, err := uninstallGitHook(path)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if !removed {
		t.Error("expected removed=true")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("hook with user content should survive: %v", err)
	}
	s := string(content)
	if !strings.Contains(s, "user hook") {
		t.Errorf("user content lost:\n%s", s)
	}
	if strings.Contains(s, syncHookMarkerStart) {
		t.Errorf("knowing block not stripped:\n%s", s)
	}
}

func TestUninstallGitHook_NoBlockIsNoOp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "post-merge")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	removed, err := uninstallGitHook(path)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if removed {
		t.Error("expected removed=false for a hook without our block")
	}
}

func TestUninstallGitHook_MissingFile(t *testing.T) {
	removed, err := uninstallGitHook(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if removed {
		t.Error("expected removed=false for missing file")
	}
}

// gitInit sets up a throwaway repo with two commits and returns its path plus
// the two commit hashes. Skips the test if git is unavailable.
func gitInit(t *testing.T) (root, c1, c2 string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root = t.TempDir()
	run := func(args ...string) string {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("init", "-q")
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-q", "-m", "first")
	c1 = run("rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-q", "-m", "second")
	c2 = run("rev-parse", "HEAD")
	return root, c1, c2
}

func TestChangedFilesSince(t *testing.T) {
	root, c1, c2 := gitInit(t)

	changed, determined := changedFilesSince(root, c1, c2)
	if !determined {
		t.Error("real diff should be determined")
	}
	if len(changed) != 1 || changed[0] != "b.txt" {
		t.Errorf("expected [b.txt], got %v", changed)
	}

	// Equal commits -> determined, empty (genuinely nothing changed).
	if got, det := changedFilesSince(root, c2, c2); got != nil || !det {
		t.Errorf("equal commits should be determined+empty, got files=%v determined=%v", got, det)
	}
	// Empty old commit -> NOT determined (first index; caller falls back to full).
	if got, det := changedFilesSince(root, "", c2); got != nil || det {
		t.Errorf("empty old commit should be undetermined, got files=%v determined=%v", got, det)
	}
	// Unknown commit -> NOT determined (gone after rebase/gc); caller must NOT
	// treat this as "no changes" and skip enrichment.
	if got, det := changedFilesSince(root, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", c2); got != nil || det {
		t.Errorf("unknown commit should be undetermined, got files=%v determined=%v", got, det)
	}
}

func TestGitHooksDir(t *testing.T) {
	root, _, _ := gitInit(t)
	dir, err := gitHooksDir(root)
	if err != nil {
		t.Fatalf("gitHooksDir: %v", err)
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("hooks dir should be absolute, got %q", dir)
	}
	if filepath.Base(dir) != "hooks" {
		t.Errorf("expected .../hooks, got %q", dir)
	}
}
