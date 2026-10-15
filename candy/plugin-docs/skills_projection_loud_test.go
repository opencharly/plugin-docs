package docs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// skills_projection_loud_test.go — plugin-docs#18: the complement must fail LOUDLY.
//
// `collectCandySkills` used to `continue` when a candy `skill:` entity failed to decode. That dropped
// the entity from the docs skill projection with no signal at all: the complement that keeps a moved
// candy's recipe references resolving would simply omit the skill, every page would still build, and
// nothing anywhere would say so — which is how a 215-file marketplace pin lag hid behind it.
//
// This test is the difference between the two behaviours. It FAILS on the old code (which returned a
// shorter slice and no error) and PASSES on the new one (which returns an error naming the entity and
// its source file). Note what it is NOT: it is not a loosening of the cross-reference gate — that gate
// stays exactly as strict as it was, and this is a separate control at a separate seam.
func TestCollectCandySkills_FailsLoudlyOnADecodeError(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "candy", "broken"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A `skill:` node whose value is a scalar, not a mapping: the kind discriminator is present, so
	// the walker yields the entity, and the decode into spec.Skill then cannot succeed.
	const broken = "broken:\n" +
		"    candy:\n" +
		"        version: 2026.200.1000\n" +
		"        description: Local fixture.\n" +
		"        candy:\n" +
		"            - 'x'\n" +
		"broken-skill:\n" +
		"    skill: \"not a mapping\"\n"
	if err := os.WriteFile(filepath.Join(root, "candy", "broken", "charly.yml"), []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}

	ents, err := walkRemote([]repoRoot{{Namespace: "", Dir: root}})
	if err != nil {
		t.Fatalf("walkRemote: %v", err)
	}

	out, err := collectCandySkills(ents)
	if err == nil {
		t.Fatalf("a candy skill entity that cannot decode was dropped silently (got %d skills, no error) — "+
			"the complement would omit it and every page would still build: that is plugin-docs#18", len(out))
	}
	// The message must be actionable: which entity, and which file it came from.
	if !strings.Contains(err.Error(), "broken-skill") {
		t.Errorf("error does not name the entity: %v", err)
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Errorf("error does not name the source file/dir: %v", err)
	}
}
