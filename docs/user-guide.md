---
title: Bloem Plugin Catalog User Guide
description: How a Bloem Server administrator adds the private plugin catalog, browses it, installs and updates plugins from it — and how a plugin author prepares a release that the catalog updater will accept.
summary: The consumer side (adding a repository, the Catalog tab, installing, updating, uninstalling) and the publisher side (manifest fields, release assets, checksums, tagging, triggering the updater, reading a rejection).
tags:
  - end-user
  - admin
  - plugins
  - developer
  - catalog
audience:
  - operator
  - developer
last_reviewed: 2026-09-05
related:
  - admin-guide.md
  - private-staging.md
---

# Bloem Plugin Catalog User Guide

This guide has two readers.

- **Section 1–2: a Bloem Server administrator** who has been given a catalog URL and wants plugins
  from it to appear on the server's **Admin → Plugins** page.
- **Section 3–5: a plugin author** who maintains one of the six retained plugin repositories and
  wants a new version to reach the catalog.

Running the catalog itself — secrets, workflows, hosting the static tree — is the
[Admin Guide](admin-guide.md).

> **Identifiers.** The catalog repository, its Go module and its secrets keep the pre-rename
> `Vondel` identity, and plugin ids in the manifest proto carry a `silo.` prefix. Where this guide
> shows such a name, it is the literal value the tooling checks, not a typo. The product is Bloem.

---

## 1. What the catalog gives you

A Bloem Server can install plugins three ways: by uploading a binary, by giving it an archive URL,
or from a **catalog** — a JSON index a repository serves, listing plugin versions with download
URLs and SHA-256 checksums. Installing from a catalog means:

- the server shows only entries built for *its* operating system and CPU (`linux/amd64`,
  `linux/arm64` or `darwin/arm64`) and for plugin API `v1`;
- the download is verified against the catalog checksum before anything is installed;
- the *Installed* tab can tell you when a newer version exists in the catalog.

This private catalog lists six plugins: TMDB and TVDB metadata, ebook, audiobook and manga
metadata providers, and the `*arr` autoscan integration (the exact list and versions are in the
Admin Guide, 1.2). Each carries a presentation block — display name, summary, description, setup
notes, links, publisher, licence — which is what the Catalog tab renders.

---

## 2. Using the catalog on a Bloem Server

### 2.1 What you need

The **catalog URL** from the person operating it. It looks like `http://<host>:8080/catalog.json`
and points at the materialised static tree on a private network your server can reach. It must be
an anonymously readable URL with no token in it: the server stores the URL in its database and
fetches it as-is, and the catalog operator's rules forbid credential-bearing URLs.

You cannot use the GitHub repository's raw `manifest.json` URL: the repository is private and the
binary links inside it point at private releases, so the server would get 404s.

### 2.2 Add the repository

1. Sign in as a platform administrator and open **Admin → Plugins**.
2. Scroll to the **Repositories** section.
3. Enter a **Repository name** (any label, e.g. "Bloem private") and the catalog URL, and add it.
   New repositories are enabled by default.
4. The repository row shows when it was last fetched. The managed rows already present ("Managed
   by Silo", the official upstream sources) are separate; the **Include approved community
   plugins** switch controls only those managed rows, not yours.

The server fetches every enabled repository when you open the Catalog tab; a repository it cannot
fetch is skipped with a log line `skipping broken plugin repository`, and the others still appear.

### 2.3 Browse and install

Open the **Catalog** tab (the page remembers it as `?tab=catalog`). Use **Search the plugin
catalog** to filter. Each entry shows its display name, version, summary, publisher and links from
the presentation block; the repository name tells you which source it came from. Entries whose
platform or API version do not match your server are simply not shown.

Choose **Install** on an entry. The server:

1. resolves the binary URL for your platform against the repository URL (relative URLs in the
   static tree become absolute);
2. downloads the binary and computes its SHA-256;
3. compares it with the catalog's checksum for that platform (falling back to the release's
   `checksums.txt` if the entry carried none) — a mismatch aborts with `binary checksum mismatch`;
4. records the installation, enabled, with its repository id.

The plugin then appears on the **Installed** tab. Enable, disable, configure, bind and uninstall
work exactly as for any other installed plugin; the catalog only changes where the binary came
from.

### 2.4 Updates

Each installation has an **update policy**. With a catalog-backed installation the server can see
the catalog version for the same plugin id, show that an update is available on the Installed tab,
and — depending on the policy — install it. Because this catalog lists exactly one version per
plugin, "the catalog version" is always the single reviewed release; rolling back means the
catalog operator republishing the older tag (Admin Guide, 4.3), after which the server sees *that*
as the current version.

### 2.5 Removing

Uninstall from the Installed tab as usual. Removing the repository row stops future catalog fetches
from it but does not touch plugins already installed.

### 2.6 If something is wrong

| What you see | What to try |
|---|---|
| The repository row exists but the Catalog tab shows nothing from it | The URL is unreachable from the server, returns a non-200 status, or the JSON has no entry for your OS/arch. Fetch it with `curl` *from the server host* and check the log for `skipping broken plugin repository`. |
| `plugin … does not support platform …` on install | The catalog has no binary for your platform. The six retained plugins all ship three platforms, so this usually means a stale or hand-edited tree. Ask the operator to re-materialise. |
| `binary checksum mismatch` | The served binary does not match the catalog. Do not retry blindly; tell the catalog operator, who re-materialises from verified releases. |
| `plugin silo_api_version "…" is not supported` | The entry targets a different plugin API than this server. This catalog only lists `v1`. |
| An update never appears | The catalog still lists the version you have; check with the operator whether the release was published into it. |

---

## 3. Publishing a plugin version (author side)

The catalog accepts a release only from the six allowlisted repositories, and only if the release
passes every check in `cmd/update-catalog`. This section is the checklist that makes a release
pass first time. Terms: your **source manifest** is the `manifest.json` at the root of your
repository; the **tag** is the git tag of the release.

### 3.1 Version and tag

- Set `version` in your manifest to a plain `MAJOR.MINOR.PATCH` (the materialiser rejects
  prerelease suffixes).
- Tag the commit as `v` + that version, exactly: manifest `1.2.24` → tag `v1.2.24`. The updater
  checks `tag == "v" + version` and refuses otherwise.
- Publish the release as a normal, **non-draft, non-prerelease** release. Drafts and prereleases
  are rejected, and so is a release that has no `published_at`.

### 3.2 The source manifest

Commit `manifest.json` at the repository root so it exists at the tag. It is the plugin SDK's
`PluginManifest` in protojson. Required by the catalog tooling:

| Field | Requirement |
|---|---|
| `plugin_id` | Non-empty; must equal the id the materialiser expects for your repository (e.g. `silo.tmdb`). |
| `version` | Equals the tag without `v`. |
| `silo_api_version` | Exactly `v1`. |
| `supported_platforms` | Exactly three entries: `{os: darwin, arch: arm64}`, `{os: linux, arch: amd64}`, `{os: linux, arch: arm64}`, no duplicates. |
| `capabilities` | At least one; each with non-empty `type` and `id`. |
| `presentation` | Every field the SDK's `ValidateCatalogPresentation` requires, validated against your repository URL: `display_name`, `summary`, `description_markdown`, `setup_markdown`, `homepage_url`, `source_url`, `support_url`, `changelog_url`, `publisher_name`, `publisher_url`, `license_spdx`. The identity test additionally expects `source_url` and `homepage_url` to be the repository URL, `publisher_name` `Vondel` and `publisher_url` `https://github.com/Vondel-Media`. |
| `checksum` | May be the placeholder `__CHECKSUM__` in source; the catalog blanks it and uses per-platform release checksums instead. |

The binary itself must print a manifest whose `plugin_id`, `version`, `silo_api_version`,
`presentation.source_url` and `presentation.publisher_url` equal the source manifest, and whose
`checksum` equals the binary's own SHA-256 (the SDK stamps this at start-up). The updater runs
`plugin-<os>-<arch> manifest` on a matching platform to check this; the GitHub runner is
`linux/amd64`, so the Linux binary is always checked.

### 3.3 Release assets

Exactly four assets, named exactly:

```
checksums.txt
plugin-darwin-arm64
plugin-linux-amd64
plugin-linux-arm64
```

Anything else — an extra `.sha256`, a source tarball you attached yourself, a differently cased
name — fails `release must contain exactly four assets`. The three binaries are bare executables,
not archives.

`checksums.txt` must contain exactly three lines, one per binary, in `sha256sum` format:

```
<64 hex chars>  plugin-darwin-arm64
<64 hex chars>  plugin-linux-amd64
<64 hex chars>  plugin-linux-arm64
```

A leading `*` before the name (binary-mode `sha256sum`) is tolerated; a directory path is not.
Generate it from the *final* binaries — after any stripping, signing or rebuild — with:

```sh
sha256sum plugin-darwin-arm64 plugin-linux-amd64 plugin-linux-arm64 > checksums.txt
```

Upload the binaries and `checksums.txt` to the release. Their download URLs must be GitHub's own
(`https://github.com/<owner>/<name>/releases/download/<tag>/<asset>`), which is automatic when you
upload through GitHub.

### 3.4 Trigger the catalog update

Either:

- from your repository's release workflow, send a `repository_dispatch` to the catalog repository
  with `event_type: plugin_release_published` and
  `client_payload: {"repo": "<owner>/<name>", "tag": "<tag>"}` (this needs a token with access to
  the catalog repository; keep it in your repository's secrets, never in the workflow file); or
- ask the catalog operator to run **Actions → Update Catalog → Run workflow** with your `repo` and
  `tag`.

Updates for different plugins queue behind one another; a run takes a few minutes. Success is a
commit on the catalog's `main` titled `chore: update catalog for <repo>@<tag>`.

### 3.5 Reading a rejection

The updater exits with one line naming the check. The most common:

| Message | Fix |
|---|---|
| `fetch release: GitHub API request returned status 404` | Tag not found or release is a draft. |
| `fetch source manifest: … status 404` | `manifest.json` missing at the repository root at that tag. |
| `validate release assets: release must contain exactly four assets` | Remove extras / add the missing one. |
| `verify release assets: binary checksum mismatch for plugin-linux-amd64` | `checksums.txt` was generated from an earlier build. Regenerate and replace the asset. |
| `validate source identity: source manifest version does not exactly match release tag` | Bump `version` in the manifest, commit, retag. |
| `validate native binary manifest: native manifest identity or checksum does not match source and release` | The Linux binary prints a different id/version/URLs, or does not stamp its checksum — rebuild with the current SDK. |
| `build catalog package: source manifest presentation: …` | A presentation field is missing or its URL is not acceptable for the repository. |
| `build catalog package: release "…" is a prerelease` | Untick "pre-release" on the GitHub release. |

Fix the release **in place** (replace assets) when the manifest and tag are right; retag when the
manifest must change. Re-running the updater with the same repo and tag is idempotent: it replaces
your catalog entry with the corrected release.

### 3.6 What happens to your entry

Your plugin id gets exactly one entry in `manifest.json`, replaced wholesale on every publish:
manifest (version set from the tag, `checksum` blanked), `repo_url`, `checksums_url`, and three
`binaries` entries each with URL and SHA-256. Older versions are not kept in the catalog; they stay
as releases in your repository. The catalog operator then updates the pinned version in the
catalog's identity test and, if a static tree is served, re-materialises it — until that happens
servers reading the static tree still see the previous version.

---

## 4. Developer notes: the catalog library

If you are writing tooling around the catalog, `catalog/catalog.go` is the public surface:

```go
import "github.com/Vondel-Media/vondel-plugins/catalog"

index, _ := catalog.RepositoryIndex{}, error(nil)     // decode manifest.json into this
source, err := catalog.DecodeSourceManifest(manifestJSON)   // protojson → *PluginManifest, unknown fields discarded
pkg, err := catalog.BuildPackageFromRelease("Vondel-Media/vondel-plugin-tmdb", source, release)
index = catalog.UpsertPackage(index, pkg)              // replace-by-plugin-id, sorted
```

Types: `Release{TagName, Draft, Prerelease, PublishedAt, Assets []Asset}` mirrors the GitHub
release API; `Asset{ID, Name, URL, BrowserDownloadURL}`; `CatalogPackage{Manifest, RepoURL,
ChecksumsURL, Binaries map["os/arch"]PlatformBinary}`; `PlatformBinary{URL, Checksum}`.
`BuildPackageFromRelease` does not download anything and does not fill checksums — that is the
updater's job after it has verified `checksums.txt`. The module depends on
`github.com/Vondel-Media/vondel-plugin-sdk v0.13.3` for the manifest proto and presentation
validator; the SDK is private, so set `GOPRIVATE=github.com/Vondel-Media/*`.

Platform keys are derived from asset names by `platformKeyFromAssetName`: `plugin-linux-amd64` →
`linux/amd64`; any other shape is rejected as an unsupported platform binary.

---

## 5. Frequently asked questions

**Can the catalog hold two versions of my plugin?** No. One entry per plugin id; publishing
replaces it.

**Can I add a seventh plugin?** Not without a code change in the catalog repository: both
allowlists and the identity test are literal. Ask the operator.

**Is the catalog signed?** No. Integrity rests on GitHub authentication for the sources and on
SHA-256 checksums verified by the updater, the materialiser and the installing server.

**Why does my server show "Managed by Silo" repositories I never added?** Those are the server's
built-in managed sources; they are unrelated to this private catalog and are controlled by the
"Include approved community plugins" switch.

**Do servers need GitHub access?** No. Servers read the materialised static tree, which contains
the binaries themselves; GitHub is only touched by the catalog tooling.

---

## Glossary

- **Catalog tab** — the part of a Bloem Server's Plugins page that lists installable entries from repositories.
- **Checksums file** — `checksums.txt`, three `sha256sum`-style lines, one per binary.
- **Installed tab** — the list of plugins already on the server, with update status.
- **Presentation** — the human-facing fields of a plugin manifest shown on the Catalog tab.
- **Release** — a GitHub release: a tag plus attached assets.
- **Repository (server side)** — a catalog URL a Bloem Server fetches.
- **Retained plugin** — one of the six plugins this catalog is allowed to list.
- **Static tree** — the served directory containing `catalog.json` and the binaries.
- **Update policy** — the per-installation setting that governs whether catalog updates are applied automatically.

## Source References

- `cmd/update-catalog/main.go` — every acceptance check, exact asset names, checksum format, native manifest check, error messages
- `catalog/catalog.go` — `BuildPackageFromRelease`, `UpsertPackage`, platform key derivation, package shape
- `catalog/identity_test.go` — expected presentation values and pinned versions
- `cmd/materialize-private-catalog/main.go` — stable-version rule, relative URLs in the static tree
- `.github/workflows/update-catalog.yml` — `plugin_release_published` payload, manual inputs, commit message
- `docs/private-staging.md`, `README.md` — no credential-bearing URLs; not a public endpoint
- `bloem-server/internal/plugins/catalog_service.go`, `catalog_settings.go`, `installer.go`, `internal/api/handlers/plugins.go`, `web/src/pages/AdminPlugins.tsx` — the server-side Repositories section, Catalog/Installed tabs, platform and API filtering, checksum verification, update policy
