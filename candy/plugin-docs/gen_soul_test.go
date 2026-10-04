package docs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateSoulPublishesSource covers the projection: SOUL.md is published verbatim apart from
// the one mechanical edit — the source H1 is dropped because Starlight renders the frontmatter
// title as the page heading, so keeping it shows the title twice. SOUL.md is deliberately
// self-contained (it cites no other document and carries no links), so there is no link rewrite,
// and the body's identity prose survives untouched.
func TestGenerateSoulPublishesSource(t *testing.T) {
	root := writeTree(t, map[string]string{
		"SOUL.md": strings.Join([]string{
			"# SOUL.md — Who You Are",
			"",
			"You are charly — a someone, not a something.",
			"",
			"## What you stand for",
			"",
			"A lie left standing in the record is something you cannot bear.",
			"",
		}, "\n"),
	})
	out := t.TempDir()

	if err := generateSoul(root, out); err != nil {
		t.Fatalf("generateSoul: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(out, "soul.md"))
	if err != nil {
		t.Fatalf("read emitted soul.md: %v", err)
	}
	got := string(raw)

	if !strings.Contains(got, "title: \"Who We Are\"") {
		t.Errorf("emitted page is missing its frontmatter title:\n%s", got)
	}
	// Without the header the page is invisible to pruneGeneratedPages and would survive as an
	// orphan the moment the generator stopped emitting it.
	if !strings.Contains(got, generatedHeader) {
		t.Errorf("emitted page is missing the generated header:\n%s", got)
	}
	if strings.Contains(got, "# SOUL.md — Who You Are") {
		t.Errorf("the source H1 should be dropped, the frontmatter title renders it:\n%s", got)
	}
	if !strings.Contains(got, "You are charly — a someone, not a something.") {
		t.Errorf("the source body was not published:\n%s", got)
	}
	if !strings.Contains(got, "## What you stand for") {
		t.Errorf("a section heading was dropped:\n%s", got)
	}
}

// TestGenerateSoulMissingSource keeps the generator from emitting an empty page when its single
// source is absent.
func TestGenerateSoulMissingSource(t *testing.T) {
	err := generateSoul(t.TempDir(), t.TempDir())
	if err == nil {
		t.Fatal("expected an error when SOUL.md is absent, got nil")
	}
	if !strings.Contains(err.Error(), "SOUL.md") {
		t.Errorf("error should name the missing source, got: %v", err)
	}
}
