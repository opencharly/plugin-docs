package docs

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/opencharly/sdk/candywalk"
)

// retired.go — the corpus's RETIREMENT list, honoured by the candy-walk complement.
//
// THE DEFECT THIS CLOSES (opencharly/plugin-docs#27). `generate` merges the corpus's skills with
// the candy `skill:` entities a remote-aware walk collects (`collectCandySkills`), keeping the
// corpus's copy when both exist and the walk's when the corpus lacks it — the complement that
// keeps a MOVED candy's recipe references resolving between a candy move and the next corpus
// regeneration. A skill the corpus has RETIRED is, by definition, absent from the corpus, so the
// walk always wins it back: after opencharly/marketplace#644 stopped publishing `immich-ml`
// (retired.cue), this generator still wrote `recipes/immich/immich-ml.md` from the retired
// pod-immich-ml tree, reached — as opencharly/plugin-marketplace#63 measured — through stale
// subpath pins into old `distro-*` trees.
//
// THE POLICY IS DATA, in ONE place: `retired.cue` at the corpus root, the same file
// opencharly/plugin-marketplace#65 defines and validates against its served `#Retired`. `--plugins`
// already names the corpus checkout, so the list is read from beside `marketplace.json`. A retired
// repo's entities are never contributed by the walk.
const retiredFileName = "retired.cue"

// retiredSchema is the SAME closed shape as the generator's `#Retired` (a 3-line definition). It is
// restated here rather than imported because plugin-docs must not depend on the marketplace
// plugin's module; the contract is the FIELD NAME and the closed struct, and a drift in either
// fails this read loudly rather than silently retiring nothing.
const retiredSchema = `
#Retired: {
	retired: [...string]
	retired_skills?: [...string]
}
`

// retiredSet is what the corpus has retired: whole REPOS, and individual SKILL cards. The card
// half exists because a stale card does not always come with a retired repo — the thing it
// describes can be consolidated or removed while its owner lives on, and its node then survives
// only in the owner's old tags (opencharly/plugin-marketplace#64).
type retiredSet struct {
	repos  map[string]bool
	skills map[string]bool // "<family>/<name>"
}

// skillRetired reports whether a skill card's (family, name) is retired.
func (r retiredSet) skillRetired(family, name string) bool {
	return r.skills[family+"/"+name]
}

// loadRetired reads the corpus's retirement list from its root. A corpus that has retired nothing
// carries no file, and that is a no-op — every corpus had that shape before the list existed.
// Every OTHER problem is fatal: a malformed list must never degrade to "nothing retired".
func loadRetired(pluginsDir string) (retiredSet, error) {
	path := filepath.Join(pluginsDir, retiredFileName)
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return retiredSet{repos: map[string]bool{}, skills: map[string]bool{}}, nil
	}
	if err != nil {
		return retiredSet{}, fmt.Errorf("read %s: %w", retiredFileName, err)
	}
	repos, skills, err := parseRetired(raw)
	if err != nil {
		return retiredSet{}, fmt.Errorf("%s: %w", retiredFileName, err)
	}
	out := retiredSet{repos: make(map[string]bool, len(repos)), skills: make(map[string]bool, len(skills))}
	for _, p := range repos {
		out.repos[p] = true
	}
	for _, p := range skills {
		out.skills[p] = true
	}
	return out, nil
}

// parseRetired validates one retirement list against the closed shape and returns its repo paths
// in the form the walk's export dirs carry (`github.com/opencharly/<repo>`).
func parseRetired(raw []byte) (repos, skills []string, err error) {
	ctx := cuecontext.New()
	schema := ctx.CompileString(retiredSchema)
	if err := schema.Err(); err != nil {
		return nil, nil, fmt.Errorf("compile the retirement schema: %w", err)
	}
	def := schema.LookupPath(cue.ParsePath("#Retired"))
	if !def.Exists() {
		return nil, nil, errors.New("the retirement schema declares no #Retired")
	}
	data := ctx.CompileBytes(raw)
	if err := data.Err(); err != nil {
		return nil, nil, err
	}
	unified := data.Unify(def)
	if err := unified.Validate(); err != nil {
		return nil, nil, fmt.Errorf("does not match #Retired: %w", err)
	}
	var doc struct {
		Retired       []string `json:"retired"`
		RetiredSkills []string `json:"retired_skills"`
	}
	if err := unified.Decode(&doc); err != nil {
		return nil, nil, err
	}
	for _, p := range doc.Retired {
		repoPath := strings.TrimPrefix(strings.TrimSpace(p), "@")
		if !strings.HasPrefix(repoPath, "github.com/") {
			return nil, nil, fmt.Errorf("retired entry %q is not a fully qualified repo path (want e.g. \"github.com/opencharly/pod-immich-ml\")", p)
		}
		repos = append(repos, repoPath)
	}
	for _, p := range doc.RetiredSkills {
		key := strings.TrimSpace(p)
		family, name, ok := strings.Cut(key, "/")
		if !ok || family == "" || name == "" || strings.Contains(name, "/") {
			return nil, nil, fmt.Errorf("retired skill entry %q is not <family>/<name>", p)
		}
		skills = append(skills, key)
	}
	return repos, skills, nil
}

// isRetiredEntity reports whether a walked entity came from a repo the corpus has retired.
// Entity.SourceRoot is the fetched export dir (`<cache>/.files/<repoPath>@<version>`); a LOCAL
// entity ("" — the generator's own tree) is never retired by this list, which names repos the
// corpus stopped publishing from, not skills the root tree defines.
func isRetiredEntity(e candywalk.Entity, retired map[string]bool) bool {
	if len(retired) == 0 || e.SourceRoot == "" {
		return false
	}
	stem := exportedRepoStem(e.SourceRoot)
	if stem == "" {
		return false
	}
	for repoPath := range retired {
		if strings.HasSuffix(stem, string(filepath.Separator)+repoPath) {
			return true
		}
	}
	return false
}

// exportedRepoStem drops the `@<version>` suffix from a files-only export dir, returning the
// repo-path stem. It returns "" for a dir that does not carry that shape, which keeps an
// unrecognised layout from silently retiring anything.
func exportedRepoStem(dir string) string {
	d := filepath.Clean(dir)
	base := filepath.Base(d)
	at := strings.LastIndex(base, "@")
	if at <= 0 || at == len(base)-1 {
		return ""
	}
	return filepath.Join(filepath.Dir(d), base[:at])
}
