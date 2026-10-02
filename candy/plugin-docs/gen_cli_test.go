package docs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opencharly/spec/spec"
)

// cliFixture builds two command words from one plugin slice: `solo`, served by a single
// candy, and `shared`, served by two. Both shapes must be emitted from the same call,
// because every defect this test guards came from a multi-owner word being rendered with
// logic that only survives contact with single-owner pages.
func cliFixture() []pluginEntity {
	return []pluginEntity{
		{entity: entity{Name: "plugin-box", Candy: &candyView{
			Version: "2026.194.0000",
			Plugin:  &spec.Plugin{Providers: []spec.PluginCapability{"command:shared", "command:solo"}}}}, CompiledIn: true},
		{entity: entity{Name: "plugin-feature", Candy: &candyView{
			Version: "2026.179.0000",
			Plugin:  &spec.Plugin{Providers: []spec.PluginCapability{"command:shared"}}}}, CompiledIn: true},
	}
}

// TestGenerateCLIMultiOwnerPageNamesEveryOwner locks the two reader-visible surfaces of a
// word served by more than one candy. Both regressed silently in production and neither is
// caught by counting pages: the page existed and the build was green.
func TestGenerateCLIMultiOwnerPageNamesEveryOwner(t *testing.T) {
	out := t.TempDir()
	if _, err := generateCLI(out, cliFixture()); err != nil {
		t.Fatalf("generateCLI: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(out, "reference", "cli", "shared.md"))
	if err != nil {
		t.Fatalf("read shared.md: %v", err)
	}
	got := string(raw)

	// The frontmatter description is ALSO the search-result text, so it is read before the
	// body. "served by THE x plugin candy" asserts an exclusivity the body contradicts, and
	// naming either owner alone is the same defect — a fix that swapped which one it named
	// would look like a cure while preserving it.
	if !strings.Contains(got, `description: "The shared command word, served by 2 plugin candies: plugin-box and plugin-feature."`) {
		t.Errorf("multi-owner description does not name both owners:\n%s", firstLines(got, 4))
	}
	if strings.Contains(got, "served by the plugin-box plugin candy") ||
		strings.Contains(got, "served by the plugin-feature plugin candy") {
		t.Errorf("multi-owner description still asserts a single owner:\n%s", firstLines(got, 4))
	}

	// The fact table must be keyed by owner. Unkeyed, the rows repeat with no grouping —
	// and while both Placement values may coincide, the Version values do not, so a reader
	// cannot tell which version belongs to which candy.
	for _, want := range []string{
		"| **plugin-box — Version** | `2026.194.0000` |",
		"| **plugin-feature — Version** | `2026.179.0000` |",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("fact table missing owner-keyed row %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "| **Version** |") {
		t.Errorf("multi-owner fact table still carries an unkeyed Version row:\n%s", got)
	}
}

// TestGenerateCLISingleOwnerPageIsUnchanged is the other half: the multi-owner cure must not
// leak into the common case. A single-owner page keeps the definite article, which is true
// there, and the plain field names it has always had.
func TestGenerateCLISingleOwnerPageIsUnchanged(t *testing.T) {
	out := t.TempDir()
	if _, err := generateCLI(out, cliFixture()); err != nil {
		t.Fatalf("generateCLI: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(out, "reference", "cli", "solo.md"))
	if err != nil {
		t.Fatalf("read solo.md: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, `description: "The solo command word, served by the plugin-box plugin candy."`) {
		t.Errorf("single-owner description changed:\n%s", firstLines(got, 4))
	}
	if !strings.Contains(got, "| **Version** | `2026.194.0000` |") {
		t.Errorf("single-owner fact table should keep plain field names:\n%s", got)
	}
	if strings.Contains(got, "plugin-box — Version") {
		t.Errorf("single-owner page should not be owner-keyed:\n%s", got)
	}
}

// TestGenerateCLIWordServedTwiceGetsOnePage guards the original silent overwrite: keyed by
// word with one write per PROVIDER, the second write clobbered the first — 56 emitted, 55
// written, exit 0. The count is the assertion, since the page existed either way.
func TestGenerateCLIWordServedTwiceGetsOnePage(t *testing.T) {
	out := t.TempDir()
	n, err := generateCLI(out, cliFixture())
	if err != nil {
		t.Fatalf("generateCLI: %v", err)
	}
	if n != 2 {
		t.Errorf("returned count = %d, want 2 (distinct words, not providers)", n)
	}
	ents, err := os.ReadDir(filepath.Join(out, "reference", "cli"))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(ents) != n {
		t.Errorf("emitted %d, wrote %d — a page was silently overwritten", n, len(ents))
	}
}

// TestGenerateCLIPagePathIsSanitized guards page path == served URL == link target for a
// command word that carries a colon. The `noun:verb` form is the normal spelling of a nested
// command (twelve live under `box` alone), and this emitter was the ONE page-path producer
// that wrote the raw identity instead of running it through sanitizeSegment — every link
// target already sanitizes each segment (sanitizeSitePathSegments), so the page was emitted at
// a name its own links did not point at.
//
// The failure is invisible in the file name and only shows up as a URL: Astro strips `:` when
// it derives a route slug, so `reference/cli/build:box.md` is served at
// `/reference/clibuildbox/` while every generated link goes to `/reference/cli/build-box/`.
// Nineteen such pages were committed to the docs repo before this test existed.
//
// This is asserted as a PAIR — the sanitized file exists AND the raw one does not — because
// checking only the first would pass on a generator that wrote both.
func TestGenerateCLIPagePathIsSanitized(t *testing.T) {
	out := t.TempDir()
	plugins := []pluginEntity{
		{entity: entity{Name: "plugin-box", Candy: &candyView{
			Version: "2026.194.0000",
			Plugin:  &spec.Plugin{Providers: []spec.PluginCapability{"command:build:box"}}}}, CompiledIn: true},
	}
	if _, err := generateCLI(out, plugins); err != nil {
		t.Fatalf("generateCLI: %v", err)
	}

	// The word keeps its raw spelling in the page's own prose — only the PATH is sanitized.
	raw, err := os.ReadFile(filepath.Join(out, "reference", "cli", "build-box.md"))
	if err != nil {
		t.Fatalf("sanitized page reference/cli/build-box.md not emitted: %v", err)
	}
	if !strings.Contains(string(raw), "build:box") {
		t.Errorf("the word's raw spelling should still be rendered on the page:\n%s", firstLines(string(raw), 6))
	}

	if _, err := os.Stat(filepath.Join(out, "reference", "cli", "build:box.md")); err == nil {
		t.Errorf("raw colon page reference/cli/build:box.md was still written — " +
			"the page path did not go through sanitizeSegment")
	}

	// And the invariant the sanitizer exists to establish: the emitted path is exactly what
	// the link-target sanitizer produces for the same word.
	want := "reference/cli/" + sanitizeSegment("build:box") + ".md"
	if got := "reference/cli/build-box.md"; got != want {
		t.Errorf("page path %q does not match the link-target sanitization %q", got, want)
	}
}

func firstLines(s string, n int) string {
	parts := strings.SplitN(s, "\n", n+1)
	if len(parts) > n {
		parts = parts[:n]
	}
	return strings.Join(parts, "\n")
}
