# Bloem Plugin Catalog

The private plugin catalog for Bloem Server, and the tooling that keeps it honest. The catalog is
a single JSON file, `manifest.json`, that lists which plugin versions a Bloem Server may install:
each entry carries the plugin's manifest, its source repository, a `checksums.txt` URL and
per-platform binary URLs with SHA-256 checksums. Alongside it live two commands — an **updater**
that adds a reviewed GitHub release to the catalog, and a **materialiser** that turns the catalog
into a static directory you can serve on a private network. It is for the operator who publishes
plugin releases and serves the catalog, the Bloem Server administrator who installs from it, and
the plugin author who wants a release accepted.

The catalog starts empty and is populated only from allowlisted private releases after their
source repositories and release artifacts have passed review. This repository is not a public
catalog endpoint: Bloem Server public defaults must not point here while the repository and its
release sources require authentication.

> **Naming.** This repository predates the Bloem rename and still carries its earlier identity in
> real identifiers: the Go module is `github.com/Vondel-Media/vondel-plugins`, the secrets are
> `VONDEL_*`, the allowlisted plugin repositories live under the `Vondel-Media` GitHub
> organisation, and plugin ids carry the `silo.` prefix from the SDK's manifest proto. Tests and
> the updater's allowlist enforce those names, so they appear verbatim below. The product is Bloem.

## Features

**The catalog**

- One reviewed, committed `manifest.json` in the `RepositoryIndex` shape a Bloem Server fetches: manifest, `repo_url`, `checksums_url` and three `binaries` entries per plugin.
- Exactly six retained plugins, one version each: `silo.tmdb`, `silo.tvdb`, `silo.ebook-metadata`, `silo.audiobook-metadata`, `silo.manga-metadata`, `silo.autoscan.arr`; the list is literal in code and pinned by `catalog/identity_test.go`.
- Every entry advertises `darwin/arm64`, `linux/amd64` and `linux/arm64` and plugin API `v1`; a server shows only entries matching its own platform and API version.
- A complete presentation block per plugin (display name, summary, description, setup notes, links, publisher, licence) — what the server's Catalog tab renders.

**The updater (`cmd/update-catalog`)**

- Adds or replaces one plugin's entry from a GitHub release tag, reading the release and the source `manifest.json` at that tag through the GitHub API.
- Accepts only the six allowlisted repositories and only non-draft, non-prerelease, published releases.
- Requires exactly four assets (`checksums.txt` plus the three `plugin-<os>-<arch>` binaries) with GitHub's own download URLs, downloads all of them, and verifies every binary against `checksums.txt`.
- Checks the source manifest: version equals the tag without `v`, API version `v1`, exactly three platforms, capabilities with type and id, and a presentation block that passes the SDK's `ValidateCatalogPresentation` against the repository URL.
- On a matching platform runs the binary's `manifest` subcommand and compares plugin id, version, API version, checksum, source URL and publisher URL with the source manifest.
- Replaces the entry wholesale, sorts by plugin id, and writes deterministic JSON; the workflow runs it twice and requires byte-identical output.
- Strips the token from any redirect an asset download follows.

**The materialiser (`cmd/materialize-private-catalog`)**

- Downloads every retained release again, re-verifies checksums against fresh `checksums.txt` content, and requires them to equal what the catalog recorded.
- Writes a static tree — `catalog.json` plus `plugins/<id>/<version>/{checksums.txt,plugin-*}` — with URLs relative to the static root, which a Bloem Server resolves against the repository URL it was given.
- Atomic publish: builds into a hidden sibling directory and renames into place only on complete success; a failed run leaves the previous tree untouched.
- Refuses a symlink or non-directory output, a non-regular catalog file, a catalog of any size other than six, prerelease version suffixes, and mismatched repository or plugin ids.
- Scans its whole output for the token it was given and fails on a hit ("credential canary detected in staging output"); download caps of 512 MiB per binary and 1 MiB for `checksums.txt`.

**Automation and trust**

- `Update Catalog` workflow triggered by `repository_dispatch` (`plugin_release_published` with `{"repo", "tag"}`) from a plugin repository, or manually from the Actions tab; runs queue and never race.
- Three repository secrets with one job each: `VONDEL_MODULES_TOKEN` (private SDK module), `VONDEL_CATALOG_SOURCE_TOKEN` (private releases and manifests), `VONDEL_CATALOG_PUSH_TOKEN` (push to `main`); a test asserts each appears exactly once, in the step that needs it and no earlier.
- The SDK module is prefetched into a sanitised cache in a job that never checks this repository out, so the module token never coexists with a checkout.
- CI guards against `replace` directives, upstream dispatch endpoints, visibility changes, `npm publish`, `docker push` and Pages publishes.
- No cryptographic signing: trust rests on GitHub authentication, the allowlists, and SHA-256 checksums verified three times (updater, materialiser, installing server). No token belongs in source, catalog data, release assets or server binaries.
- Rollback is an ordinary publish of the previous tag; the catalog never lists two versions of one plugin.

## Quick start

**Add the catalog to a Bloem Server.** You need the catalog URL from the person operating it,
typically `http://<host>:8080/catalog.json` on a private network — the materialised static tree,
not the GitHub `manifest.json`, which the server cannot authenticate to. Sign in as a platform
administrator, open **Admin → Plugins**, scroll to **Repositories**, enter a name and the URL, and
add it. Open the **Catalog** tab, pick an entry and choose **Install**; the server downloads the
binary for its platform, verifies the checksum and records the installation. The plugin then shows
on the **Installed** tab like any other, including update detection against the catalog.

**Publish a plugin version.** In the plugin repository: set `version` in `manifest.json` to a plain
`MAJOR.MINOR.PATCH`, tag the commit `v` + that version, build the three binaries and generate the
checksums file from the final files:

    sha256sum plugin-darwin-arm64 plugin-linux-amd64 plugin-linux-arm64 > checksums.txt

Publish a normal (non-draft, non-prerelease) GitHub release with exactly those four assets. Then
either let the plugin repository's release workflow send a `repository_dispatch` of type
`plugin_release_published` with `{"repo": "<owner>/<name>", "tag": "<tag>"}`, or run
**Actions → Update Catalog → Run workflow** here with `repo` and `tag`. Success is a commit on
`main` titled `chore: update catalog for <repo>@<tag>`. Afterwards bump the pinned version in
`catalog/identity_test.go` and, where a static tree is served, re-materialise it:

    GITHUB_TOKEN="$VONDEL_CATALOG_SOURCE_TOKEN" \
      GOWORK=off go run ./cmd/materialize-private-catalog \
      -catalog manifest.json \
      -output /srv/vondel-plugin-staging

Serve the output on the private network only, for example `python3 -m http.server 8080 --bind
127.0.0.1` from inside the directory.

**Local development.** Go 1.26, `GOPRIVATE=github.com/Vondel-Media/*` with Git credentials for that
organisation, and `GOWORK=off go test ./...`; the tests run against fake HTTP servers and need no
network.

## Documentation

- [docs/admin-guide.md](docs/admin-guide.md) — for the catalog operator: secrets, the two workflows, publishing a release, hosting the static tree, the full command reference, retention and troubleshooting.
- [docs/user-guide.md](docs/user-guide.md) — for Bloem Server administrators and plugin authors: adding the repository, browsing and installing, and the release checklist that makes the updater accept a version.
- [docs/private-staging.md](docs/private-staging.md) — the one-shot materialiser recipe and the no-credentials rule for served URLs.

## License and provenance

This fork retains the upstream Apache-2.0 license. See [LICENSE](LICENSE) and
[NOTICE](NOTICE) for attribution and the exact imported revision.
