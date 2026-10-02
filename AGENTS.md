# AGENTS.md — plugin-docs

Standalone runtime plugin repo owning the `charly docs` generator
(`command:docs`, out-of-process). The plugin is a Go module at
`candy/plugin-docs/` (module path
`github.com/opencharly/plugin-docs/candy/plugin-docs`); the root `charly.yml`
only declares `discover: candy` so the repo is a project and its candy is
scanned.

Canonical files:

- `candy/plugin-docs/charly.yml` — the `plugin-docs:` candy entity (`plugin:`
  block, `plan:` check).
- `candy/plugin-docs/plugin.go` — the command provider (`NewProvider()` /
  `NewMeta()` / `CliMain`) and the `Invoke(OpRun)` path.
- `candy/plugin-docs/command.go` — the `charly docs` CLI dispatch.
- `candy/plugin-docs/generate.go` — the top-level generator; the per-page
  emitters are `gen_*.go`; `catalog.go` walks the repos and resolves refs;
  `links.go` rewrites cross-references.
- `candy/plugin-docs/schema/docs.cue` — the self-contained `#DocsPlugin`.
- `charly.yml` — the root project manifest (`discover: candy`) plus the
  `docs-resolve` disposable local bed.
- `.github/workflows/tag-on-merge.yml` — CalVer tag + `CHANGELOG/` on merge.
- `README.md` — user overview only; never agent guidance.

## Load these skills first (R0)

- `/charly-build:docs` — the `charly docs generate` verb, the site's generated
  trees, and the docs-repo regeneration flow. Load before changing any emitter.
  This candy carries no `skill:` entity of its own; the gap is tracked in
  [opencharly/opencharly#291](https://github.com/opencharly/opencharly/issues/291).
- `/charly-internals:plugin` — the plugin authoring reference: the `plugin:`
  block, the `command` provider class, the per-plugin CUE-schema contract.
- `/charly-internals:skills` — the skill corpus contract the generator projects
  (frontmatter, the cross-reference rules, the drift gate).
- `/charly-internals:git-workflow` — before any git/PR action.

## Build / validate / test

- `go build ./...` in `candy/plugin-docs/` — compile the plugin module.
- `go test ./...` in `candy/plugin-docs/` — the plugin's Go tests (the
  catalog/resolver/emitter seams).
- `charly box validate` at the repo root — the structural check (the candy +
  `plugin:` block, CUE schema).
- The merge gate is the **org-wide** `charly/pr-validator` (required check
  `validate / validate`, defined in `opencharly/.github`); this repo has **no**
  per-repo candy gate.
- The R10 witness is the `docs-resolve` disposable local bed (declared in
  `charly.yml`): it runs `charly docs generate` against a hermetic fixture and
  asserts the Go-module tag form resolves, that the landing projection lifts the
  hero tagline from under the README's badge lines while keeping the badges in the
  body, and that the CLI page path for a colon-carrying command word is the
  sanitized form. The fixture README carries badges for exactly that reason — a
  fixture without them does not exercise the landing path at all — and the fixture
  project resolves `plugin-box` through `extra_repos` for the same reason: it is
  the candy that declares the colon-carrying `command:<verb>:box` words, so a
  fixture without it does not exercise the CLI page-path path at all. Its
  hand-authored start page links to one of those words at the sanitized spelling,
  which makes the fixture's own link gate the witness.

## Modify this repo

- Edit the `plugin-docs:` candy entity, the Go source, and `schema/docs.cue`
  **together** — the schema is the served declaration surface.
- Keep the generator reading DECLARATIVE sources (`plugin.providers`, the
  per-plugin schema, each candy's `description:`) — parsing the host's rendered
  Kong grammar would be the fragile shim R4 forbids.
- An unresolvable `/charly-<plugin>:<skill>` cross-reference is a hard build
  error by design: fix the reference in its owning candy, never weaken the gate.

## Landing

- PR-only. Every change lands through a pull request; the org-required
  `charly/pr-validator` validates the diff and body and arms native auto-merge on
  PASS. Direct pushes to `main` are blocked.
- History lives in `CHANGELOG/` (written by `tag-on-merge` at merge time); the PR
  body IS the changelog.
- The authoritative rulebook is the umbrella `AGENTS.md` in
  `opencharly/opencharly` and `charly/AGENTS.md` in the charly repo. Do not
  restate its rules here.
