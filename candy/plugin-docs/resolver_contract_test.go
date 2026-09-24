package docs

import (
	"os"
	"os/exec"
	"testing"

	"github.com/opencharly/spec/refs"
)

// TestPinnedSpecResolvesMixedTagForms is the CONTRACT TEST for this repo's spec
// pin. The catalog resolves extra_repos/release_repos through refs.GitLatestTag
// (defaultRepoResolver, catalog.go), so plugin-docs' RESOLUTION BEHAVIOUR comes
// from its pinned github.com/opencharly/spec. A repo mid-migration carries BOTH
// tag encodings — the plain CalVer v<YYYY>.<DDD>.<HHMM> and the Go-module form
// v0.<YYYYDDD>.<HHMM> the proxy-consumed root-module repos (sdk/spec/plugin-gh)
// require — and spec must rank the newer Go-form tag above a STALE plain tag, or
// the generated plugin-gh page freezes at the old plain tag (the exact defect
// that pinned plugin-gh to v2026.252.1501).
//
// This test is therefore pin-sensitive by design: with a spec pin that predates
// the fix it FAILS; the bump to the fixed tag makes it pass. It runs against a
// LOCAL git repo (no network), so it is a normal unit test, not a live test.
func TestPinnedSpecResolvesMixedTagForms(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("SKIP: git not on PATH")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "--initial-branch=main")
	run("commit", "-q", "--allow-empty", "-m", "c")
	// The real plugin-gh shape: a stale plain tag, then newer Go-form tags.
	for _, tag := range []string{"v2026.252.1500", "v2026.252.1501", "v0.2026266.2326", "v0.2026267.723"} {
		run("tag", tag)
	}

	got, err := refs.GitLatestTag(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "v0.2026267.723" {
		t.Fatalf("pinned spec resolved %q, want v0.2026267.723 — the newer Go-module tag; "+
			"the stale plain tag v2026.252.1501 must not win (bump the spec pin to the tag carrying the fix)", got)
	}
}
