# Releasing

Cutting a release is a prep commit followed by a tag. The workflow in `.github/workflows/release.yml` builds the binaries and publishes the notes; everything below is what has to be true *before* you push the tag, because the workflow fails rather than guesses.

## What the automation enforces

Two gates fail a release outright:

- **The tag must match `nns.Version`.** The workflow runs `alp version` on the built binary and compares it to the tag. `nns/version.go` is the single source of truth: the flake stamps it into `main.version`, so a mismatch would ship binaries reporting the wrong version.
- **`CHANGELOG.md` must have a section for the tag.** Release notes are extracted from the `## [X.Y.Z]` heading matching the tag. A tag with no section fails instead of publishing empty notes.

Two more are enforced by `go test`, so they break CI before you ever tag:

- `TestVersionMatchesChangelog` compares the newest CHANGELOG section to `nns.Version`.
- `TestSchemaUnreleasedIsNotVersion` fails when `SchemaUnreleased` equals `Version`, which is what forces you to clear it as part of the bump.

## The prep commit

Do all of these together. They are interlocked: any one alone leaves the tree red.

1. **`nns/version.go`** — bump `Version` to the new tag.

2. **`CHANGELOG.md`** — rename `## [Unreleased]` to `## [X.Y.Z] - YYYY-MM-DD`. Every user-visible change since the last release needs an entry; the section is published verbatim as the release notes, so write it for someone deciding whether to upgrade. Call out behaviour changes explicitly, including ones that are source-compatible but make a previously passing command fail.

3. **`nns/schema.go`** — set `SchemaUnreleased` back to `""`. During development it names the version being worked on, so the docs can mark new fields unreleased rather than linking a tag that does not exist. Once the tag ships, every Since value links its tag.

4. **Regenerate the config reference** — `go generate ./nns`, which rewrites `docs/config.md`. This is not optional: the page renders `SchemaUnreleased` into its intro and its schema-history list, so skipping it leaves the site claiming the release is unreleased.

5. **`web/src/pages/index.astro`** — update the flake-input example to the new tag. Nothing checks this, and it has silently rotted before.

Then verify:

```
go test ./...
nix run .#alp -- version    # expect X.Y.Z-dirty before committing, X.Y.Z after
```

The `-dirty` suffix comes from an uncommitted tree and disappears once the prep commit lands. That is deliberate, so a working-tree build is never mistaken for the release it is based on.

## Choosing the number

Before 1.0, a minor bump may change behaviour; that is stated at the top of the CHANGELOG. In practice:

- **Minor** for a new config field, a new command or flag, a new section in `state.json`, a new network dependency, or anything that makes a previously passing `reconcile` report drift. All of these have shipped as minors.
- **Patch** for fixes that change no schema, no state format, and no verdict a command reaches.

When in doubt, prefer minor. A config repo pins a tag, so an unexpected behaviour change is more expensive than an unnecessary version digit.

## Adding a schema field mid-cycle

When a new HCL field lands between releases:

1. Set `SchemaUnreleased` in `nns/schema.go` to the *next* version, if it is not set already.
2. Give the field's `SchemaFieldSince` entry that same version.
3. Add the prose to `SchemaFieldDocs`, and the version to the `want` map in `TestSchemaFieldSinceValues`.
4. Run `go generate ./nns`.

`TestSchemaSinceNotAheadOfVersion` allows a Since value ahead of `Version` only when it equals `SchemaUnreleased`, which is what keeps the docs honest about what has actually shipped.

## Licence

The BUSL Additional Use Grant and its node-machine limit apply **per version**, and the Change Date varies per version. Whatever `LICENSE` says when you tag is what binds that version permanently; lowering a limit later does not affect releases already published. Check it deliberately when the terms are under discussion, rather than assuming the last release's terms carry forward.

## Tagging

Once the prep commit is on `main`:

```
git tag vX.Y.Z
git push origin vX.Y.Z
```

The workflow cross-compiles `alp` for each target with the tag injected into `main.version`, packages the binaries with `LICENSE`, `NOTICE`, `THIRD_PARTY_LICENSES`, `CHANGELOG.md`, and `docs/`, then publishes the release with the tag's CHANGELOG section and the computed BUSL dates as notes.
