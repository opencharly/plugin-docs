package docs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/opencharly/spec/refs"
)

// TestCollectCandySkills_ProjectsMovedCandySkill is the R10 gate for the candy-skill
// projection (collectCandySkills + the generate() append): a moved candy's skill: entity
// (family + content + references) must project into the skill set from the SAME remote-aware
// walk that resolves the candy. FAILS without the projection: the pre-Phase-3 generator read
// skills ONLY from the (stale) marketplace corpus, so a moved candy's skill reference dangled.
func TestCollectCandySkills_ProjectsMovedCandySkill(t *testing.T) {
	// A synthetic local project referencing a REAL moved candy with a skill: entity. Use
	// layer-ripgrep (the Phase-0 pilot — carries ripgrep-skill with family: tools).
	tag := newestTag(t, "github.com/opencharly/layer-ripgrep")
	if _, err := refs.DownloadRepo("github.com/opencharly/layer-ripgrep", tag); err != nil {
		t.Fatalf("fetch layer-ripgrep: %v", err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "candy", "local-keeper"), 0o755); err != nil {
		t.Fatal(err)
	}
	ref := "@github.com/opencharly/layer-ripgrep:" + tag
	local := "local-keeper:\n    candy:\n        version: 2026.200.1000\n        description: Local fixture.\n        candy:\n            - '" + ref + "'\n"
	if err := os.WriteFile(filepath.Join(root, "candy", "local-keeper", "charly.yml"), []byte(local), 0o644); err != nil {
		t.Fatal(err)
	}

	ents, err := walkRemote([]repoRoot{{Namespace: "", Dir: root}})
	if err != nil {
		t.Fatalf("walkRemote: %v", err)
	}
	skills := collectCandySkills(ents)
	found := false
	for _, s := range skills {
		if s.Name == "ripgrep" {
			found = true
			if s.PluginDir != "tools" {
				t.Fatalf("ripgrep skill family = %q, want tools", s.PluginDir)
			}
			if s.Body == "" {
				t.Fatal("ripgrep skill body empty — the projected card would be blank")
			}
		}
	}
	if !found {
		t.Fatalf("moved candy skill ripgrep not projected from the remote walk; got %d skills", len(skills))
	}
	t.Logf("projected %d candy skills incl. ripgrep (family=tools)", len(skills))
}

// TestDedupeSkills_CorpusWinsAndWalkFillsGaps is the R10 gate for the recipes-index dedupe.
// The corpus read and the candy walk project the SAME skill entities once the marketplace has
// been regenerated, and the walk visits a candy more than once; appending them unfiltered made
// recipes/index.md list one card 2-3 times. FAILS without dedupeSkills: the merged slice carries
// one entry per source occurrence instead of one per skillKey.
func TestDedupeSkills_CorpusWinsAndWalkFillsGaps(t *testing.T) {
	corpus := []skill{
		{PluginDir: "tools", Name: "ripgrep", Body: "corpus-body"},
		{PluginDir: "tools", Name: "yay", Body: "corpus-yay"},
	}
	walked := []skill{
		{PluginDir: "tools", Name: "ripgrep", Body: "walked-body"},       // duplicate of corpus
		{PluginDir: "tools", Name: "ripgrep", Body: "walked-body-again"}, // duplicate of the duplicate
		{PluginDir: "tools", Name: "goplaces", Body: "walked-only"},      // gap the walk fills
	}
	got := dedupeSkills(corpus, walked)

	// One entry per skillKey.
	seen := map[string]int{}
	for _, s := range got {
		seen[skillKey(s)]++
	}
	for k, n := range seen {
		if n != 1 {
			t.Errorf("skillKey %q appears %d times, want 1", k, n)
		}
	}
	if len(got) != 3 {
		t.Fatalf("want 3 distinct skills (ripgrep, yay, goplaces), got %d", len(got))
	}
	// The corpus copy wins over the walked copy.
	for _, s := range got {
		if s.Name == "ripgrep" && s.Body != "corpus-body" {
			t.Errorf("corpus must win for a duplicate skill; got body %q", s.Body)
		}
	}
	// The walk's unique contribution survives.
	found := false
	for _, s := range got {
		if s.Name == "goplaces" {
			found = true
		}
	}
	if !found {
		t.Error("the walk's corpus-absent skill (goplaces) must survive the merge")
	}
}
