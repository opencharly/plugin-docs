package docs

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opencharly/spec/refs"
)

// TestCollectEntitiesRemote_ResolvesMovedCandy is the R10 gate for the Phase-3 discovery
// change: collectEntities (sources.go) must resolve a MOVED candy's @github.com/opencharly/...
// ref through the standalone fetch (refs.DownloadRepo — the SAME mechanism the runtime uses)
// and return the entity with SourceRoot pointing at the FETCHED repo tree, so collectPlugins
// reads schema/ from there. This test FAILS without the change: the pre-Phase-3 code used
// candywalk.CollectEntities (pure local FS walk) which cannot resolve any remote ref.
//
// It uses a REAL standalone candy repo (layer-ripgrep — the Phase-0 pilot, small) fetched at
// its newest tag, so it exercises the actual fetch path, not a stub.
func TestCollectEntitiesRemote_ResolvesMovedCandy(t *testing.T) {
	// Fetch the real moved candy repo at its newest tag (the standalone fetch the generator uses).
	tag, err := refs.GitDefaultBranch(refs.RepoGitURL("github.com/opencharly/layer-ripgrep"))
	if err != nil {
		t.Fatalf("resolve layer-ripgrep default branch: %v", err)
	}
	_ = tag // branch is not a v-calver; the walker only resolves v-tagged refs — see below
	// The walker's remoteRefRe only matches v<calver> refs, so resolve the NEWEST TAG.
	tag = newestTag(t, "github.com/opencharly/layer-ripgrep")
	_, err = refs.DownloadRepo("github.com/opencharly/layer-ripgrep", tag)
	if err != nil {
		t.Fatalf("fetch layer-ripgrep: %v", err)
	}

	// A synthetic local project whose candy list references the moved candy.
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "candy", "local-keeper"), 0o755); err != nil {
		t.Fatal(err)
	}
	ref := "@github.com/opencharly/layer-ripgrep:" + tag
	local := "local-keeper:\n    candy:\n        version: 2026.200.1000\n        description: Local fixture.\n        candy:\n            - '" + ref + "'\n"
	if err := os.WriteFile(filepath.Join(root, "candy", "local-keeper", "charly.yml"), []byte(local), 0o644); err != nil {
		t.Fatal(err)
	}
	// The charly/charly.yml mirror the compiled-plugins read needs.
	if err := os.MkdirAll(filepath.Join(root, "charly"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "charly", "charly.yml"), []byte("compiled_plugins: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ents, err := collectEntities([]repoRoot{{Namespace: "", Dir: root}})
	if err != nil {
		t.Fatalf("collectEntities: %v", err)
	}

	// The moved candy must be discovered, with SourceRoot = the fetched repo dir.
	var ripgrep *entity
	for i := range ents {
		if ents[i].Name == "ripgrep" {
			ripgrep = &ents[i]
		}
	}
	if ripgrep == nil {
		t.Fatalf("moved candy ripgrep not discovered via the remote fetch; got entities: %+v", ents)
	}
	if ripgrep.SourceRoot == "" {
		t.Fatal("remote entity SourceRoot is empty — collectPlugins cannot read schema/ from the fetched tree")
	}
	// The fetched repo's actual schema dir must be readable through SourceRoot.
	// layer-ripgrep is a config candy (no plugin:), so schema/ may be absent; the SourceRoot
	// contract is what collectPlugins relies on. Assert the fetched candy manifest resolves.
	if _, err := os.Stat(filepath.Join(ripgrep.SourceRoot, ripgrep.Dir, "charly.yml")); err != nil {
		t.Fatalf("remote entity manifest not readable at SourceRoot %s: %v", ripgrep.SourceRoot, err)
	}
	t.Logf("moved candy ripgrep resolved: SourceRoot=%s Dir=%s", ripgrep.SourceRoot, ripgrep.Dir)
}

// newestTag resolves a repo's newest v-calver tag via git ls-remote, so the walker's
// v-tagged remoteRefRe matches (the refs kit shells out to git for clone/fetch, so an
// ls-remote here is the same toolchain).
func newestTag(t *testing.T, repoPath string) string {
	t.Helper()
	out, err := exec.Command("git", "ls-remote", "--tags", refs.RepoGitURL(repoPath)).Output()
	if err != nil {
		t.Fatalf("git ls-remote %s: %v", repoPath, err)
	}
	var tags []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		refName := fields[1]
		name := strings.TrimPrefix(refName, "refs/tags/")
		if !strings.HasPrefix(name, "v") || strings.HasSuffix(name, "^{}") {
			continue
		}
		tags = append(tags, name)
	}
	if len(tags) == 0 {
		t.Fatalf("no v-tags for %s", repoPath)
	}
	return tags[len(tags)-1]
}
