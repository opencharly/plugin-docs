package docs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/opencharly/spec/spec"
)

// TestAggregateEmittersOrderIndependent locks in that a word served by MORE THAN ONE plugin
// version emits deterministically regardless of the corpus walk order. The catalog documents
// every DEFINED version, so ties are normal — `deploy` is served by two plugin-fleet versions
// at once. Keying the sort on (class, word) alone left the relative order of those rows to the
// walk order, so ANY corpus pin change reshuffled the whole provider index. This test drives
// the two aggregate emitters with the same entity set in two opposite orders and requires
// byte-identical output.
func TestAggregateEmittersOrderIndependent(t *testing.T) {
	fleetA := pluginEntity{entity: entity{
		Name:      "plugin-fleet",
		Namespace: "github.com/opencharly/plugin-fleet:v2026.250.0537",
		Candy:     &candyView{Plugin: &spec.Plugin{Providers: []spec.PluginCapability{"command:deploy"}}},
	}}
	fleetB := pluginEntity{entity: entity{
		Name:      "plugin-fleet",
		Namespace: "github.com/opencharly/plugin-fleet:v2026.250.0829",
		Candy:     &candyView{Plugin: &spec.Plugin{Providers: []spec.PluginCapability{"command:deploy"}}},
	}}
	file := pluginEntity{entity: entity{
		Name:      "plugin-file",
		Namespace: "github.com/opencharly/plugin-file:v2026.242.2145",
		Candy:     &candyView{Plugin: &spec.Plugin{Providers: []spec.PluginCapability{"verb:file"}}},
	}}

	emit := func(order []pluginEntity) (string, string) {
		dir := t.TempDir()
		if _, err := generateProviderIndex(dir, order); err != nil {
			t.Fatalf("generateProviderIndex: %v", err)
		}
		if _, err := generateCLI(dir, order); err != nil {
			t.Fatalf("generateCLI: %v", err)
		}
		prov, err := os.ReadFile(filepath.Join(dir, "reference", "providers.md"))
		if err != nil {
			t.Fatalf("read providers.md: %v", err)
		}
		cli, err := os.ReadFile(filepath.Join(dir, "reference", "cli", "deploy.md"))
		if err != nil {
			t.Fatalf("read cli/deploy.md: %v", err)
		}
		return string(prov), string(cli)
	}

	prov1, cli1 := emit([]pluginEntity{fleetA, fleetB, file})
	prov2, cli2 := emit([]pluginEntity{file, fleetB, fleetA}) // reversed walk order

	if prov1 != prov2 {
		t.Errorf("reference/providers.md depends on walk order:\n--- order1\n%s\n--- order2\n%s", prov1, prov2)
	}
	if cli1 != cli2 {
		t.Errorf("reference/cli/deploy.md depends on walk order:\n--- order1\n%s\n--- order2\n%s", cli1, cli2)
	}
}

// TestGenerateRecipesIndexOrderIndependent extends the same property to the recipe index: two
// marketplace plugins sharing a Name (different Dir) are a tie in writeBucket's plugin sort, and
// the emitted recipes/index.md must not depend on their input order.
func TestGenerateRecipesIndexOrderIndependent(t *testing.T) {
	a := marketplacePlugin{Name: "charly-internals", Source: "./internals", Category: "development", Description: "a"}
	b := marketplacePlugin{Name: "charly-internals", Source: "./internals-extra", Category: "development", Description: "b"}
	sa := skill{PluginDir: "internals", PluginName: "charly-internals", Name: "skills", Title: "Skills"}
	sb := skill{PluginDir: "internals-extra", PluginName: "charly-internals", Name: "skills", Title: "Skills extra"}

	emit := func(m *marketplace, skills []skill) string {
		dir := t.TempDir()
		if err := generateRecipesIndex(dir, skills, m); err != nil {
			t.Fatalf("generateRecipesIndex: %v", err)
		}
		raw, err := os.ReadFile(filepath.Join(dir, "recipes", "index.md"))
		if err != nil {
			t.Fatalf("read recipes/index.md: %v", err)
		}
		return string(raw)
	}

	got1 := emit(&marketplace{Plugins: []marketplacePlugin{a, b}}, []skill{sa, sb})
	got2 := emit(&marketplace{Plugins: []marketplacePlugin{b, a}}, []skill{sb, sa}) // reversed walk order
	if got1 != got2 {
		t.Errorf("recipes/index.md depends on walk order:\n--- order1\n%s\n--- order2\n%s", got1, got2)
	}
}
