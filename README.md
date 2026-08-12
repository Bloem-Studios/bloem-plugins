# Vondel Plugin Catalog

Private staging catalog metadata and update tooling for Vondel-owned plugins.
The catalog starts empty and is populated only from allowlisted private Vondel
releases after their source forks and release artifacts have passed review.

This repository is not a public catalog endpoint. Vondel Server public defaults
must not point here while the repository and its release sources require
authentication.

## Catalog updates

Vondel plugin repositories may dispatch `plugin_release_published` with an
allowlisted `repo` and exact release `tag`. The updater rejects drafts,
prereleases, incomplete platform assets, duplicate assets, and repositories
outside its literal Vondel allowlist.

Automation uses these private repository secrets:

- `VONDEL_MODULES_TOKEN` reads the private Vondel SDK for Go builds.
- `VONDEL_CATALOG_SOURCE_TOKEN` reads private plugin releases and tagged
  manifests.
- `VONDEL_CATALOG_PUSH_TOKEN` pushes reviewed catalog updates to this private
  repository.

No token belongs in source, catalog data, release assets, or server binaries.

## License and provenance

This fork retains the upstream Apache-2.0 license. See [LICENSE](LICENSE) and
[NOTICE](NOTICE) for attribution and the exact imported revision.
