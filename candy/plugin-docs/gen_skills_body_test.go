package docs

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opencharly/sdk/candywalk"
	"gopkg.in/yaml.v3"
)

// entity builds one candy `skill:` node, optionally with an inline body.
func entityNode(t *testing.T, name, sourceRoot, inlineBody string) candywalk.Entity {
	t.Helper()
	body := "name: " + name + "\nfamily: fixtures\n"
	if inlineBody != "" {
		body += "content: |-\n  " + inlineBody + "\n"
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return candywalk.Entity{Name: name, Kind: "skill", Dir: "candy/fixture", SourceRoot: sourceRoot, Value: *doc.Content[0]}
}

// writeSkillFile materializes a repo-root skill file (and one reference) at SourceRoot.
func writeSkillFile(t *testing.T, repoRoot, name, body string, withReference bool) {
	t.Helper()
	dir := filepath.Join(repoRoot, candywalk.AgentsSkillsDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	entry := "---\nname: " + name + "\ndescription: file description\n---\n\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(entry), 0o644); err != nil {
		t.Fatal(err)
	}
	if withReference {
		if err := os.MkdirAll(filepath.Join(dir, "references"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "references", "detail.md"), []byte("# detail\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestCollectCandySkills_SkipsBodilessNodeVisibly covers the SKIP branch: a node with neither an
// inline `content:` nor a SKILL.md contributes no card, and says so on stderr.
//
// FAILS WITHOUT THE CHANGE: the pre-fix complement appended a card with an empty body (the blank
// card of opencharly/plugin-docs#28). The live no-op proof cannot catch a regression here — a
// blank card is superseded by the corpus and the published tree stays byte-identical — so this
// unit case is the only proof.
func TestCollectCandySkills_SkipsBodilessNodeVisibly(t *testing.T) {
	emptyRepo := t.TempDir() // no .agents/skills at all
	ents := []candywalk.Entity{
		entityNode(t, "bodiless", emptyRepo, ""),
		entityNode(t, "inline", emptyRepo, "Inline body."),
	}

	// Capture the visible skip line: it is the only signal that the node was dropped.
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	got, cerr := collectCandySkills(ents, retiredSet{})
	_ = w.Close()
	os.Stderr = old
	captured, _ := io.ReadAll(r)
	if cerr != nil {
		t.Fatalf("collectCandySkills: %v", cerr)
	}

	var names []string
	for _, s := range got {
		names = append(names, s.Name)
	}
	if strings.Contains(strings.Join(names, ","), "bodiless") {
		t.Fatalf("a node with no body was projected: %v", names)
	}
	if !strings.Contains(strings.Join(names, ","), "inline") {
		t.Fatalf("the inline-bodied node was dropped: %v", names)
	}
	if !strings.Contains(string(captured), "skipping candy skill") || !strings.Contains(string(captured), "bodiless") {
		t.Fatalf("the skip was silent — stderr = %q", captured)
	}
}

// TestCollectCandySkills_FileBodyAndReferences covers the second half of the fix: a node with no
// inline body takes BOTH its body and its references from the file.
//
// FAILS WITHOUT THE CHANGE: the pre-fix complement read neither, so the card was blank and the
// reference pages it should have emitted were absent.
func TestCollectCandySkills_FileBodyAndReferences(t *testing.T) {
	repo := t.TempDir()
	writeSkillFile(t, repo, "filed", "Body from the file.", true)
	got, err := collectCandySkills([]candywalk.Entity{entityNode(t, "filed", repo, "")}, retiredSet{})
	if err != nil {
		t.Fatalf("collectCandySkills: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("projected %d skills, want 1", len(got))
	}
	if strings.TrimSpace(got[0].Body) != "Body from the file." {
		t.Fatalf("body = %q, want the file's body", got[0].Body)
	}
	if len(got[0].References) != 1 || got[0].References[0].Stem != "detail" {
		t.Fatalf("references = %+v, want the file's reference `detail`", got[0].References)
	}
}
