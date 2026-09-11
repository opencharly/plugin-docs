package docs

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/opencharly/sdk/candywalk"
	"github.com/opencharly/spec/refs"
	"github.com/opencharly/spec/spec"
	"gopkg.in/yaml.v3"
)

// docsKind is the kind discriminator of the docs: generation config node (the closed
// #DocsConfig contract in spec/schema/docs.cue - the ONE docs-site generation config entity
// per repo, discovered by the same name-first walk as skill:/hook:/marketplace: entities).
const docsKind = "docs"

// siteConfig is the docs: node RESOLVED generation config: spec.DocsConfig decoded from
// <root>/charly.yml with the CUE defaults applied. It is the single config source for the
// catalog assembly - where the compiled corpus is declared, where its go.mod pins live, and
// which release/extra repos to document. Every path and toggle comes from the config (or its
// schema defaults); no generation knob is hardcoded.
type siteConfig struct {
	// compiledEnabled gates the compiled-in plugin corpus assembly (spec default: true).
	compiledEnabled bool
	// compiledPluginsPath is the charly.yml whose compiled_plugins: list declares the
	// compiled-in corpus (spec default: "charly/charly.yml" - the charly repo own
	// charly.yml at the umbrella root).
	compiledPluginsPath string
	// goModPath is the go.mod whose require: pins the compiled corpus modules and the
	// release repos (spec default: "charly/charly/go.mod").
	goModPath string
	// releaseRepos / extraRepos are bare candy repo names (plugin-review, plugin-pipeline,
	// plugin-gh, ...) documented outside the compiled corpus. Release repos resolve like a
	// go.mod require: entry - from the compiled corpus go.mod when present, else the repo
	// latest CalVer tag at generation time. Extra repos skip the go.mod check entirely.
	releaseRepos []string
	extraRepos   []string
}

// defaultSiteConfig returns siteConfig at the schema CUE defaults - what an ABSENT docs:
// node means (every #DocsConfig field carries a default, so no node is just all-defaults).
// An absent node therefore behaves EXACTLY like the pre-config generator: the compiled corpus
// is read from charly/charly.yml + charly/charly/go.mod and no release/extra repos exist.
func defaultSiteConfig() siteConfig {
	return siteConfig{
		compiledEnabled:     true,
		compiledPluginsPath: "charly/charly.yml",
		goModPath:           "charly/charly/go.mod",
	}
}

// readDocsConfig reads and decodes the docs: node from <root>/charly.yml into the generated
// spec.DocsConfig types, and resolves the CUE defaults onto a siteConfig. The node is
// name-first (<name>: {docs: <body>}), exactly as the shared walk discovers it; the node
// NAME is irrelevant, the kind discriminator is what matters, and more than one docs: node is
// a configuration error (the schema contract: ONE docs-site generation config per repo).
//
// The generated types drop CUE defaults (every bool field is omitempty), so a *true | bool
// field decodes false whether absent or explicitly false. Presence of the compiled corpus
// sources.compiled.enabled toggle is therefore read off the RAW yaml node before the decode;
// everything else is default-by-zero-value.
func readDocsConfig(root string) (siteConfig, error) {
	raw, err := os.ReadFile(filepath.Join(root, unifiedFileName))
	if err != nil {
		return siteConfig{}, fmt.Errorf("read %s: %w", filepath.Join(root, unifiedFileName), err)
	}
	var doc map[string]yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return siteConfig{}, fmt.Errorf("parse %s: %w", filepath.Join(root, unifiedFileName), err)
	}

	var d spec.DocsConfig
	found := 0
	for _, node := range doc { // name-first: entity name -> kind discriminator map
		if node.Kind != yaml.MappingNode {
			continue // the prepended version: header / directives are not entities
		}
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value != docsKind {
				continue
			}
			found++
			if err := node.Content[i+1].Decode(&d); err != nil {
				return siteConfig{}, fmt.Errorf("decode docs: node: %w", err)
			}
		}
	}
	if found == 0 {
		return defaultSiteConfig(), nil
	}
	if found > 1 {
		return siteConfig{}, fmt.Errorf("%s declares %d docs: nodes; the schema contract is ONE docs-site generation config per repo", filepath.Join(root, unifiedFileName), found)
	}

	cfg := siteConfig{
		compiledEnabled:     true, // CUE default *true
		compiledPluginsPath: "charly/charly.yml",
		goModPath:           "charly/charly/go.mod",
		releaseRepos:        append([]string(nil), d.Sources.ReleaseRepos...),
		extraRepos:          append([]string(nil), d.Sources.ExtraRepos...),
	}
	if v := d.Sources.Compiled.CompiledPluginsPath; v != "" {
		cfg.compiledPluginsPath = v
	}
	if v := d.Sources.Compiled.GoModPath; v != "" {
		cfg.goModPath = v
	}
	// The enabled toggle CUE default is *true, which the omitempty bool cannot represent -
	// probe the raw node for an EXPLICIT value.
	if n := yamlChild(nodeByKind(doc, docsKind), "sources", "compiled", "enabled"); n != nil {
		var enabled bool
		if err := n.Decode(&enabled); err != nil {
			return siteConfig{}, fmt.Errorf("decode docs: sources.compiled.enabled: %w", err)
		}
		cfg.compiledEnabled = enabled
	}
	return cfg, nil
}

// nodeByKind returns the value body of the FIRST top-level node carrying the given kind
// discriminator ("" when absent). readDocsConfig guarantees exactly one such node, so the
// FIRST is THE node.
func nodeByKind(doc map[string]yaml.Node, kind string) *yaml.Node {
	for _, node := range doc {
		if node.Kind != yaml.MappingNode {
			continue
		}
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == kind {
				return node.Content[i+1]
			}
		}
	}
	return nil
}

// yamlChild walks nested mapping keys from a yaml node, returning the value node for the key
// path or nil when any level is absent or not a mapping.
func yamlChild(n *yaml.Node, keys ...string) *yaml.Node {
	for _, k := range keys {
		if n == nil || n.Kind != yaml.MappingNode {
			return nil
		}
		next := (*yaml.Node)(nil)
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Value == k {
				next = n.Content[i+1]
				break
			}
		}
		n = next
	}
	return n
}

// compiledPluginRef maps a go.mod require version for a compiled plugin module - the Go-tag
// form v0.YYYYDDD.C (github.com/opencharly/<name>/candy/<name> pins) - onto the repo CALVER
// TAG, v2026.DDD.CCCC. The two formats differ in the major-version prefix and the counter
// width: Go canonicalizes the numeric counter to its minimal form (a release the repo tags
// v2026.254.0005 is pinned in go.mod as v0.2026254.5), while the repo tags publish it
// zero-padded to four digits. The padding is therefore RE-DERIVED here, never trusted from the
// input, so a padded and an unpadded pin of the same release converge on the same tag.
func compiledPluginRef(v string) (string, error) {
	rest, ok := strings.CutPrefix(v, "v0.")
	if !ok {
		return "", fmt.Errorf("version %q is not a v0.YYYYDDD.C Go tag", v)
	}
	cal, cstr, ok := strings.Cut(rest, ".")
	if !ok {
		return "", fmt.Errorf("version %q is not a v0.YYYYDDD.C Go tag", v)
	}
	if len(cal) != 7 {
		return "", fmt.Errorf("version %q: %q is not a 7-digit YYYYDDD CalVer day", v, cal)
	}
	year, doy := cal[:4], cal[4:]
	if _, err := strconv.Atoi(year); err != nil {
		return "", fmt.Errorf("version %q: %q is not a numeric year", v, year)
	}
	if _, err := strconv.Atoi(doy); err != nil {
		return "", fmt.Errorf("version %q: %q is not a numeric day-of-year", v, doy)
	}
	c, err := strconv.Atoi(cstr)
	if err != nil {
		return "", fmt.Errorf("version %q: %q is not a numeric counter", v, cstr)
	}
	if c < 0 || c > 9999 {
		return "", fmt.Errorf("version %q: counter %d out of the 4-digit 0-9999 range", v, c)
	}
	return fmt.Sprintf("v%s.%s.%04d", year, doy, c), nil
}

// readCompiledPluginNames reads ONLY the compiled_plugins: list from a charly.yml that
// declares the compiled-in corpus (the same one-field decode pluginsgen uses - one canonical
// read of the list, R3).
func readCompiledPluginNames(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read compiled corpus manifest %s: %w", path, err)
	}
	var doc struct {
		CompiledPlugins []string "yaml:\"compiled_plugins\""
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse compiled corpus manifest %s: %w", path, err)
	}
	var out []string
	for _, n := range doc.CompiledPlugins {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	return out, nil
}

// readGoModVersions reads every github.com/opencharly/... require: pin from a go.mod - the
// module-path -> version map the compiled corpus and the release-repo resolution share.
func readGoModVersions(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read go.mod %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	versions := make(map[string]string)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		if strings.HasPrefix(fields[0], "github.com/opencharly/") {
			versions[fields[0]] = fields[1]
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan go.mod %s: %w", path, err)
	}
	return versions, nil
}

// requiredModuleVersion returns the require: pin for a bare repo name when the compiled
// corpus go.mod requires a module of it (the module paths live under the repo:
// github.com/opencharly/<repo>/candy/<repo>). Keys are scanned in sorted order so the match
// is deterministic when one repo carries several modules.
func requiredModuleVersion(versions map[string]string, repo string) (string, bool) {
	prefix := "github.com/opencharly/" + repo + "/"
	mods := make([]string, 0, len(versions))
	for mod := range versions {
		mods = append(mods, mod)
	}
	sort.Strings(mods)
	for _, mod := range mods {
		if strings.HasPrefix(mod, prefix) {
			return versions[mod], true
		}
	}
	return "", false
}

// repoResolver is the pair of fetch seams the catalog assembly needs: the DownloadRepo seam
// candywalk remote walk already uses, plus the newest-CalVer-tag resolution for repos the
// go.mod does not require. Both are injectable so the assembly runs on fixtures without
// network; generate() supplies refs.DownloadRepo + refs.GitLatestTag.
type repoResolver struct {
	download  func(repoPath, version string) (string, error)
	latestTag func(repoPath string) (string, error)
}

// defaultRepoResolver is the production seam pair: the SAME standalone fetch the runtime and
// the closure walk use (refs.DownloadRepo), plus the raw newest-tag primitive.
func defaultRepoResolver() repoResolver {
	return repoResolver{
		download: refs.DownloadRepo,
		latestTag: func(repoPath string) (string, error) {
			return refs.GitLatestTag(refs.RepoGitURL(repoPath))
		},
	}
}

// assembleCatalog is the generator ONE catalog assembly (R5). It unions
//
//   - the discovered/@github local closure - CollectEntitiesRemote over the superproject plus
//     every box/<distro> submodule (the pre-existing remote-aware walk, kept as-is),
//   - the COMPILED corpus - the compiled_plugins list from <root>/<compiled.plugins.path>,
//     each name pinned by <root>/<go.mod path> (module github.com/opencharly/<name>/candy/<name>),
//     its v0.YYYYDDD.C pin mapped to the repo tag v2026.DDD.CCCC and the fetched repo
//     candy/<name> walked as a remote entity (SourceRoot = the fetched dir),
//   - the RELEASE and EXTRA repos from the docs: config - each resolved via the same
//     DownloadRepo seam, at the go.mod require pin when the compiled corpus go.mod requires
//     the repo, else its latest CalVer tag resolved at generation time; their candies are
//     walked the same way.
//
// The compiled membership map (name -> true) is returned alongside, so placement is computed
// from the SAME list the assembly feeds on - one source, never transcribed.
func assembleCatalog(root string, cfg siteConfig, r repoResolver) ([]candywalk.Entity, map[string]bool, error) {
	roots := make([]candywalk.Root, 0)
	local, err := repoRoots(root)
	if err != nil {
		return nil, nil, err
	}
	for _, lr := range local {
		roots = append(roots, candywalk.Root{Namespace: lr.Namespace, Dir: lr.Dir})
	}
	compiled := make(map[string]bool)

	// The go.mod is read lazily: only when a compiled name or a release repo actually needs a
	// pin. An EMPTY compiled corpus with no release/extra repos therefore never requires the
	// go.mod to exist - the pre-config behavior (charly/charly.yml alone) is preserved.
	var versions map[string]string
	ensureVersions := func() error {
		if versions != nil {
			return nil
		}
		v, err := readGoModVersions(filepath.Join(root, cfg.goModPath))
		if err != nil {
			return err
		}
		versions = v
		return nil
	}

	// Fetched roots are deduped by repoPath:ref - the same identity key the closure walk uses
	// for its own fetches - so a repo fetched twice at the same ref is walked once.
	fetched := make(map[string]bool)
	appendFetchedRoot := func(repoPath, ref string) error {
		key := repoPath + ":" + ref
		if fetched[key] {
			return nil
		}
		fetched[key] = true
		dir, err := r.download(repoPath, ref)
		if err != nil {
			return fmt.Errorf("fetch %s@%s: %w", repoPath, ref, err)
		}
		roots = append(roots, candywalk.Root{Namespace: key, Dir: dir})
		return nil
	}

	if cfg.compiledEnabled {
		names, err := readCompiledPluginNames(filepath.Join(root, cfg.compiledPluginsPath))
		if err != nil {
			return nil, nil, err
		}
		for _, name := range names {
			compiled[name] = true
			mod := "github.com/opencharly/" + name + "/candy/" + name
			if err := ensureVersions(); err != nil {
				return nil, nil, err
			}
			ver, ok := versions[mod]
			if !ok {
				return nil, nil, fmt.Errorf("compiled plugin %q: no require pin for %s in %s (a compiled plugin must be pinned in the compiled corpus go.mod)", name, mod, cfg.goModPath)
			}
			ref, err := compiledPluginRef(ver)
			if err != nil {
				return nil, nil, fmt.Errorf("compiled plugin %q: %w", name, err)
			}
			if err := appendFetchedRoot("github.com/opencharly/"+name, ref); err != nil {
				return nil, nil, err
			}
		}
	}

	repos := append(append([]string(nil), cfg.releaseRepos...), cfg.extraRepos...)
	seenRepo := make(map[string]bool)
	for _, repo := range repos {
		repo = strings.TrimSpace(repo)
		if repo == "" || seenRepo[repo] {
			continue
		}
		seenRepo[repo] = true
		repoPath := "github.com/opencharly/" + repo

		ref := ""
		if ver, ok := requiredModuleVersion(versions, repo); ok {
			mapped, err := compiledPluginRef(ver)
			if err != nil {
				return nil, nil, fmt.Errorf("release repo %q: %w", repo, err)
			}
			ref = mapped
		} else {
			if r.latestTag == nil {
				return nil, nil, fmt.Errorf("no go.mod require pin for %s and no tag resolver configured", repoPath)
			}
			// Release/extra repos OUTSIDE the compiled corpus have no pin of record - resolve
			// the repo latest CalVer tag at generation time.
			tag, err := r.latestTag(repoPath)
			if err != nil {
				return nil, nil, fmt.Errorf("resolve latest tag for %s: %w", repoPath, err)
			}
			ref = tag
		}
		if err := appendFetchedRoot(repoPath, ref); err != nil {
			return nil, nil, err
		}
	}

	raw, err := candywalk.CollectEntitiesRemote(roots, func(p, v string) (string, error) { return r.download(p, v) })
	if err != nil {
		return nil, nil, err
	}
	return dedupeEntities(raw), compiled, nil
}

// dedupeEntities collapses identical remote entities. The closure walk fetches every
// @github ref it finds, and a config-fetched root is walked directly, so the SAME repo at the
// SAME ref can surface twice (once per seam) and double-emit its page set - deduped here on
// the entity identity (namespace, name, kind, dir, source root), keeping both a compiled pin
// and a diverging closure ref as the two distinct entities they are.
func dedupeEntities(in []candywalk.Entity) []candywalk.Entity {
	seen := make(map[string]bool)
	out := make([]candywalk.Entity, 0, len(in))
	for _, e := range in {
		key := e.Namespace + "\x00" + e.Name + "\x00" + e.Kind + "\x00" + e.Dir + "\x00" + e.SourceRoot
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, e)
	}
	return out
}
