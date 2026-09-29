# plugin-docs

The `charly docs` generator for OpenCharly — renders the reference half of the
[opencharly.ai](https://opencharly.ai) documentation site from a project's
canonical sources, served out-of-process (`command:docs`).

The plugin is a standalone Go module the host `syscall.Exec`s in CLI mode on the
first `charly docs …` invocation. It is deliberately NOT compiled into the charly
binary: it is a DEV-TIME documentation generator, run on a contributor's machine
to regenerate the site.

## What it provides

| Capability | Surface |
|---|---|
| `command:docs` | `charly docs generate` |

## What it generates

- one page per skill (plus each `references/*.md` split file, with every
  `/charly-<plugin>:<skill>` cross-reference rewritten to a site link — and any
  unresolvable one failing the build);
- one page per plugin candy carrying its providers, its placement COMPUTED from
  `compiled_plugins` membership, and its rendered CUE parameter schema;
- a provider cross-index mapping every reserved word to the plugin that serves it;
- one page per defined candy and box;
- `VISION.md` with its H1 dropped and links rewritten for the web.

The generator reads DECLARATIVE sources rather than charly's assembled CLI model:
every fact the site needs is already declared (`plugin.providers`, the per-plugin
`schema/*.cue`, and each candy's `description:`).

## How to use it

Compose the plugin candy in a project's `candy:` list:

```yaml
- '@github.com/opencharly/plugin-docs/candy/plugin-docs:<tag>'
```

Then run the generator:

```bash
charly docs generate
```

## Layout

- `candy/plugin-docs/` — the plugin module: `plugin.go` (the command provider +
  `NewProvider()` / `NewMeta()` / `CliMain`), `command.go` (the CLI dispatch),
  `generate.go` (the top-level generator), the per-page emitters (`gen_skills.go`,
  `gen_plugins.go`, `gen_entities.go`, `gen_cli.go`, `gen_vision.go`, …),
  `catalog.go` (the repo walk + resolution), `links.go` (the cross-reference
  rewriter), `schema/docs.cue` (the self-contained `#DocsPlugin`), the Go tests,
  `cmd/serve/main.go`.
- `charly.yml` — the root project manifest (`discover: candy`) plus the
  `docs-resolve` disposable local bed (the extra_repos resolution witness).
- `.github/workflows/tag-on-merge.yml` — CalVer tag + `CHANGELOG/` on merge.

## Related

- Owning skill: `/charly-build:docs` — the `charly docs generate` verb and the
  site's generated trees. This candy carries no `skill:` entity of its own; the
  gap is tracked in
  [opencharly/opencharly#291](https://github.com/opencharly/opencharly/issues/291).
- `/charly-internals:plugin` — the plugin/provider model, including the `command`
  provider class.
- [`opencharly/docs`](https://github.com/opencharly/docs) — the standalone
  documentation site repo.
- [`opencharly/charly`](https://github.com/opencharly/charly) — the charly CLI.
