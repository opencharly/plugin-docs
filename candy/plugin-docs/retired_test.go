package docs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opencharly/sdk/candywalk"
	"gopkg.in/yaml.v3"
)

// TestCollectCandySkills_SkipsRetiredRepo is the R10 gate for opencharly/plugin-docs#27: the
// candy-walk complement must never contribute a skill that came from a repo the corpus has
// retired.
//
// FAILS WITHOUT THE CHANGE: the walk contributes every `skill:` entity it reaches, so a retired
// repo's skill (absent from the corpus by definition) always won the dedupe back — measured:
// `recipes/immich/immich-ml.md` was re-emitted after opencharly/marketplace#644 retired it.
func TestCollectCandySkills_SkipsRetiredRepo(t *testing.T) {
	ent := func(name, sourceRoot string) candywalk.Entity {
		var doc yaml.Node
		body := "name: " + name + "\nfamily: immich\ncontent: |-\n  Body of " + name + ".\n"
		if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
			t.Fatalf("fixture %s: %v", name, err)
		}
		return candywalk.Entity{Name: name, Kind: "skill", Dir: "candy/fixture", SourceRoot: sourceRoot, Value: *doc.Content[0]}
	}
	retired := map[string]bool{"github.com/opencharly/pod-immich-ml": true}
	ents := []candywalk.Entity{
		ent("immich-ml", "/cache/.files/github.com/opencharly/pod-immich-ml@v2026.274.0811"),
		ent("immich", "/cache/.files/github.com/opencharly/pod-immich@v2026.327.0043"),
		// A LOCAL entity (the generator's own tree) is never retired by a repo list.
		ent("local-skill", ""),
	}
	got, err := collectCandySkills(ents, retiredSet{repos: retired})
	if err != nil {
		t.Fatalf("collectCandySkills: %v", err)
	}
	names := make([]string, 0, len(got))
	for _, s := range got {
		names = append(names, s.Name)
	}
	joined := strings.Join(names, ",")
	if strings.Contains(joined, "immich-ml") {
		t.Fatalf("a retired repo's skill was projected: %v", names)
	}
	for _, want := range []string{"immich", "local-skill"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("%q missing from the projection: %v", want, names)
		}
	}
}

// TestParseRetired pins the retirement list's shape and its strictness — a typo must fail the
// generation, never quietly retire nothing. Both lists are covered: REPOS and SKILL CARDS.
func TestParseRetired(t *testing.T) {
	cases := []struct {
		name       string
		src        string
		wantRepos  string
		wantSkills string
		wantErr    string
	}{
		{
			name:      "valid repos, @ form accepted",
			src:       "retired: [\"github.com/opencharly/a\", \"@github.com/opencharly/b\"]",
			wantRepos: "github.com/opencharly/a,github.com/opencharly/b",
		},
		{
			name:       "valid skill cards",
			src:        "retired: []\nretired_skills: [\"openclaw/openclaw-desktop\", \"automation/herdr\"]",
			wantSkills: "openclaw/openclaw-desktop,automation/herdr",
		},
		{name: "empty list", src: "retired: []"},
		{name: "unknown field is closed", src: "retired: []\nretierd: []\n", wantErr: "field not allowed"},
		{name: "unqualified path", src: "retired: [\"pod-immich-ml\"]", wantErr: "not a fully qualified repo path"},
		{name: "a card needs family/name", src: "retired: []\nretired_skills: [\"herdr\"]", wantErr: "is not <family>/<name>"},
		{name: "a card has exactly two segments", src: "retired: []\nretired_skills: [\"a/b/c\"]", wantErr: "is not <family>/<name>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repos, skills, err := parseRetired([]byte(tc.src))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseRetired: %v", err)
			}
			if strings.Join(repos, ",") != tc.wantRepos {
				t.Fatalf("repos = %v, want %q", repos, tc.wantRepos)
			}
			if strings.Join(skills, ",") != tc.wantSkills {
				t.Fatalf("skills = %v, want %q", skills, tc.wantSkills)
			}
		})
	}
}

// TestCollectCandySkills_SkipsRetiredCard: a card named in `retired_skills` is never contributed,
// whatever tree carries it, while a live sibling is — the control opencharly/plugin-marketplace#64
// named, honoured here too because the docs complement reads the same file.
//
// FAILS WITHOUT THE CHANGE: the complement skipped only retired REPOS, so a retired card came
// back from any stale edge.
func TestCollectCandySkills_SkipsRetiredCard(t *testing.T) {
	ent := func(name, family string) candywalk.Entity {
		var doc yaml.Node
		body := "name: " + name + "\nfamily: " + family + "\ncontent: |-\n  Body of " + name + ".\n"
		if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
			t.Fatalf("fixture %s: %v", name, err)
		}
		return candywalk.Entity{Name: name, Kind: "skill", Dir: "candy/fixture", Value: *doc.Content[0]}
	}
	retired := retiredSet{repos: map[string]bool{}, skills: map[string]bool{"openclaw/openclaw-desktop": true}}
	ents := []candywalk.Entity{
		ent("openclaw-desktop", "openclaw"),
		ent("openclaw", "openclaw"),
	}
	got, err := collectCandySkills(ents, retired)
	if err != nil {
		t.Fatalf("collectCandySkills: %v", err)
	}
	var names []string
	for _, s := range got {
		names = append(names, s.Name)
	}
	joined := strings.Join(names, ",")
	if strings.Contains(joined, "openclaw-desktop") {
		t.Fatalf("a retired card was projected: %v", names)
	}
	if !strings.Contains(joined, "openclaw") {
		t.Fatalf("the live sibling was dropped: %v", names)
	}
}

// TestLoadRetired_AbsentIsEmptyAndMalformedIsFatal: a corpus with no list retires nothing; a
// present-but-broken list must fail rather than degrade to "nothing retired".
func TestLoadRetired_AbsentIsEmptyAndMalformedIsFatal(t *testing.T) {
	dir := t.TempDir()
	got, err := loadRetired(dir)
	if err != nil || len(got.repos) != 0 || len(got.skills) != 0 {
		t.Fatalf("loadRetired(absent) = %+v, %v; want empty, nil", got, err)
	}
	if err := os.WriteFile(filepath.Join(dir, retiredFileName), []byte("retired: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadRetired(dir); err == nil {
		t.Fatal("loadRetired accepted a malformed list")
	}
}
