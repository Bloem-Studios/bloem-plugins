---
title: Bloem Plugin Catalog Admin Guide
description: How to run the private Bloem plugin catalog and its updater — repository secrets, the two GitHub workflows, publishing a plugin release into the catalog, materialising and hosting a static staging catalog, retention, and troubleshooting — written for someone doing it for the first time.
summary: What the catalog is, the manifest.json format, the update-catalog and materialize-private-catalog commands with every flag and environment variable, CI and update automation, the checksum-based trust model, retention rules, and symptom-to-fix troubleshooting.
tags:
  - admin
  - operator
  - plugins
  - catalog
  - release-engineering
audience:
  - operator
last_reviewed: 2026-09-05
related:
  - user-guide.md
  - private-staging.md
  - ../README.md
---

# Bloem Plugin Catalog Admin Guide

This repository is the **private plugin catalog** for Bloem Server: a single JSON file
(`manifest.json`) that lists which plugin versions a Bloem Server may install, plus the tooling that
keeps that file honest. This guide is for the person who operates it — holds the GitHub secrets,
publishes plugin releases into it, and serves it to servers. It assumes no prior knowledge of the
catalog; if a word is unfamiliar, see the [glossary](#glossary).

If you are a Bloem Server administrator who wants to *use* the catalog, or a plugin author who
wants to *get a release into it*, read the [User Guide](user-guide.md) first; it covers the
consumer side and the release checklist.

> **Naming note.** This repository predates the Bloem rename and still carries its earlier
> identity in real identifiers: the Go module is `github.com/Vondel-Media/vondel-plugins`, the
> secrets are `VONDEL_*`, and the allowlisted plugin repositories live under the `Vondel-Media`
> GitHub organisation. Those names are enforced by tests (`catalog/identity_test.go`) and by the
> updater's allowlist, so this guide uses them verbatim where they are identifiers. The product is
> Bloem.

**How this guide is organised.** Part 1 explains what the catalog is and how a server reads it.
Part 2 covers hosting: secrets, workflows, the static staging tree. Part 3 is the reference for the
two commands and the manifest format. Part 4 is operations and troubleshooting.

---

## Part 1 — What you are running

### 1.1 The catalog in one paragraph

A Bloem Server has an **Admin → Plugins** page with a *Catalog* tab. Behind that tab is a list of
**repositories**: URLs the server fetches as JSON. Each JSON document is a `RepositoryIndex` — a
`plugins` array of packages, each with a plugin manifest, a repository URL, a `checksums.txt` URL
and per-platform binary URLs with SHA-256 checksums. The server shows entries whose API version
and platform match, downloads the binary on install, verifies the checksum, and refuses a mismatch.
This repository produces exactly one such document, `manifest.json`, and nothing else.

### 1.2 What is in the catalog today

The catalog holds **exactly six plugins, one version each**. The list is literal in code
(`retainedPlugins` in `cmd/materialize-private-catalog/main.go`, `allowedRepositories` in
`cmd/update-catalog/main.go`, and the exact versions in `catalog/identity_test.go`):

| Plugin id | Source repository | Version pinned by tests |
|---|---|---|
| `silo.tmdb` | `Vondel-Media/vondel-plugin-tmdb` | 1.2.23 |
| `silo.tvdb` | `Vondel-Media/vondel-plugin-tvdb` | 1.2.27 |
| `silo.ebook-metadata` | `Vondel-Media/vondel-plugin-ebook-metadata` | 0.1.3 |
| `silo.audiobook-metadata` | `Vondel-Media/vondel-plugin-audiobook-metadata` | 0.1.6 |
| `silo.manga-metadata` | `Vondel-Media/vondel-plugin-manga-metadata` | 0.1.3 |
| `silo.autoscan.arr` | `Vondel-Media/vondel-plugin-autoscan-arr` | 0.1.4 |

Every entry advertises three platforms — `darwin/arm64`, `linux/amd64`, `linux/arm64` — and
`silo_api_version: v1`. (The `silo.` prefix and the `silo_api_version` field are wire-level names
in the plugin SDK's manifest proto; they are not something this repository can rename.)

### 1.3 The three pieces

| Piece | Path | Role |
|---|---|---|
| The catalog | `manifest.json` | The reviewed, committed source of truth. |
| The updater | `cmd/update-catalog` | Adds or replaces one plugin's entry from a GitHub release, after verifying it. Run by the **Update Catalog** workflow. |
| The materialiser | `cmd/materialize-private-catalog` | Downloads every retained release, re-verifies it, and writes a static directory (`catalog.json` + binaries) you can serve on a private network. Run by an operator, by hand. |

`catalog/catalog.go` holds the shared types and `BuildPackageFromRelease`, the function that turns
a GitHub release plus a source manifest into a catalog package.

### 1.4 Trust model

There is **no cryptographic signing** in this pipeline. Trust rests on three things:

1. **GitHub authentication.** The updater and materialiser read releases through the GitHub API
   with a token that can see the private plugin repositories. Nothing unauthenticated can be
   published from.
2. **Allowlists.** The updater accepts only the six repositories above; the materialiser accepts
   only those six *and* checks each package's plugin id matches its repository.
3. **SHA-256 checksums, verified three times.** The updater downloads every release asset,
   checks each binary against `checksums.txt`, and records the checksums in the catalog. The
   materialiser downloads again and checks the catalog's checksums against fresh `checksums.txt`
   content. The Bloem Server checks the downloaded binary against the catalog's checksum on
   install (`binary checksum mismatch` is a hard failure there).

When the updater runs on a machine whose OS/arch matches one of the release binaries, it also
executes that binary with the argument `manifest` and compares the printed manifest's plugin id,
version, API version, checksum, source URL and publisher URL against the source manifest.

> **No token belongs in source, catalog data, release assets, or server binaries.** The
> materialiser scans its whole output for the token it was given and fails if it finds it
> ("credential canary detected in staging output"). Never point a Bloem Server at a URL that
> carries credentials.

---

## Part 2 — Hosting the catalog

### 2.1 Repository secrets

The workflows use three GitHub Actions secrets on this private repository:

| Secret | Used by | Purpose |
|---|---|---|
| `VONDEL_MODULES_TOKEN` | `ci.yml`; `update-catalog.yml` (prefetch job only) | Read access to the private plugin SDK module `github.com/Vondel-Media/vondel-plugin-sdk` so `go build`/`go test` can resolve it. |
| `VONDEL_CATALOG_SOURCE_TOKEN` | `update-catalog.yml` (update step only, as `GITHUB_TOKEN`) | Read access to the private plugin repositories: release metadata, release assets, and `manifest.json` at the tag. |
| `VONDEL_CATALOG_PUSH_TOKEN` | `update-catalog.yml` (final commit step only) | Push access to this repository's `main`. |

A test (`catalog/automation_test.go`) asserts that each secret appears exactly once in the update
workflow, in the step that needs it and no earlier, that checkouts use `persist-credentials: false`,
that `setup-go` uses `cache: false`, and that the workflow never runs `gh auth setup-git`. Do not
"simplify" the workflows in a way that breaks those invariants; CI will fail.

### 2.2 CI (`.github/workflows/ci.yml`)

Runs on every pull request and on pushes to `main`. Steps: check out; set up Go 1.26; write a
temporary `GIT_ASKPASS` script that answers with `x-access-token` / `$VONDEL_MODULES_TOKEN`;
`go clean -modcache`; then `GOWORK=off go test ./...`, `go vet ./...`, `go build ./...` with
`GOPRIVATE`/`GONOSUMDB` set to `github.com/Vondel-Media/*`; finally a **Guard private-only source**
step that fails the build if:

- any `go.mod` contains a `replace` directive;
- any workflow references the upstream project's dispatch token or its catalog repository's
  `dispatches`/`actions` endpoints;
- any workflow contains a repository visibility change, `npm publish`, `docker push`, or a
  GitHub Pages publish.

The askpass script is removed in an `always()` step.

### 2.3 The update workflow (`.github/workflows/update-catalog.yml`)

**Triggers.**

- `repository_dispatch` with event type `plugin_release_published` and a `client_payload` of
  `{"repo": "<owner/name>", "tag": "vX.Y.Z"}` — the path plugin repositories use.
- `workflow_dispatch` with inputs `repo` and `tag` — the manual path from the Actions tab.

Concurrency group `plugin-catalog-update` with `cancel-in-progress: false`: updates queue, they
never race.

**Job 1 — `prefetch-private-sdk`.** Runs *without* checking out this repository. With
`VONDEL_MODULES_TOKEN` it downloads exactly `vondel-plugin-sdk@v0.13.3` into a private module
cache, greps that cache for the token (fails with `sanitized SDK cache contains credential
material` if found), tars it, and uploads it as the artifact `sanitized-vondel-plugin-sdk-v0.13.3`
with one-day retention. This is how the module token never coexists with a checkout.

**Job 2 — `update`.** Checks out, installs the sanitised SDK cache into `GOMODCACHE`, runs
`go test ./...`, then runs the updater **twice** and requires the two results to be byte-identical
(`cmp`), and requires `jq -e . manifest.json` to parse. If `manifest.json` changed, it commits as
`github-actions[bot]` with the message `chore: update catalog for <repo>@<tag>`, does
`git pull --rebase` and pushes `HEAD:main` using `VONDEL_CATALOG_PUSH_TOKEN` through a second
temporary askpass script. If nothing changed, it exits cleanly.

> **The identity test pins exact versions.** `catalog/identity_test.go` asserts each plugin's
> version literally (see 1.2). The update job runs tests *before* the updater changes the file, so
> a version bump passes that run and lands on `main`; the *next* CI run then fails until someone
> updates `exactVersions` in the test (and, if the materialiser is used, the same version is what
> it will fetch). Treat "bump the test" as part of every release — see 4.2.

### 2.4 Publishing a plugin version (operator side)

1. Confirm the plugin repository and tag meet the release contract (User Guide, section 3).
2. Either let the plugin repository's own automation send `plugin_release_published`, or open
   **Actions → Update Catalog → Run workflow** and enter `repo` (`Vondel-Media/vondel-plugin-tmdb`)
   and `tag` (`v1.2.24`).
3. Watch the run. Any failure in "Update catalog manifest" is the updater refusing the release;
   the message names the check (Part 3.1 lists them all).
4. On success, `main` gains one commit touching only `manifest.json`. Review the diff: the entry
   for that plugin id is replaced wholesale with the new version, new URLs and new checksums.
5. Update `exactVersions` in `catalog/identity_test.go` in a follow-up commit so CI is green.
6. If you serve a static staging tree, re-run the materialiser (2.5); servers pointed at the
   static tree do not see the change until you do.

### 2.5 Serving the catalog to Bloem Servers

`manifest.json` on GitHub is **not** a usable endpoint: the repository is private, the binary URLs
point at private GitHub releases, and a Bloem Server cannot authenticate to either. The README
says it plainly: "This repository is not a public catalog endpoint. … public defaults must not point
here while the repository and its release sources require authentication."

The supported way to serve it is the **static staging tree** produced by the materialiser
(`docs/private-staging.md`):

```sh
GITHUB_TOKEN="$VONDEL_CATALOG_SOURCE_TOKEN" \
  GOWORK=off go run ./cmd/materialize-private-catalog \
  -catalog manifest.json \
  -output /srv/vondel-plugin-staging
```

This produces:

```
/srv/vondel-plugin-staging/
  catalog.json
  plugins/<plugin id>/<version>/checksums.txt
  plugins/<plugin id>/<version>/plugin-darwin-arm64
  plugins/<plugin id>/<version>/plugin-linux-amd64
  plugins/<plugin id>/<version>/plugin-linux-arm64
```

`catalog.json` is the same `RepositoryIndex` shape but with **relative** `checksums_url` and binary
URLs (`plugins/silo.tmdb/1.2.23/plugin-linux-amd64`), which a Bloem Server resolves against the
repository URL it was given. Serve the directory with any static file server on a network only
your servers can reach; the documented example is:

```sh
cd /srv/vondel-plugin-staging
python3 -m http.server 8080 --bind 127.0.0.1
```

Then add `http://<host>:8080/catalog.json` as a repository on each Bloem Server (User Guide,
section 2). The static tree contains no credentials by construction (the canary scan), so it can be
served anonymously *within that network*. It must still not be exposed publicly: the binaries are
private releases.

The materialiser is **atomic**: it builds into a hidden sibling temp directory, and only on
complete success renames it into place, moving the previous tree aside and deleting it afterwards.
A failed run leaves the previous tree untouched. It also refuses an `-output` that is a symlink or
a non-directory, and refuses a `-catalog` that is not a regular file.

---

## Part 3 — Reference

### 3.1 `cmd/update-catalog`

```
GOWORK=off go run ./cmd/update-catalog -repo <owner/name> -tag <vX.Y.Z> [-manifest manifest.json]
```

| Flag | Default | Meaning |
|---|---|---|
| `-repo` | (required) | GitHub repository in `owner/name` form. Must be in the literal allowlist of six. |
| `-tag` | (required) | Release tag. Must be exactly `v` + the manifest's `version`. |
| `-manifest` | `manifest.json` | Path of the catalog file to read and rewrite. An empty file is treated as an empty catalog. |

| Environment | Meaning |
|---|---|
| `GITHUB_TOKEN` | Bearer token for `api.github.com`. Optional in code (unauthenticated reads work for public repositories) but required in practice because the plugin repositories are private. The token is stripped from any redirect the asset download follows. |

**What it verifies, in order.** Each failure exits 1 with the message shown.

1. `repository "…" is not allowed` — repo outside the allowlist.
2. Fetches the release by tag (`/repos/{repo}/releases/tags/{tag}`, no redirects followed) and the
   source `manifest.json` at that tag (`/repos/{repo}/contents/manifest.json?ref={tag}`, raw).
3. `release must contain exactly four assets` / `unexpected asset` / `duplicate asset` — assets
   must be exactly `checksums.txt`, `plugin-darwin-arm64`, `plugin-linux-amd64`, `plugin-linux-arm64`.
4. `non-allowlisted API URL` / `non-allowlisted browser download URL` — each asset's `url` must be
   `https://api.github.com/repos/{repo}/releases/assets/{id}` and its `browser_download_url` must
   be `https://github.com/{repo}/releases/download/{tag}/{name}`.
5. Downloads all four assets (`Accept: application/octet-stream`, at most 4 redirects).
6. `checksums.txt must contain exactly three lines`, each `<64 hex> <bare name>` (a leading `*`
   is tolerated, path separators are not); every binary's SHA-256 must match; every binary must be
   named exactly once.
7. `source manifest version does not exactly match release tag`; API version must be `v1`;
   `supported_platforms` must be exactly the three, no duplicates.
8. Native check (only when a `plugin-<GOOS>-<GOARCH>` asset exists for the running machine):
   writes the binary to a temp file, runs it with `manifest`, and compares plugin id, version, API
   version, checksum (must equal the binary's SHA-256), presentation `source_url` and
   `publisher_url`.
9. `BuildPackageFromRelease`: rejects drafts, prereleases, unpublished releases, missing or
   duplicate `checksums.txt`, unsupported or duplicate platform binaries, an advertised platform
   with no binary, capabilities without type and id, and runs the SDK's
   `ValidateCatalogPresentation` against the manifest's `presentation` block with the repository
   URL. It sets the package version from the tag (leading `v` stripped) and blanks the manifest's
   self-stamped `checksum` (catalog entries carry per-platform checksums instead).
10. `UpsertPackage`: removes any existing entry with the same plugin id, appends the new one,
    sorts by plugin id, and writes the file with two-space indentation and a trailing newline.

### 3.2 `cmd/materialize-private-catalog`

```
GITHUB_TOKEN=… GOWORK=off go run ./cmd/materialize-private-catalog [-catalog manifest.json] [-output private-staging]
```

| Flag | Default | Meaning |
|---|---|---|
| `-catalog` | `manifest.json` | Catalog to materialise. Must be a regular, non-symlink file. |
| `-output` | `private-staging` | Directory to (re)create atomically. Must not be a symlink or an existing non-directory. |

| Environment | Meaning |
|---|---|
| `GITHUB_TOKEN` | **Required** (`GITHUB_TOKEN is required`). Used for every API read; stripped on redirects; scanned for in the output. |

**What it verifies.** `catalog must contain exactly the six retained plugins`; per package:
repository URL must be `https://github.com/<owner>/<name>` with no query, fragment or user info;
repository must be retained and its plugin id must match; no duplicate repositories; version must
match `^\d+\.\d+\.\d+$` (no prerelease suffixes); API version `v1`; at least one capability;
exactly three platforms in both `supported_platforms` and `binaries`; each binary checksum 64 hex;
each binary URL exactly `https://github.com/{repo}/releases/download/v{version}/plugin-{os}-{arch}`;
`checksums_url` exactly the release's `checksums.txt`. Then it fetches the release (tag `v{version}`,
not draft/prerelease, published), validates the four assets exactly as the updater does, downloads
them (binaries capped at 512 MiB, `checksums.txt` at 1 MiB), verifies checksums, and requires the
catalog's recorded checksum to equal the freshly verified one (`catalog checksum does not match
verified release`). Output files are written `0644` (checksums, catalog) and `0755` (binaries).
The HTTP client timeout is two minutes per request.

### 3.3 `manifest.json` format

```json
{
  "plugins": [
    {
      "manifest": {
        "plugin_id": "silo.tmdb",
        "version": "1.2.23",
        "silo_api_version": "v1",
        "supported_platforms": [{"os": "linux", "arch": "amd64"}, …],
        "capabilities": [{"type": "metadata_provider.v1", "id": "tmdb", "display_name": "…", "description": "…", "metadata": {…}}],
        "presentation": {
          "display_name": "…", "summary": "…", "description_markdown": "…", "setup_markdown": "…",
          "homepage_url": "…", "source_url": "…", "support_url": "…", "changelog_url": "…",
          "publisher_name": "Vondel", "publisher_url": "https://github.com/Vondel-Media",
          "license_spdx": "AGPL-3.0-only"
        }
      },
      "repo_url": "https://github.com/Vondel-Media/vondel-plugin-tmdb",
      "checksums_url": "https://github.com/…/releases/download/v1.2.23/checksums.txt",
      "binaries": {
        "darwin/arm64": {"url": "…/plugin-darwin-arm64", "checksum": "<sha256>"},
        "linux/amd64":  {"url": "…/plugin-linux-amd64",  "checksum": "<sha256>"},
        "linux/arm64":  {"url": "…/plugin-linux-arm64",  "checksum": "<sha256>"}
      }
    }
  ]
}
```

Go types: `RepositoryIndex{Plugins []CatalogPackage}`, `CatalogPackage{Manifest, RepoURL,
ChecksumsURL, Binaries map[string]PlatformBinary}`, `PlatformBinary{URL, Checksum}`
(`catalog/catalog.go`). The `manifest` block is the SDK's `PluginManifest` proto in protojson; the
catalog tests also assert that no package serialises with `Silo-Server/`, `api.github.com` or
`Authorization: Bearer` in it.

### 3.4 Retention

- **One version per plugin.** `UpsertPackage` replaces; the catalog never lists two versions of the
  same plugin id. Older versions live on only as GitHub releases in the plugin repository and in
  git history here.
- **Exactly six plugins.** Adding a seventh requires code changes in three places (both allowlists
  and the identity test); the materialiser refuses a catalog of any other size.
- **Static trees keep only what the catalog says.** Re-running the materialiser replaces the whole
  output directory; a previous version's binaries are deleted with it. Keep your own archive if you
  need rollback material, or simply re-run the updater with the older tag (it is an ordinary
  release).

---

## Part 4 — Operating

### 4.1 Local development

Go 1.26. Because the SDK is private, set `GOPRIVATE=github.com/Vondel-Media/*` and have git
credentials for that organisation; run everything with `GOWORK=off` so a parent workspace cannot
pull in a `replace` (CI forbids `replace` directives in this repository, and the identity test
checks `go.mod` for them). `go test ./...` runs the catalog unit tests, the workflow/documentation
invariants, and the updater/materialiser tests against fake HTTP servers; no network access is
needed.

### 4.2 Release checklist for this repository

1. Updater run succeeded and the `manifest.json` diff is what you expect.
2. `catalog/identity_test.go` `exactVersions` updated; CI green.
3. If the SDK version changes: update `go.mod`, the identity test's `v0.13.3` assertion, and the
   three literal `v0.13.3` occurrences in `update-catalog.yml` (module download, artifact name,
   tar name).
4. Static tree re-materialised where one is served.

### 4.3 Rolling back a plugin version

Run the update workflow with the previous tag. The updater does not know or care about ordering;
it replaces the entry with whatever release you name, as long as it passes verification. Then
re-materialise.

### 4.4 Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `repository "…" is not allowed` | Repo not in the six-entry allowlist. | Only retained repositories can publish; adding one is a code change (3.4). |
| `GitHub API request returned status 404` on fetch release | Tag does not exist, is a draft, or the token cannot see the repository. | Check the tag and that `VONDEL_CATALOG_SOURCE_TOKEN` has read access. |
| `source manifest request returned status 404` | No `manifest.json` at the repository root at that tag. | The plugin must commit its manifest at the tag. |
| `release must contain exactly four assets` | Extra, missing or differently named assets. | Publish exactly `checksums.txt` + the three `plugin-*` binaries. |
| `checksums.txt must contain exactly three lines` / `binary checksum mismatch` | Checksums file malformed or stale. | Regenerate it from the final binaries with `sha256sum plugin-*`. |
| `source manifest version does not exactly match release tag` | Tag `v1.2.24` but manifest says `1.2.23` (or tag lacks the `v`). | Bump the manifest version before tagging. |
| `source manifest must advertise exactly three supported platforms` | Manifest lists fewer/more/duplicate platforms. | Advertise exactly darwin/arm64, linux/amd64, linux/arm64. |
| `native manifest identity or checksum does not match` | The binary's self-reported manifest differs from the committed one, or the binary was rebuilt after `checksums.txt`. | Rebuild, regenerate checksums, re-upload, retag if needed. |
| `source manifest presentation: …` | A presentation field failed the SDK's `ValidateCatalogPresentation` (typically a URL not under the repository, or a missing field). | Fill every presentation field; source/homepage should be the repository URL. |
| Update workflow: second run differs (`cmp` fails) | Non-deterministic output — should never happen; indicates a tooling bug. | Investigate before merging anything. |
| Update workflow: push rejected | `VONDEL_CATALOG_PUSH_TOKEN` expired or lacks push. | Rotate the secret. |
| CI: `sanitized SDK cache contains credential material` | The module token leaked into the downloaded cache. | Do not proceed; inspect the SDK module for embedded URLs. |
| CI: identity test fails on version | A catalog update landed without bumping `exactVersions`. | Update the test (4.2). |
| Materialiser: `catalog must contain exactly the six retained plugins` | Catalog edited by hand or a plugin removed. | Restore the six entries. |
| Materialiser: `credential canary detected in staging output` | The token string appeared in a downloaded file. | Stop; rotate the token; examine the offending release. |
| Materialiser: `output must be a real directory path` | `-output` is a symlink or a file. | Point at a real directory (existing or not). |
| Bloem Server shows nothing from the repository | The repository URL is unreachable from the server, returns non-200, or the JSON has no entry for the server's OS/arch and API `v1`. | Check the server's log line `skipping broken plugin repository`; verify the URL serves `catalog.json`. |
| Bloem Server install fails with `binary checksum mismatch` | The served binary differs from the catalog checksum (stale static tree). | Re-materialise. |

---

## Glossary

- **Allowlist** — the literal list of six source repositories the tooling accepts.
- **Asset** — a file attached to a GitHub release.
- **Catalog / repository index** — the JSON document a Bloem Server fetches to list installable plugins.
- **Materialise** — download and verify every retained release into a static, servable directory.
- **Package** — one catalog entry: manifest plus URLs and checksums.
- **Presentation** — the human-facing block of a plugin manifest (names, summaries, links, licence).
- **Repository dispatch** — a GitHub event another repository sends to trigger the update workflow.
- **Retained plugin** — one of the six the catalog is allowed to contain.
- **Source manifest** — the plugin's own `manifest.json` at the release tag.
- **Static staging tree** — the materialiser's output directory.

## Source References

- `README.md`, `docs/private-staging.md` — purpose, secrets, static hosting recipe
- `manifest.json` — the catalog and its six entries
- `catalog/catalog.go` — types, `BuildPackageFromRelease`, `UpsertPackage`
- `catalog/identity_test.go`, `catalog/automation_test.go`, `catalog/catalog_test.go` — pinned versions, allowlisted repositories, workflow and documentation invariants
- `cmd/update-catalog/main.go` — flags, `GITHUB_TOKEN`, every verification step and message
- `cmd/materialize-private-catalog/main.go` — flags, retained plugins, size limits, atomic publish, canary scan
- `.github/workflows/ci.yml`, `.github/workflows/update-catalog.yml` — triggers, jobs, secrets, guards
- `go.mod`, `NOTICE`, `LICENSE` — module identity, SDK pin, provenance
- `bloem-server/internal/plugins/catalog_service.go`, `installer.go` — how a server reads the index, filters by API version and platform, resolves relative URLs, and verifies checksums
