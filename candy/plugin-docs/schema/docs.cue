// plugin-docs's OWN self-contained CUE schema — the plugin's declaration
// surface, used two ways exactly like every other plugin's schema (there is
// no schema-less plugin):
//
//  1. SERVE over Describe — the host splices `base ++ plugin` at the load gate, so the
//     plugin's declarations travel WITH it and a self-contained schema that will not
//     splice is a LOUD load failure.
//  2. DOCUMENT the plugin's published surface — the reference site's per-plugin page is
//     rendered from its providers, this schema, and the candy description.
//
// command:docs's authored input is its pass-through CLI grammar (the OpRun `{args:
// [...]}` envelope), so this schema DOCUMENTS the command contract rather than a
// structured plugin_input. SELF-CONTAINED: it references no base def, so it compiles
// STANDALONE (the property that lets the SDK compile it serve-side).
#DocsPlugin: {
	// The declared capability words (the plugin.providers surface), recorded here as
	// part of the plugin's published declaration surface.
	providers: [...string]

	// The command word the plugin serves.
	command: "docs"

	// The subcommands of the `charly docs` CLI tree.
	subcommands: ["generate"]

	// What the command does, in one line (the public-docs surface).
	contract: string & !=""

	// The configuration surface: env var names the plugin reads, recorded here as part
	// of the plugin's published declaration surface.
	config?: [string]: string
}
