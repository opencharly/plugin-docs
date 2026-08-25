package docs

import (
	"bytes"
	"github.com/opencharly/spec/refs"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The leaf generators and gates each have their own unit tests, and every one of them passed while
// generate() was free to not call them at all: deleting the generateLanding, generateGrievances or
// verifySidebarLinks call site from generate() left `go test ./...` fully green. Leaf coverage
// proves a function works; it says nothing about whether the pipeline runs it. These two tests
// cover the wiring, by driving the real entry point and asserting on what the run leaves behind.

// generateSite runs generate() over the real repository into a throwaway site whose sidebar the
// test controls.
//
// The layout mirrors the real one because both gates at the end of generate() depend on it:
// verifySidebarLinks walks UP from the content root for the Astro config, so the config must sit
// above --out rather than inside it, and verifySiteLinks resolves every internal link against the
// pages present in --out — including the hand-authored ones the generator does not own but does
// link to (the README projection alone points at eight of them). Seeding those is therefore part
// of building a site, not a convenience.
// generateSite runs generate() against the charly project checkout (superprojectRoot — the
// in-repo repo root or the standalone repo's charly submodule, both holding the FULL corpus)
// with a synthetic plugins dir (the generator's skills pass needs a marketplace.json; the
// corpus itself is the checkout's own, so the seeded pages' internal links resolve).
func generateSite(t *testing.T, sidebarEntries ...string) (out string, err error) {
	t.Helper()

	root := superprojectRoot(t)
	base := t.TempDir()
	out = filepath.Join(base, "src", "content", "docs")
	if mkErr := os.MkdirAll(out, 0o755); mkErr != nil {
		t.Fatalf("create content root: %v", mkErr)
	}
	if wErr := os.WriteFile(filepath.Join(base, "astro.config.mjs"),
		[]byte(astroConfig(sidebarEntries...)), 0o644); wErr != nil {
		t.Fatalf("write astro config: %v", wErr)
	}
	seedHandAuthoredPages(t, root, out)

	// The skills pass needs the marketplace corpus (plugins/.claude-plugin/marketplace.json) to
	// generate the recipe pages the seeded hand-authored pages link to. marketplaceCorpusDir
	// resolves it from the in-repo checkout OR fetches it from the opencharly/marketplace repo via
	// refs.DownloadRepo (the same standalone fetch the generator uses).
	pluginsDir := marketplaceCorpusDir(t)

	return out, generate(root, out, pluginsDir)
}

// marketplaceCorpusDir returns a directory holding the marketplace corpus
// (plugins/.claude-plugin/marketplace.json) the skills pass needs to generate the recipe pages
// the seeded hand-authored pages link to. In the IN-REPO layout the charly checkout carries it
// at <root>/plugins; in the STANDALONE layout it is fetched from the opencharly/marketplace repo
// via refs.DownloadRepo — the SAME standalone fetch the generator's CollectEntitiesRemote uses,
// so the corpus-dependent tests exercise the changed fetch path rather than skipping.
func marketplaceCorpusDir(t *testing.T) string {
	t.Helper()
	// The Phase-3 pipeline regenerates the marketplace corpus via `charly marketplace generate`
	// (the remote-aware discovery); the PUBLISHED marketplace repo at main is stale until that
	// regeneration lands. Prefer a freshly regenerated tree when one is provided (the RDD bed
	// regenerates into a temp dir and passes it via CHARLY_DOCS_MARKETPLACE), else fall back to
	// the in-repo checkout, else fetch the published repo.
	if dir := os.Getenv("CHARLY_DOCS_MARKETPLACE"); dir != "" {
		if fileExists(filepath.Join(dir, ".claude-plugin", "marketplace.json")) {
			return dir
		}
	}
	root := superprojectRoot(t)
	inRepo := filepath.Join(root, "plugins")
	if fileExists(filepath.Join(inRepo, ".claude-plugin", "marketplace.json")) {
		return inRepo
	}
	// Standalone layout: fetch the marketplace repo at its newest tag.
	tag, err := refs.GitDefaultBranch(refs.RepoGitURL("github.com/opencharly/marketplace"))
	if err != nil {
		t.Fatalf("resolve marketplace default branch: %v", err)
	}
	dir, err := refs.DownloadRepo("github.com/opencharly/marketplace", tag)
	if err != nil {
		t.Fatalf("fetch marketplace corpus: %v", err)
	}
	return dir
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}

// superprojectRoot resolves the charly project checkout whose corpus the generator reads.
// In the IN-REPO layout (charly/candy/plugin-docs) that is two levels up (the charly repo
// root, holding charly.yml + candy/). In the STANDALONE layout (the candy de-submodule
// cutover, Phase 2c — this repo IS opencharly/plugin-docs) the corpus lives in the pinned
// charly SUBMODULE at <repo>/charly, which this repo's .gitmodules declares and CI checks
// out recursively. The module dir itself (candy/plugin-docs) carries a charly.yml too — but
// that is a CANDY manifest, not a project root — so the discriminator is the presence of a
// candy/ SIBLING dir (the project layout), never the candy's own manifest.
func superprojectRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// Walk up looking for a dir holding charly.yml AND a candy/ sibling (a project root,
	// not a candy dir). In the in-repo layout cwd = charly/candy/plugin-docs, so this
	// skips the candy dir and lands on the charly repo root.
	for dir := cwd; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, unifiedFileName)); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "candy")); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	// Standalone layout: the charly submodule at <repo>/charly.
	repoRoot := filepath.Dir(filepath.Dir(cwd)) // candy/plugin-docs -> <repo>
	sub := filepath.Join(repoRoot, "charly")
	if _, err := os.Stat(filepath.Join(sub, unifiedFileName)); err != nil {
		t.Fatalf("no charly project root above %s and no charly submodule checkout at %s: %v", cwd, sub, err)
	}
	return sub
}

// seedHandAuthoredPages copies the fixture's hand-authored pages into a fresh --out.
//
// The fixture lives in testdata/site (a snapshot of the docs repo's start/ concepts/ guides/
// trees, taken at the docs de-submodule cutover): charly no longer carries the docs submodule,
// and the whole-site link gates still need the hand-authored pages the generator links to but
// does not own. The snapshot is versioned with the generator; when the README projection or the
// gates' expectations change, refresh it from the opencharly/docs repo's src/content/docs.
//
// Pages are selected by the same rule pruneGeneratedPages uses — a page is generated if and only
// if it carries the generated header — rather than by a hardcoded list of directories, so a new
// hand-authored tree needs no change here and, more importantly, so nothing generated can leak in.
// That exclusion is what makes the assertions in these tests airtight: a seeded page can never
// carry the generated header, so a page that carries one was written by the run under test.
func seedHandAuthoredPages(t *testing.T, root, out string) {
	t.Helper()

	src := filepath.Join(root, "candy", "plugin-docs", "testdata", "site")
	// Standalone-layout fallback: the fixture tree lives in THIS module's own testdata/
	// (the in-repo path above only exists in the pre-cutover charly checkout).
	if _, err := os.Stat(src); err != nil {
		src = filepath.Join("testdata", "site")
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("no hand-authored fixture tree at %s — refresh it from the opencharly/docs "+
			"repo's src/content/docs (start/ concepts/ guides/): %v", src, err)
	}

	header := []byte(generatedHeader)
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".md", ".mdx":
		default:
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(raw, header) {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(out, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, raw, 0o644)
	})
	if err != nil {
		t.Fatalf("seed hand-authored pages from %s: %v", src, err)
	}
}

// TestGenerateWiresLeafGenerators proves generate() actually calls the leaf generators, by
// asserting on the pages a real run leaves behind. Each page is checked for the generated header
// as well as for existence: the seed deliberately excludes every header-carrying page, so the
// header is proof the run wrote the file rather than the fixture.
//
// ENVIRONMENT PRECONDITION: the seeded hand-authored pages (testdata/site, a snapshot of the
// docs repo) link to recipe pages generated from the MARKETPLACE skills corpus
// (plugins/.claude-plugin/marketplace.json). In the STANDALONE plugin-docs layout the charly
// submodule carries the candy corpus but NOT the marketplace corpus (that lives in the
// opencharly/marketplace repo, not checked out here) — so the run cannot resolve every link and
// the test SKIPS with the live venue named. The wiring assertions it makes are exercised there:
// the check-docs R10 bed and the docs repo's deploy workflow run `charly docs generate` against
// the full pinned charly + marketplace checkout. In the IN-REPO layout (a charly checkout with
// the marketplace corpus) the test runs in full.
func TestGenerateWiresLeafGenerators(t *testing.T) {
	_ = marketplaceCorpusDir(t) // fetch the corpus (or fail) before the assertions
	out, genErr := generateSite(t,
		"      { label: 'The Vision', link: '/vision/' },",
		"      { label: 'Recipes', link: '/recipes/' },",
	)

	// The page assertions run BEFORE the error is reported, and deliberately so. Removing a leaf
	// call site can also trip the site-link gate — the README projection links to /grievances/, so
	// dropping generateGrievances surfaces as a dead link — and that error alone never says which
	// call site went missing. Naming the unwired caller is the whole point of this test.
	for _, tc := range []struct {
		page   string
		caller string
	}{
		{page: "index.md", caller: "generateLanding"},
		{page: "grievances.md", caller: "generateGrievances"},
		{page: "liberation.md", caller: "generateLiberation"},
	} {
		raw, err := os.ReadFile(filepath.Join(out, tc.page))
		if err != nil {
			t.Errorf("generate() did not emit %s — its %s call site is not wired: %v",
				tc.page, tc.caller, err)
			continue
		}
		if !bytes.Contains(raw, []byte(generatedHeader)) {
			t.Errorf("%s carries no generated header, so it is the seeded fixture rather than "+
				"output of %s", tc.page, tc.caller)
		}
	}

	if genErr != nil {
		t.Fatalf("generate: %v", genErr)
	}
}

// TestGeneratePreservesHeaderlessFilesUnderGeneratedTrees is the test that failed the validator's
// resetTree-vs-prune finding. resetTree removed whole trees BY PATH before regenerating — an
// unread os.RemoveAll that would have deleted a hand-authored page living under a generated
// directory. prune-first makes it redundant: every emitted page carries the generated header, so
// pruning by header already clears the orphans, and a file without the header is preserved
// precisely because the boundary is content, not location. This test plants a headerless file
// under reference/candy/ and asserts generate() leaves it alone.
func TestGeneratePreservesHeaderlessFilesUnderGeneratedTrees(t *testing.T) {
	_ = marketplaceCorpusDir(t) // fetch the corpus (or fail) before the assertions
	root := superprojectRoot(t)
	base := t.TempDir()
	out := filepath.Join(base, "src", "content", "docs")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatalf("create content root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "astro.config.mjs"),
		[]byte(astroConfig("      { label: 'The Vision', link: '/vision/' },")), 0o644); err != nil {
		t.Fatalf("write astro config: %v", err)
	}
	seedHandAuthoredPages(t, root, out)
	_ = marketplaceCorpusDir(t) // the corpus (env override, in-repo, or fetched) — generateSite uses it

	kept := filepath.Join(out, "reference", "candy", "keep-me.md")
	if err := os.MkdirAll(filepath.Dir(kept), 0o755); err != nil {
		t.Fatalf("mkdir reference/candy: %v", err)
	}
	if err := os.WriteFile(kept, []byte("Hand-authored under a generated tree.\n"), 0o644); err != nil {
		t.Fatalf("write keep-me.md: %v", err)
	}

	if err := generate(root, out, marketplaceCorpusDir(t)); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Fatalf("generate() removed a headerless file under reference/candy by path: %v", err)
	}
}

// TestGenerateWiresSidebarGate proves generate() runs the sidebar gate. The gate is the only one
// that reads the Astro config, so a sidebar entry pointing at a page the run never emits fails
// through verifySidebarLinks and nothing else — which is what makes this a wiring test for that
// call site specifically.
func TestGenerateWiresSidebarGate(t *testing.T) {
	_ = marketplaceCorpusDir(t) // fetch the corpus (or fail) before the assertions
	_, err := generateSite(t,
		"      { label: 'The Vision', link: '/vision/' },",
		"      { label: 'Ghost', link: '/no-such-page/' },",
	)
	if err == nil {
		t.Fatal("generate() succeeded with a dead sidebar target — " +
			"its verifySidebarLinks call site is not wired")
	}
	if !strings.Contains(err.Error(), "/no-such-page/") {
		t.Errorf("error should name the dead sidebar target, got: %v", err)
	}
}
