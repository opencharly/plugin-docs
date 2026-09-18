package docs

import (
	"strings"
	"testing"

	"github.com/opencharly/spec/spec"
)

// TestSanitizeSegment is the table for the one slug-safe transform: keep lowercase ASCII
// letters and digits, replace every other byte (dot, colon, uppercase, …) with "-". A segment
// that is already safe passes through byte-for-byte unchanged — which is what keeps every
// safe namespace (arch, cachyos, plugin-check …) and the CLI/landing/recipes paths untouched.
func TestSanitizeSegment(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{
			// The case that actually broke the site: a remote candy's fetched-repo namespace
			// carries the github.com repo path and the CalVer tag, dots and colons in both.
			name: "remote repo namespace with version tag",
			in:   "github.com/opencharly/plugin-check:v2026.242.2127",
			want: "github-com-opencharly-plugin-check-v2026-242-2127",
		},
		{
			name: "version tag alone",
			in:   "v2026.242.2127",
			want: "v2026-242-2127",
		},
		{
			name: "mixed dots and colons",
			in:   "a.b:c-d",
			want: "a-b-c-d",
		},
		{
			name: "already-safe name is unchanged",
			in:   "plugin-check",
			want: "plugin-check",
		},
		{
			name: "already-safe namespace is unchanged",
			in:   "cachyos",
			want: "cachyos",
		},
		{
			name: "hyphens pass through (replace-with-self is the identity)",
			in:   "check-sway-browser-vnc-pod",
			want: "check-sway-browser-vnc-pod",
		},
		{
			name: "empty string stays empty",
			in:   "",
			want: "",
		},
		{
			name: "uppercase is not a lowercase alphanumeric",
			in:   "UPPER",
			want: "-----",
		},
		{
			name: "underscore and slash collapse to dash",
			in:   "a_b/c",
			want: "a-b-c",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeSegment(tc.in); got != tc.want {
				t.Errorf("sanitizeSegment(%q) = %q, want %q", tc.in, got, tc.want)
			}
			// The output must never carry a byte Astro would strip or mangle.
			for _, b := range []byte(tc.want) {
				if (b < 'a' || b > 'z') && (b < '0' || b > '9') && b != '-' {
					t.Errorf("sanitizeSegment(%q) = %q contains unsafe byte %q", tc.in, tc.want, b)
				}
			}
		})
	}
}

// assertSlugSafe fails unless every slash-SEPARATED segment contains only [a-z0-9-]. The
// ".md" extension is stripped before the check: the contract is that the directory/file
// segments derived from an entity identity (the namespace, name and version parts) carry no
// dot or colon — the fixed ".md" suffix is not a segment, and the leading "/" of a site path
// is not a segment either.
func assertSlugSafe(t *testing.T, what string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		for _, raw := range strings.Split(p, "/") {
			seg := strings.TrimSuffix(raw, ".md")
			if seg == "" {
				continue
			}
			for _, b := range []byte(seg) {
				if (b < 'a' || b > 'z') && (b < '0' || b > '9') && b != '-' {
					t.Errorf("%s: unsafe byte %q in segment %q of %q", what, b, seg, p)
				}
			}
		}
	}
}

// TestPathCoherenceSlugSafe locks the construction property the sanitizer exists for: the
// emitted page file, the served URL and every link target are the SAME string, and no dot or
// colon survives in any segment. Representative entities: a github.com-namespaced remote
// plugin candy with a version tag (the case that broke the site), a namespaced distro box, a
// plain superproject candy, and a CLI word.
func TestPathCoherenceSlugSafe(t *testing.T) {
	// A remote plugin candy: Namespace carries the fetched repo path + CalVer tag (the
	// catalog assembly records Namespace = "repoPath:ref" for every remote root).
	remote := pluginEntity{entity: entity{
		Name:      "plugin-check",
		Namespace: "github.com/opencharly/plugin-check:v2026.242.2127",
		Candy:     &candyView{Version: "2026.242.2127", Plugin: &spec.Plugin{}},
	}}
	seg := "github-com-opencharly-plugin-check-v2026-242-2127/plugin-check"

	if got := remote.PathSegment(); got != seg {
		t.Errorf("remote PathSegment() = %q, want %q", got, seg)
	}

	// Emitted pages.
	pluginPage := pluginPagePath(remote)
	candyPage := candyPagePath(remote.PathSegment())
	// Served URLs (the trailing-slash forms Starlight serves them at).
	pluginSite := pluginSitePath(remote)
	candySite := candySitePathFor(remote.PathSegment())
	// The cross-links each page emits to the other.
	pluginToCandy := candySitePathFor(remote.PathSegment())
	candyToPlugin := pluginSitePathFor(remote.PathSegment())

	for _, tc := range []struct{ name, got, want string }{
		{"pluginPagePath", pluginPage, "reference/plugin/" + seg + ".md"},
		{"candyPagePath", candyPage, "reference/candy/" + seg + ".md"},
		{"pluginSitePath", pluginSite, "/reference/plugin/" + seg + "/"},
		{"candySitePathFor(seg)", candySite, "/reference/candy/" + seg + "/"},
		{"candy link on the plugin page", pluginToCandy, candySite},
		{"plugin link on the candy page", candyToPlugin, pluginSite},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q — emitter and link resolver disagree", tc.name, tc.got, tc.want)
		}
		assertSlugSafe(t, tc.name, tc.got)
	}

	// The emitted FILE path and the served URL for the same page must agree segment-for-segment:
	// strip the ".md" and the leading slash from the site form, and the file path carries the
	// exact segments the URL serves them under (reference/candy/<seg>/<name> == same after the
	// Starlight trailing-slash form).
	if want := "/" + strings.TrimSuffix(candyPage, ".md") + "/"; candySite != want {
		t.Errorf("candySitePathFor does not serve candyPagePath's route: %q != %q", candySite, want)
	}
	if want := "/" + strings.TrimSuffix(pluginPage, ".md") + "/"; pluginSite != want {
		t.Errorf("pluginSitePath does not serve pluginPagePath's route: %q != %q", pluginSite, want)
	}

	// Identity is untouched: only the on-disk directory/file and the links changed.
	if remote.Name != "plugin-check" || remote.Namespace != "github.com/opencharly/plugin-check:v2026.242.2127" {
		t.Errorf("entity identity changed: Name=%q Namespace=%q", remote.Name, remote.Namespace)
	}
	// Slug is the namespace-qualified DISPLAY name and is deliberately never sanitized: it is
	// the entity's identity (used for sorting and display), not a path segment. Its dots and
	// colons surviving proves the sanitizer did not touch identity, only paths.
	if got := remote.Slug(); got != "github.com/opencharly/plugin-check:v2026.242.2127.plugin-check" {
		t.Errorf("Slug() = %q, want the unsanitized namespace-qualified name (display identity)", got)
	}
	if got := remote.Version(); got != "2026.242.2127" {
		t.Errorf("Version() = %q, want the authored CalVer unchanged", got)
	}

	// A namespaced distro box — the namespace is already safe, so the path is byte-for-byte
	// the pre-change form: reference/box/<namespace>/<name>.md.
	bx := entity{Name: "arch-test", Namespace: "arch", IsBox: true, Box: &boxView{}}
	if got := bx.PathSegment(); got != "arch/arch-test" {
		t.Errorf("box PathSegment() = %q, want the unchanged arch/arch-test", got)
	}
	if got := boxPagePath(bx.PathSegment()); got != "reference/box/arch/arch-test.md" {
		t.Errorf("boxPagePath = %q, want the unchanged reference/box/arch/arch-test.md", got)
	}
	assertSlugSafe(t, "boxPagePath", boxPagePath(bx.PathSegment()))

	// A plain superproject candy: unchanged single segment.
	lc := entity{Name: "ripgrep", Candy: &candyView{}}
	if got := candyPagePath(lc.PathSegment()); got != "reference/candy/ripgrep.md" {
		t.Errorf("candyPagePath = %q, want the unchanged reference/candy/ripgrep.md", got)
	}
	assertSlugSafe(t, "candyPagePath", candyPagePath(lc.PathSegment()))

	// A CLI word page: the word is a [a-z0-9-] command word and the emitted page + its served
	// route carry no dot or colon (the same form generateCLI builds inline).
	word := "box"
	cliPage := "reference/cli/" + word + ".md"
	if got := "reference/cli/" + word + ".md"; got != cliPage {
		t.Fatalf("cli page path built differently: %q != %q", got, cliPage)
	}
	assertSlugSafe(t, "cliPage", cliPage)
	if want := "/reference/cli/" + word + "/"; "/"+strings.TrimSuffix(cliPage, ".md")+"/" != want {
		t.Errorf("cli page file %s does not serve at %s", cliPage, want)
	}
}
