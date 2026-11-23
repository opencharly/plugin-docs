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
	got, err := collectCandySkills(ents, retired)
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
// generation, never quietly retire nothing.
func TestParseRetired(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		want    string
		wantErr string
	}{
		{name: "valid, @ form accepted", src: "retired: [\"github.com/opencharly/a\", \"@github.com/opencharly/b\"]", want: "github.com/opencharly/a,github.com/opencharly/b"},
		{name: "empty list", src: "retired: []"},
		{name: "unknown field is closed", src: "retired: []\nretierd: []\n", wantErr: "field not allowed"},
		{name: "unqualified path", src: "retired: [\"pod-immich-ml\"]", wantErr: "not a fully qualified repo path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseRetired([]byte(tc.src))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseRetired: %v", err)
			}
			if strings.Join(got, ",") != tc.want {
				t.Fatalf("retired = %v, want %q", got, tc.want)
			}
		})
	}
}

// TestLoadRetired_AbsentIsEmptyAndMalformedIsFatal: a corpus with no list retires nothing; a
// present-but-broken list must fail rather than degrade to "nothing retired".
func TestLoadRetired_AbsentIsEmptyAndMalformedIsFatal(t *testing.T) {
	dir := t.TempDir()
	got, err := loadRetired(dir)
	if err != nil || len(got) != 0 {
		t.Fatalf("loadRetired(absent) = %v, %v; want empty, nil", got, err)
	}
	if err := os.WriteFile(filepath.Join(dir, retiredFileName), []byte("retired: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadRetired(dir); err == nil {
		t.Fatal("loadRetired accepted a malformed list")
	}
}
