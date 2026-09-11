package docs

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestCompiledPluginRef locks the version-mapping contract: a go.mod require pin of a compiled
// plugin module (v0.YYYYDDD.C) maps onto the repo CalVer tag (v2026.DDD.CCCC), and the counter
// is RE-PADDED from its numeric value - so padded and unpadded pins of the same release
// converge (0005-to-5 and 0829-to-829 below), and a 4-digit counter passes through unchanged.
func TestCompiledPluginRef(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		// Padded vs unpadded counter of the same release converge (the mapping contract cases).
		{"v0.2026250.0005", "v2026.250.0005"}, // padded pin form
		{"v0.2026250.5", "v2026.250.0005"},    // go.mod canonicalizes the counter: 0005-to-5
		{"v0.2026250.0829", "v2026.250.0829"}, // padded pin form
		{"v0.2026250.829", "v2026.250.0829"},  // unpadded: 0829-to-829
		// Real pins from the charly compiled corpus.
		{"v0.2026254.5", "v2026.254.0005"},    // plugin-check, pinned unpadded
		{"v0.2026250.150", "v2026.250.0150"},  // plugin-check tag v2026.250.0150
		{"v0.2026237.1413", "v2026.237.1413"}, // plugin-addr: already 4 digits
		{"v0.2026251.0500", "v2026.251.0500"}, // padded 4-digit form passes through
	} {
		got, err := compiledPluginRef(tc.in)
		if err != nil {
			t.Errorf("compiledPluginRef(%q): unexpected error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("compiledPluginRef(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	for _, bad := range []string{"", "v1.2026250.5", "v0.2026250", "v0.2025.5", "v0.2026250.x", "v0.2026250.-1"} {
		if _, err := compiledPluginRef(bad); err == nil {
			t.Errorf("compiledPluginRef(%q): expected an error, got none", bad)
		}
	}
}

// writeRoot writes a fixture project root: charly.yml, charly/charly.yml (the compiled corpus
// manifest at the config default path), charly/charly/go.mod (the go.mod at the config default
// path), and a candy/ tree. Each optional field is "" to skip the file.
func writeRoot(t *testing.T, root, rootYML, charlyYML, goMod string, candyTree string) {
	t.Helper()
	if rootYML != "" {
		if err := os.WriteFile(filepath.Join(root, unifiedFileName), []byte(rootYML), 0o644); err != nil {
			t.Fatalf("write root charly.yml: %v", err)
		}
	}
	if charlyYML != "" {
		if err := os.MkdirAll(filepath.Join(root, "charly"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "charly", unifiedFileName), []byte(charlyYML), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if goMod != "" {
		if err := os.MkdirAll(filepath.Join(root, "charly", "charly"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "charly", "charly", "go.mod"), []byte(goMod), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if candyTree != "" && candyTree != "none" {
		if err := os.MkdirAll(filepath.Join(root, candyTree), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// writeCandyAt writes a name-first candy manifest under dir (root-relative).
func writeCandyAt(t *testing.T, root, dir, body string) string {
	t.Helper()
	full := filepath.Join(root, dir)
	if err := os.MkdirAll(full, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(full, unifiedFileName)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// pluginCandyManifest is a name-first candy manifest for a PLUGIN candy (name = the candy name).
func pluginCandyManifest(name, version string) string {
	return name + ":" + "\n" +
		"    candy:" + "\n" +
		"        version: " + version + "\n" +
		"        description: Fixture plugin " + name + "." + "\n" +
		"        plugin:" + "\n" +
		"            source: github.com/opencharly/" + name + "/candy/" + name + "\n" +
		"            providers:" + "\n" +
		"                - verb:alpha" + "\n"
}

// writePluginRepo writes a fetched standalone plugin repo under a temp dir: candy/<name>/charly.yml
// plus an optional schema/x.cue (the schema read must resolve through SourceRoot).
func writePluginRepo(t *testing.T, base, name, version string, withSchema bool) string {
	t.Helper()
	dir := filepath.Join(base, name)
	writeCandyAt(t, dir, "candy/"+name, pluginCandyManifest(name, version))
	if withSchema {
		schema := filepath.Join(dir, "candy", name, "schema")
		if err := os.MkdirAll(schema, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(schema, "alpha.cue"), []byte("// fixture schema\n#Alpha: close({})\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// fakeRepoResolver serves fixture repo trees for any (repoPath, ref) and records every ref
// resolved through the DownloadRepo seam, plus every call of the latest-tag seam.
type fakeRepoResolver struct {
	repos      map[string]string   // repoPath -> fetched fixture dir
	calls      map[string][]string // repoPath -> resolved refs
	latest     map[string]string   // repoPath -> tag the latest-tag seam reports
	latestSeen map[string]bool
}

func (f *fakeRepoResolver) download(repoPath, version string) (string, error) {
	if f.calls == nil {
		f.calls = make(map[string][]string)
	}
	f.calls[repoPath] = append(f.calls[repoPath], version)
	dir, ok := f.repos[repoPath]
	if !ok {
		return "", fmt.Errorf("fake resolver: no fixture for %s", repoPath)
	}
	return dir, nil
}

func (f *fakeRepoResolver) latestTag(repoPath string) (string, error) {
	if f.latestSeen == nil {
		f.latestSeen = make(map[string]bool)
	}
	f.latestSeen[repoPath] = true
	tag, ok := f.latest[repoPath]
	if !ok {
		return "", fmt.Errorf("fake resolver: no latest tag for %s", repoPath)
	}
	return tag, nil
}

func (f *fakeRepoResolver) refs(repoPath string) []string {
	return append([]string(nil), f.calls[repoPath]...)
}

// TestReadDocsConfigRoundTrip decodes a minimal docs: node from a fixture charly.yml with the
// GENERATED spec.DocsConfig types and asserts the resolved siteConfig: CUE defaults, explicit
// overrides, the presence-probed enabled toggle, and the all-defaults behavior of an ABSENT node.
func TestReadDocsConfigRoundTrip(t *testing.T) {
	// Block-style docs: node: sources -> release/extra lists + an explicit compiled override.
	root := t.TempDir()
	writeRoot(t, root,
		"docs:"+"\n"+
			"    docs:"+"\n"+
			"        sources:"+"\n"+
			"            release_repos:"+"\n"+
			"                - plugin-review"+"\n"+
			"                - plugin-pipeline"+"\n"+
			"            extra_repos:"+"\n"+
			"                - plugin-gh"+"\n"+
			"            compiled:"+"\n"+
			"                enabled: false"+"\n"+
			"                compiled_plugins_path: custom/charly.yml"+"\n"+
			"                go_mod_path: custom/go.mod"+"\n",
		"", "", "none")
	cfg, err := readDocsConfig(root)
	if err != nil {
		t.Fatalf("readDocsConfig: %v", err)
	}
	if cfg.compiledEnabled {
		t.Error("compiledEnabled = true, want false (explicit sources.compiled.enabled: false)")
	}
	if cfg.compiledPluginsPath != "custom/charly.yml" {
		t.Errorf("compiledPluginsPath = %q, want custom/charly.yml", cfg.compiledPluginsPath)
	}
	if cfg.goModPath != "custom/go.mod" {
		t.Errorf("goModPath = %q, want custom/go.mod", cfg.goModPath)
	}
	if got := strings.Join(cfg.releaseRepos, ","); got != "plugin-review,plugin-pipeline" {
		t.Errorf("releaseRepos = %q, want the config list", got)
	}
	if got := strings.Join(cfg.extraRepos, ","); got != "plugin-gh" {
		t.Errorf("extraRepos = %q, want the config list", got)
	}

	// The same node, name-first under a DIFFERENT name, with the DEFAULT paths and an absent
	// enabled toggle (the schema default *true wins).
	root2 := t.TempDir()
	writeRoot(t, root2,
		"site-docs:"+"\n"+
			"    docs:"+"\n"+
			"        sources:"+"\n"+
			"            release_repos:"+"\n"+
			"                - plugin-lane"+"\n",
		"", "", "none")
	cfg2, err := readDocsConfig(root2)
	if err != nil {
		t.Fatalf("readDocsConfig: %v", err)
	}
	if !cfg2.compiledEnabled {
		t.Error("compiledEnabled = false, want the CUE default true")
	}
	if cfg2.compiledPluginsPath != "charly/charly.yml" || cfg2.goModPath != "charly/charly/go.mod" {
		t.Errorf("defaults not applied: %+v", cfg2)
	}
	if got := strings.Join(cfg2.releaseRepos, ","); got != "plugin-lane" {
		t.Errorf("releaseRepos = %q, want plugin-lane", got)
	}

	// An ABSENT docs: node is all schema defaults - the pre-config generator behavior.
	root3 := t.TempDir()
	writeRoot(t, root3, "version: 2026.250.0001"+"\n"+"discover:"+"\n"+"    - path: candy"+"\n"+"      recursive: true"+"\n", "", "", "none")
	cfg3, err := readDocsConfig(root3)
	if err != nil {
		t.Fatalf("readDocsConfig: %v", err)
	}
	if !reflect.DeepEqual(cfg3, defaultSiteConfig()) {
		t.Errorf("absent docs: node = %+v, want all defaults", cfg3)
	}
}

// TestAssembleCatalogUnionsCompiledReleaseAndExtra proves the ONE assembly: the unioned entity
// set contains the local walk, the compiled corpus (fetched at the MAPPED v2026.DDD.CCCC refs
// through the DownloadRepo seam), the go.mod-pinned release repo, and the latest-tag release/
// extra repos - with placement and schemas resolved from the same set. No network: both seams
// are fake.
func TestAssembleCatalogUnionsCompiledReleaseAndExtra(t *testing.T) {
	base := t.TempDir()
	fake := &fakeRepoResolver{repos: map[string]string{}}
	fake.repos["github.com/opencharly/plugin-a"] = writePluginRepo(t, base, "plugin-a", "2026.250.0005", true)
	fake.repos["github.com/opencharly/plugin-b"] = writePluginRepo(t, base, "plugin-b", "2026.237.1413", false)
	fake.repos["github.com/opencharly/plugin-c"] = writePluginRepo(t, base, "plugin-c", "2026.251.0500", false)
	fake.repos["github.com/opencharly/plugin-d"] = writePluginRepo(t, base, "plugin-d", "2026.250.9999", false)
	fake.repos["github.com/opencharly/plugin-e"] = writePluginRepo(t, base, "plugin-e", "2026.250.9999", false)
	fake.latest = map[string]string{
		"github.com/opencharly/plugin-d": "v2026.250.9999",
		"github.com/opencharly/plugin-e": "v2026.250.9999",
	}

	root := filepath.Join(base, "root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	// Local walk seed: one local candy + one skill entity (the skill projection consumes the
	// same raw set).
	writeCandyAt(t, root, "candy/local-keeper", "local-keeper:"+"\n"+"    candy:"+"\n"+"        version: 2026.200.1000"+"\n"+"        description: Local fixture candy."+"\n")
	writeCandyAt(t, root, "candy/local-skill", "local-skill:"+"\n"+"    skill:"+"\n"+"        family: fixture"+"\n"+"        name: local"+"\n"+"        description: Local skill."+"\n"+"        content: A local skill body."+"\n")
	// The docs: node drives the union: plugin-d is a RELEASE repo (go.mod-pinned), plugin-e an
	// EXTRA repo (latest tag), plugin-c a release repo WITHOUT a pin of record (latest tag).
	writeRoot(t, root,
		"docs:"+"\n"+
			"    docs:"+"\n"+
			"        sources:"+"\n"+
			"            release_repos:"+"\n"+
			"                - plugin-c"+"\n"+
			"                - plugin-d"+"\n"+
			"            extra_repos:"+"\n"+
			"                - plugin-e"+"\n",
		"compiled_plugins:"+"\n"+"    - plugin-a"+"\n"+"    - plugin-b"+"\n",
		"module github.com/opencharly/fixture"+"\n"+"\n"+"require ("+"\n"+
			"\tgithub.com/opencharly/plugin-a/candy/plugin-a v0.2026250.5"+"\n"+
			"\tgithub.com/opencharly/plugin-b/candy/plugin-b v0.2026237.1413"+"\n"+
			"\tgithub.com/opencharly/plugin-c/candy/plugin-c v0.2026251.500"+"\n"+
			")"+"\n",
		"none")

	cfg, err := readDocsConfig(root)
	if err != nil {
		t.Fatalf("readDocsConfig: %v", err)
	}
	if !cfg.compiledEnabled {
		t.Fatal("compiledEnabled = false, want default true")
	}
	raw, compiled, err := assembleCatalog(root, cfg, repoResolver{download: fake.download, latestTag: fake.latestTag})
	if err != nil {
		t.Fatalf("assembleCatalog: %v", err)
	}

	// The compiled corpus is fetched at the MAPPED repo tags through the DownloadRepo seam: the
	// go.mod pin v0.2026250.5 (unpadded counter) resolves as v2026.250.0005.
	if got := strings.Join(fake.refs("github.com/opencharly/plugin-a"), ","); got != "v2026.250.0005" {
		t.Errorf("plugin-a resolved at %q, want the mapped tag v2026.250.0005 (pin v0.2026250.5)", got)
	}
	if got := strings.Join(fake.refs("github.com/opencharly/plugin-b"), ","); got != "v2026.237.1413" {
		t.Errorf("plugin-b resolved at %q, want v2026.237.1413", got)
	}
	// plugin-c: release repo pinned by the compiled corpus go.mod require -> mapped tag.
	if got := strings.Join(fake.refs("github.com/opencharly/plugin-c"), ","); got != "v2026.251.0500" {
		t.Errorf("plugin-c resolved at %q, want the go.mod-derived v2026.251.0500", got)
	}
	// plugin-d (release, no pin) and plugin-e (extra) resolve their LATEST TAG at generation time.
	if got := strings.Join(fake.refs("github.com/opencharly/plugin-d"), ","); got != "v2026.250.9999" {
		t.Errorf("plugin-d resolved at %q, want the latest-tag result", got)
	}
	if got := strings.Join(fake.refs("github.com/opencharly/plugin-e"), ","); got != "v2026.250.9999" {
		t.Errorf("plugin-e resolved at %q, want the latest-tag result", got)
	}
	if !fake.latestSeen["github.com/opencharly/plugin-d"] || !fake.latestSeen["github.com/opencharly/plugin-e"] {
		t.Error("latest-tag seam not consulted for the unrequired release/extra repos")
	}
	if fake.latestSeen["github.com/opencharly/plugin-c"] {
		t.Error("latest-tag seam consulted for plugin-c, which the go.mod pins")
	}

	// The unioned entity set: local walk + compiled corpus + release + extra, each fetched root
	// under its repoPath:ref namespace (SourceRoot = the fetched dir).
	names := map[string]string{}
	for _, e := range raw {
		if e.Namespace == "" { // local walk: the superproject + box submodules
			names["local:"+e.Name] = "local"
		} else {
			names[e.Namespace+"/"+e.Name] = e.SourceRoot
		}
	}
	for _, want := range []string{
		"local:local-keeper",
		"local:local-skill",
		"github.com/opencharly/plugin-a:v2026.250.0005/plugin-a",
		"github.com/opencharly/plugin-b:v2026.237.1413/plugin-b",
		"github.com/opencharly/plugin-c:v2026.251.0500/plugin-c",
		"github.com/opencharly/plugin-d:v2026.250.9999/plugin-d",
		"github.com/opencharly/plugin-e:v2026.250.9999/plugin-e",
	} {
		if _, ok := names[want]; !ok {
			t.Errorf("entity %q not in the unioned set; got: %v", want, names)
		}
	}
	for seg, sr := range names {
		if strings.HasPrefix(seg, "github.com/") && sr == "" {
			t.Errorf("remote entity %q has an empty SourceRoot", seg)
		}
	}

	// Placement membership comes back from the SAME assembly: exactly plugin-a + plugin-b.
	if !compiled["plugin-a"] || !compiled["plugin-b"] {
		t.Errorf("compiled map missing the corpus members: %v", compiled)
	}
	if compiled["plugin-c"] || compiled["plugin-d"] || compiled["plugin-e"] {
		t.Errorf("compiled map leaks non-compiled names: %v", compiled)
	}

	// collectPlugins consumes the unioned set: schema for a compiled plugin reads through
	// SourceRoot, and CompiledIn comes from the assembly-returned membership.
	entities, err := collectEntitiesFrom(raw)
	if err != nil {
		t.Fatalf("collectEntitiesFrom: %v", err)
	}
	plugins, err := collectPlugins(root, entities, compiled)
	if err != nil {
		t.Fatalf("collectPlugins: %v", err)
	}
	var pa *pluginEntity
	for i := range plugins {
		if plugins[i].Name == "plugin-a" {
			pa = &plugins[i]
		}
	}
	if pa == nil {
		t.Fatal("compiled plugin-a missing from the plugin set")
	}
	if !pa.CompiledIn {
		t.Error("plugin-a CompiledIn = false, want true (it is in compiled_plugins)")
	}
	if len(pa.Schemas) != 1 || pa.Schemas[0].Name != "alpha.cue" {
		t.Errorf("plugin-a schemas = %+v, want alpha.cue read through SourceRoot", pa.Schemas)
	}
	for _, p := range plugins {
		if p.Name == "plugin-d" && p.CompiledIn {
			t.Error("plugin-d CompiledIn = true, want false (release repo, not compiled)")
		}
		if p.Name == "plugin-c" || p.Name == "plugin-e" {
			if _, ok := compiled[p.Name]; ok {
				t.Errorf("plugin-%s leaked into the compiled placement map", strings.TrimPrefix(p.Name, "plugin-"))
			}
		}
	}
}

// TestAssembleCatalogPinsReleaseRepoWithCompiledDisabled proves the go.mod require pins are
// honored for release repos even when the compiled corpus is DISABLED: the version map must be
// populated for the release loop (a nil map would silently fall every repo back to its latest
// tag - the compiled-enabled union test cannot see this path). The disabled corpus is never
// read (no compiled_plugins manifest), the pinned repo resolves at the go.mod-derived tag, and
// the latest-tag seam is never consulted.
func TestAssembleCatalogPinsReleaseRepoWithCompiledDisabled(t *testing.T) {
	base := t.TempDir()
	fake := &fakeRepoResolver{repos: map[string]string{}, latest: map[string]string{}}
	fake.repos["github.com/opencharly/plugin-x"] = writePluginRepo(t, base, "plugin-x", "2026.253.0042", false)
	fake.latest["github.com/opencharly/plugin-x"] = "v2026.253.9999"

	root := filepath.Join(base, "root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writeCandyAt(t, root, "candy/local-keeper", strings.Join([]string{
		"local-keeper:",
		"    candy:",
		"        version: 2026.200.1000",
		"        description: Local fixture candy.",
	}, "\n"))
	// docs: node with the compiled corpus DISABLED and one go.mod-pinned release repo. No
	// compiled_plugins manifest is written: the disabled corpus must never be read.
	writeRoot(t, root,
		strings.Join([]string{
			"docs:",
			"    docs:",
			"        sources:",
			"            compiled:",
			"                enabled: false",
			"            release_repos:",
			"                - plugin-x",
		}, "\n"),
		"",
		strings.Join([]string{
			"module github.com/opencharly/fixture",
			"",
			"require (",
			"\tgithub.com/opencharly/plugin-x/candy/plugin-x v0.2026253.42",
			")",
		}, "\n"),
		"none")

	cfg, err := readDocsConfig(root)
	if err != nil {
		t.Fatalf("readDocsConfig: %v", err)
	}
	if cfg.compiledEnabled {
		t.Fatal("compiledEnabled = true, want false (explicit sources.compiled.enabled: false)")
	}
	raw, compiled, err := assembleCatalog(root, cfg, repoResolver{download: fake.download, latestTag: fake.latestTag})
	if err != nil {
		t.Fatalf("assembleCatalog: %v", err)
	}

	// plugin-x resolves at the go.mod require pin mapped to the repo tag - NOT its latest tag.
	// Without the version-map population this resolves v2026.253.9999 and fails both asserts.
	if got := strings.Join(fake.refs("github.com/opencharly/plugin-x"), ","); got != "v2026.253.0042" {
		t.Errorf("plugin-x resolved at %q, want the go.mod-derived v2026.253.0042 (pin v0.2026253.42)", got)
	}
	if fake.latestSeen["github.com/opencharly/plugin-x"] {
		t.Error("latest-tag seam consulted for plugin-x, which the go.mod pins")
	}
	if len(compiled) != 0 {
		t.Errorf("compiled placement map = %v, want empty with the corpus disabled", compiled)
	}
	names := map[string]string{}
	for _, e := range raw {
		if e.Namespace == "" { // local walk
			names["local:"+e.Name] = "local"
		} else {
			names[e.Namespace+"/"+e.Name] = e.SourceRoot
		}
	}
	if _, ok := names["github.com/opencharly/plugin-x:v2026.253.0042/plugin-x"]; !ok {
		t.Errorf("plugin-x entity absent from the unioned set; got: %v", names)
	}
}

// TestAssembleCatalogDedupesClosureSurfacing proves the union does not double-emit: a repo its
// OWN fixture references by @github ref at the SAME ref the compiled pin maps to surfaces once.
func TestAssembleCatalogDedupesClosureSurfacing(t *testing.T) {
	base := t.TempDir()
	fake := &fakeRepoResolver{repos: map[string]string{}, latest: map[string]string{}}
	fake.repos["github.com/opencharly/plugin-a"] = writePluginRepo(t, base, "plugin-a", "2026.250.0005", true)

	root := filepath.Join(base, "root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	// The local candy REFERENCES the moved plugin at the very ref the compiled pin maps to - the
	// closure walk will fetch it too.
	writeCandyAt(t, root, "candy/local-keeper", "local-keeper:"+"\n"+"    candy:"+"\n"+"        version: 2026.200.1000"+"\n"+"        description: Local fixture candy."+"\n"+"        candy:"+"\n"+"            - @github.com/opencharly/plugin-a:v2026.250.0005"+"\n")
	writeRoot(t, root,
		"docs:"+"\n"+"    docs:"+"\n",
		"compiled_plugins:"+"\n"+"    - plugin-a"+"\n",
		"module github.com/opencharly/fixture"+"\n"+"\n"+"require ("+"\n"+"\tgithub.com/opencharly/plugin-a/candy/plugin-a v0.2026250.5"+"\n"+")"+"\n",
		"none")

	cfg, err := readDocsConfig(root)
	if err != nil {
		t.Fatalf("readDocsConfig: %v", err)
	}
	raw, _, err := assembleCatalog(root, cfg, repoResolver{download: fake.download, latestTag: fake.latestTag})
	if err != nil {
		t.Fatalf("assembleCatalog: %v", err)
	}
	count := 0
	for _, e := range raw {
		if e.Name == "plugin-a" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("plugin-a surfaced %d times, want exactly 1 (config fetch + closure ref deduped)", count)
	}
}

// TestAssembleCatalogMissingManifestIsEmptyCorpus locks the minimal-root contract: a project
// root with NO compiled corpus manifest and NO go.mod at the default paths (the standalone
// plugin-docs repo layout - no charly/ submodule) assembles the EMPTY compiled set at the
// schema defaults, and the lazily-read go.mod is never required because an empty corpus needs
// no pins. This is the pre-config generator behavior: generation on such a root is valid,
// never an error, and the local walk still runs.
func TestAssembleCatalogMissingManifestIsEmptyCorpus(t *testing.T) {
	root := t.TempDir()
	writeRoot(t, root,
		"version: 2026.250.0001\ndiscover:\n    - path: candy\n      recursive: true\n",
		"", "", // no compiled corpus manifest, no go.mod - the minimal root
		"candy/local-keeper")
	writeCandyAt(t, root, "candy/local-keeper", "local-keeper:\n    candy:\n        version: 2026.200.1000\n        description: Local fixture candy.\n")

	cfg := defaultSiteConfig()
	fetches := 0
	raw, compiled, err := assembleCatalog(root, cfg, repoResolver{
		download: func(repoPath, version string) (string, error) {
			fetches++
			return "", fmt.Errorf("unexpected fetch of %s@%s", repoPath, version)
		},
		latestTag: nil, // never consulted: an empty corpus means no release/extra repos either
	})
	if err != nil {
		t.Fatalf("assembleCatalog: %v", err)
	}
	if fetches != 0 {
		t.Errorf("%d fetches, want 0 (no compiled plugins, no release/extra repos)", fetches)
	}
	if len(compiled) != 0 {
		t.Errorf("compiled placement map = %v, want empty with no manifest", compiled)
	}
	found := false
	for _, e := range raw {
		if e.Name == "local-keeper" {
			found = true
		}
	}
	if !found {
		t.Errorf("local walk did not surface local-keeper (got %d entities)", len(raw))
	}
}

// TestAssembleCatalogMalformedCompiledManifestErrors locks the PRESENCE discriminator: a
// manifest that is present but malformed is a real configuration error and must NOT be
// swallowed as the empty corpus - only an ABSENT file reads as empty.
func TestAssembleCatalogMalformedCompiledManifestErrors(t *testing.T) {
	root := t.TempDir()
	writeRoot(t, root,
		"version: 2026.250.0001\n",
		"compiled_plugins: [", // present, malformed (unclosed flow sequence)
		"",
		"none")
	_, _, err := assembleCatalog(root, defaultSiteConfig(), repoResolver{
		download: func(repoPath, version string) (string, error) {
			return "", fmt.Errorf("unexpected fetch of %s@%s", repoPath, version)
		},
		latestTag: nil,
	})
	if err == nil {
		t.Fatal("assembleCatalog: expected an error for a malformed manifest, got nil")
	}
	if !strings.Contains(err.Error(), "compiled corpus manifest") {
		t.Errorf("error should name the compiled corpus manifest, got: %v", err)
	}
}

// TestGenerateRegenNoOp runs the FULL generator twice over the same fixture root and asserts a
// byte-identical tree - regeneration on a clean tree is a no-op. The fixture root carries a
// docs: node with the compiled corpus DISABLED and no release/extra repos, so the run performs
// zero fetches: no network, no checkout.
func TestGenerateRegenNoOp(t *testing.T) {
	root := t.TempDir()
	base := t.TempDir()

	writeRoot(t, root,
		"docs:"+"\n"+"    docs:"+"\n"+"        sources:"+"\n"+"            compiled:"+"\n"+"                enabled: false"+"\n",
		"compiled_plugins: []"+"\n",
		"",
		"candy/local-candy")
	writeCandyAt(t, root, "candy/local-candy", "local-candy:"+"\n"+"    candy:"+"\n"+"        version: 2026.250.0001"+"\n"+"        description: The fixture candy of the determinism test."+"\n"+"        plugin:"+"\n"+"            source: github.com/opencharly/plugin-docs/candy/plugin-docs"+"\n"+"            providers:"+"\n"+"                - verb:alpha"+"\n"+"                - command:fixture"+"\n")
	// The root narrative the landing/vision/grievances/liberation passes project.
	writeRoot(t, root, "", "", "", "none")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# Fixture Root"+"\n"+"\n"+"**A fixture root.**"+"\n"+"\n"+"The fixture superproject of the regeneration no-op test."+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "VISION.md"), []byte("# The Vision"+"\n"+"\n"+"The fixture vision."+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "GRIEVANCES.md"), []byte("# Grievances"+"\n"+"\n"+"The fixture grievances."+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "LIBERATION.md"), []byte("# Liberation"+"\n"+"\n"+"The fixture manifesto."+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The marketplace corpus: one plugin with one skill card.
	pluginsDir := filepath.Join(base, "marketplace")
	if err := os.MkdirAll(filepath.Join(pluginsDir, ".claude-plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginsDir, ".claude-plugin", "marketplace.json"), []byte("{\"plugins\": [{\"name\": \"charly-fixture\", \"source\": \"./fixture\", \"description\": \"The fixture plugin corpus.\", \"category\": \"kind\"}]}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(pluginsDir, "fixture", "skills", "install"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginsDir, "fixture", "skills", "install", "SKILL.md"), []byte("# Fixture skill"+"\n"+"\n"+"A fixture skill body with no cross-references."+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	run := func(out string) {
		t.Helper()
		if err := os.MkdirAll(out, 0o755); err != nil {
			t.Fatal(err)
		}
		// The sidebar gate needs the Astro config ABOVE the content root.
		if err := os.WriteFile(filepath.Join(base, "astro.config.mjs"), []byte(astroConfig()), 0o644); err != nil {
			t.Fatal(err)
		}
		// The hero actions link to /start/install/ and /start/quickstart/ - hand-authored pages
		// the generator does not own but the site-link gate resolves.
		for _, p := range []string{"start/install.md", "start/quickstart.md"} {
			dst := filepath.Join(out, p)
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dst, []byte("# "+p+"\n"+"\n"+"Hand-authored."+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if err := generate(root, out, pluginsDir); err != nil {
			t.Fatalf("generate: %v", err)
		}
	}

	out1 := filepath.Join(base, "run1")
	out2 := filepath.Join(base, "run2")
	run(out1)
	run(out2)

	var diffs []string
	_ = filepath.WalkDir(out1, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(out1, path)
		twin := filepath.Join(out2, rel)
		a, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		b, err := os.ReadFile(twin)
		if err != nil {
			diffs = append(diffs, "run2 missing "+rel)
			return nil
		}
		if string(a) != string(b) {
			diffs = append(diffs, "differ: "+rel)
		}
		return nil
	})
	_ = filepath.WalkDir(out2, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(out2, path)
		if _, err := os.Stat(filepath.Join(out1, rel)); err != nil {
			diffs = append(diffs, "run1 missing "+rel)
		}
		return nil
	})
	if len(diffs) > 0 {
		t.Errorf("regeneration is not a no-op:\n  %s", strings.Join(diffs, "\n  "))
	}
}
