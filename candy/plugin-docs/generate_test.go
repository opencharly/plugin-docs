package docs

import (
	"bytes"
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

// generateSite runs generate() over the hermetic fixture project into a throwaway site whose
// sidebar the test controls.
//
// The layout mirrors the real one because both gates at the end of generate() depend on it:
// verifySidebarLinks walks UP from the content root for the Astro config, so the config must sit
// above --out rather than inside it, and verifySiteLinks resolves every internal link against the
// pages present in --out — including the hand-authored ones the generator does not own but does
// link to (the landing hero's Get-started actions point at /start/install/ and /start/quickstart/).
// Seeding those is therefore part of building a site, not a convenience.
//
// The ENTIRE environment is hermetic testdata, so the tests run in a bare clone with no external
// charly/marketplace checkout and no network (the R2 gate): the generate root is testdata/project,
// a minimal self-contained charly project (a docs: node with the compiled corpus DISABLED and no
// release/extra repos, so the catalog assembly never fetches, plus the README/VISION/GRIEVANCES/
// LIBERATION narratives the leaf passes project); the marketplace corpus is testdata/marketplace,
// a one-plugin fixture whose refs/skills are internally consistent (every fixture card body
// carries no unresolved harness reference — corpus completeness of the published marketplace repo
// is NOT this test's subject, the WIRING is, and the published corpus can in fact carry drift: its
// recipes/automation/crabbox-deploy.md card references the skill /charly-tools:crabbox, which has
// no defined card); and the hand-authored pages live in testdata/site, seeded with the two pages
// the generated landing links to.
func generateSite(t *testing.T, sidebarEntries ...string) (out string, err error) {
	t.Helper()

	root := fixtureProjectRoot(t)
	base := t.TempDir()
	out = filepath.Join(base, "src", "content", "docs")
	if mkErr := os.MkdirAll(out, 0o755); mkErr != nil {
		t.Fatalf("create content root: %v", mkErr)
	}
	if wErr := os.WriteFile(filepath.Join(base, "astro.config.mjs"),
		[]byte(astroConfig(sidebarEntries...)), 0o644); wErr != nil {
		t.Fatalf("write astro config: %v", wErr)
	}
	seedHandAuthoredPages(t, out)

	// The skills pass needs the marketplace corpus (plugins/.claude-plugin/marketplace.json) to
	// generate the recipe pages the seed and the recipes index link to. fixtureMarketplaceDir
	// resolves it from the fixture testdata (keeping the CHARLY_DOCS_MARKETPLACE override, the
	// seam the RDD bed uses to pass a freshly regenerated corpus).
	pluginsDir := fixtureMarketplaceDir(t)

	return out, generate(root, out, pluginsDir)
}

// fixtureMarketplaceDir returns the fixture marketplace corpus (testdata/marketplace): a small,
// INTERNALLY CONSISTENT skill set — every reference a fixture card body makes resolves to a
// defined card — so the corpus-dependent tests exercise their WIRING hermetically, without
// fetching the published opencharly/marketplace repo (whose corpus can carry drift: the
// /charly-tools:crabbox skill referenced by three real recipe cards has no defined card). A
// CHARLY_DOCS_MARKETPLACE override wins when set (the RDD bed regenerates the corpus into a temp
// dir and passes it there); otherwise the fixture corpus is used.
func fixtureMarketplaceDir(t *testing.T) string {
	t.Helper()
	if dir := os.Getenv("CHARLY_DOCS_MARKETPLACE"); dir != "" {
		if fileExists(filepath.Join(dir, ".claude-plugin", "marketplace.json")) {
			return dir
		}
	}
	dir, err := filepath.Abs(filepath.Join("testdata", "marketplace"))
	if err != nil {
		t.Fatalf("resolve fixture marketplace dir: %v", err)
	}
	if !fileExists(filepath.Join(dir, ".claude-plugin", "marketplace.json")) {
		t.Fatalf("fixture marketplace corpus missing at %s — refresh it from testdata/marketplace", dir)
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

// fixtureProjectRoot returns the hermetic generate root: testdata/project, a MINIMAL
// self-contained charly project. Its charly.yml carries a docs: node with the compiled-in corpus
// DISABLED and no release/extra repos, so the catalog assembly (assembleCatalog) walks only the
// fixture tree and performs ZERO fetches — no network, no external charly/marketplace checkout.
// The README.md/VISION.md/GRIEVANCES.md/LIBERATION.md narratives the landing/vision/grievances/
// liberation passes project live beside it. The shape mirrors the in-memory fixture
// TestGenerateRegenNoOp builds (catalog_test.go), which is the proven hermetic minimal project.
func fixtureProjectRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("testdata", "project"))
	if err != nil {
		t.Fatalf("resolve fixture project root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, unifiedFileName)); err != nil {
		t.Fatalf("no fixture charly project at %s — refresh it from testdata/project: %v", dir, err)
	}
	return dir
}

// seedHandAuthoredPages copies the fixture's hand-authored pages into a fresh --out.
//
// The fixture lives in testdata/site: the two pages the generated landing actually links to
// (start/install and start/quickstart, the hero's Get-started actions). It is the hermetic
// replacement for the docs repo's hand-authored trees: the generator-wiring tests must run in a
// bare clone, so the seeded pages are deliberately small and their internal link graph CLOSED
// within what the fixture run emits — a seed linking to reference/ or recipes/ pages that only the
// full corpus and entity tree would produce would fail the site-link gate precisely because those
// pages do not exist here, and the site-link gate checking the seeded pages like any other page is
// the point.
//
// Pages are selected by the same rule pruneGeneratedPages uses — a page is generated if and only if
// it carries the generated header — rather than by a hardcoded list of directories, so a new
// hand-authored tree needs no change here and, more importantly, so nothing generated can leak in.
// That exclusion is what makes the assertions in these tests airtight: a seeded page can never
// carry the generated header, so a page that carries one was written by the run under test.
func seedHandAuthoredPages(t *testing.T, out string) {
	t.Helper()

	src, err := filepath.Abs(filepath.Join("testdata", "site"))
	if err != nil {
		t.Fatalf("resolve hand-authored fixture tree: %v", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("no hand-authored fixture tree at %s — refresh it from testdata/site: %v", src, err)
	}

	header := []byte(generatedHeader)
	err = filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
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
// CORPUS PRECONDITION: the seeded hand-authored pages and the generated landing link to the
// recipe pages generated from the FIXTURE marketplace corpus (testdata/marketplace, internally
// consistent by construction), so the run resolves every link against pages the fixture actually
// emits. No external corpus is fetched; the fixture replaces the published marketplace repo,
// which would at its current HEAD fail every one of these runs — its recipe card
// recipes/automation/crabbox-deploy.md references /charly-tools:crabbox, a skill with no defined
// card, and the cross-reference gate fails closed on it.
func TestGenerateWiresLeafGenerators(t *testing.T) {
	_ = fixtureMarketplaceDir(t) // resolve the fixture corpus (or fail) before the assertions
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
	_ = fixtureMarketplaceDir(t) // resolve the fixture corpus (or fail) before the assertions
	root := fixtureProjectRoot(t)
	base := t.TempDir()
	out := filepath.Join(base, "src", "content", "docs")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatalf("create content root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "astro.config.mjs"),
		[]byte(astroConfig("      { label: 'The Vision', link: '/vision/' },")), 0o644); err != nil {
		t.Fatalf("write astro config: %v", err)
	}
	seedHandAuthoredPages(t, out)

	kept := filepath.Join(out, "reference", "candy", "keep-me.md")
	if err := os.MkdirAll(filepath.Dir(kept), 0o755); err != nil {
		t.Fatalf("mkdir reference/candy: %v", err)
	}
	if err := os.WriteFile(kept, []byte("Hand-authored under a generated tree.\n"), 0o644); err != nil {
		t.Fatalf("write keep-me.md: %v", err)
	}

	if err := generate(root, out, fixtureMarketplaceDir(t)); err != nil {
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
	_ = fixtureMarketplaceDir(t) // resolve the fixture corpus (or fail) before the assertions
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

// copyTree copies a directory tree (files only) from src to dst.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, path[len(src):]), 0o755)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, path[len(src):]), raw, 0o644)
	})
}

// TestGenerateGateBeforePrune locks in the ordering fix for the prune-before-gate defect
// (opencharly/charly#333): a run the cross-reference gate rejects must leave the output tree
// intact. Before the fix, pruneGeneratedPages ran first, so a refused run deleted every page
// carrying the generated header — the site was left deleted on a run that wrote nothing.
func TestGenerateGateBeforePrune(t *testing.T) {
	// A copy of the fixture marketplace corpus with a bad reference injected: the gate must
	// reject it. The fixture card internals/skills/git-workflow/SKILL.md exists exactly so this
	// injection has a stable target that a bare clone carries.
	corpus := fixtureMarketplaceDir(t)
	badCorpus := t.TempDir()
	if err := copyTree(corpus, badCorpus); err != nil {
		t.Fatalf("copy corpus: %v", err)
	}
	// Inject an unresolvable reference into a skill body.
	skillPath := filepath.Join(badCorpus, "internals", "skills", "git-workflow", "SKILL.md")
	f, err := os.OpenFile(skillPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open skill for injection: %v", err)
	}
	if _, err := f.WriteString("\nSee /charly-nonexistent:fake-skill for details.\n"); err != nil {
		t.Fatalf("inject bad reference: %v", err)
	}
	f.Close()
	t.Setenv("CHARLY_DOCS_MARKETPLACE", badCorpus)

	root := fixtureProjectRoot(t)
	base := t.TempDir()
	out := filepath.Join(base, "src", "content", "docs")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatalf("create content root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "astro.config.mjs"),
		[]byte(astroConfig()), 0o644); err != nil {
		t.Fatalf("write astro config: %v", err)
	}
	seedHandAuthoredPages(t, out)

	// Seed a generated page that the prune would delete if it ran first.
	seeded := filepath.Join(out, "seed.md")
	if err := os.WriteFile(seeded, []byte(generatedHeader+"\n# seed\n"), 0o644); err != nil {
		t.Fatalf("seed generated page: %v", err)
	}

	err = generate(root, out, badCorpus)
	if err == nil {
		t.Fatal("generate: expected the cross-reference gate to reject the bad reference, got nil")
	}
	if !strings.Contains(err.Error(), "/charly-nonexistent:fake-skill") {
		t.Fatalf("generate: expected the cross-reference error naming the bad ref, got: %v", err)
	}
	// The refused run must leave the output tree intact: the seeded page survives.
	if _, statErr := os.Stat(seeded); statErr != nil {
		t.Fatalf("refused run pruned the output tree: seeded page %s is gone (%v)", seeded, statErr)
	}
}
