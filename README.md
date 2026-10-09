# Bloem Plugin Catalog

Public metadata and future binary releases for plugins maintained by [Bloem Studios](https://github.com/Bloem-Studios).
Use [Silo's maintained catalog](https://github.com/Silo-Server/silo-plugins) for
shared metadata, autoscan, markers, request routing and Floppy plugins. Bloem
already uses that catalog by default; this repository does not duplicate its
plugin IDs or release binaries.

## Custom plugins

| Plugin | Source version | SDK version | Installation status |
| --- | --- | --- | --- |
| [Bookwarehouse](https://github.com/Bloem-Studios/bloem-plugin-bookwarehouse) | 0.2.2 | 0.26.0 | Native StorageProvider; requires exact-artifact host approval |
| [Pastime](https://github.com/Bloem-Studios/bloem-plugin-pastime) | 0.1.0 | 0.26.0 | Native managed tracking; complete host/backend integration pending |

[plugins.json](plugins.json) is a source inventory with explicit prerequisites.
Its `source_version` describes the plugin source, and `sdk_version` describes
the SDK it builds against. Neither field promises a published release or a
production installation. Native entries must not be passed to the generic
plugin installer.

[manifest.json](manifest.json) is the compatible `RepositoryIndex` feed that
Bloem's generic catalog installer consumes. It intentionally contains an empty
`plugins` array until there is a reviewed, supported custom release. No release
assets, checksum values or installation readiness are fabricated.

The public feed URL is:

```text
https://raw.githubusercontent.com/Bloem-Studios/bloem-plugins/main/manifest.json
```

Adding it as an extra repository leaves Silo's catalog available. It will not
install Bookwarehouse or activate Pastime; follow their native integration docs.

## Release tooling

`cmd/update-catalog` preserves source/tag identity validation, presentation checks,
release asset URL allowlisting, binary SHA-256 verification and deterministic
updates for future generic-installer releases. Its repository allowlist is
currently empty: neither custom plugin is approved for that path. A reviewed
change must add a supported repository before release automation can accept it.
Old duplicate Silo repositories cannot re-enter this catalog through a dispatch.

Implementation repositories are private. Official binary releases and metadata are published from this public catalog repository; source repository URLs in the inventory require authorized access. No custom binary release has been published here yet. A published,
non-draft, non-prerelease ordinary plugin release must contain `checksums.txt`
and exactly three binaries: `plugin-darwin-arm64`, `plugin-linux-amd64`, and
`plugin-linux-arm64`. The updater runs the matching-platform binary's `manifest`
command and verifies its identity against the tagged source manifest.

The update workflow accepts `plugin_release_published` dispatches or manual
`repo`/`tag` inputs. Public source reads use the workflow's read-only GitHub token;
only the final commit/push step receives `BLOEM_CATALOG_PUSH_TOKEN`. This token
must be scoped to this catalog repository. Publication and repository visibility
changes are never automated by these workflows.

## Development

Go 1.26 or newer; the SDK dependency is the public released tag v0.26.0. No
private-module or source-read credentials are required.

```sh
GOWORK=off go test -race ./...
GOWORK=off go vet ./...
GOWORK=off go build ./...
```

See [the operator guide](docs/admin-guide.md) and
[the server-user guide](docs/user-guide.md).

## License and provenance

The catalog retains its Apache-2.0 license. See [LICENSE](LICENSE) and
[NOTICE](NOTICE). Bookwarehouse and Pastime source repositories are private. Future official binaries use proprietary distribution terms (`LicenseRef-Bloem-Proprietary`); accompanying notices preserve dependency licenses. Earlier publicly licensed revisions retain their licenses. The public SDK retains Apache-2.0. Bloem is
independent of Silo; retaining Silo protocol identifiers preserves compatibility.
