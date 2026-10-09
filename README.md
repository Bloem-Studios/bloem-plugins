# Bloem Plugin Catalog

Public metadata and official proprietary binary releases from Bloem Studios. Custom plugin source remains private. Shared metadata, autoscan, marker and request plugins come from [Silo’s catalog](https://github.com/Silo-Server/silo-plugins).

## Releases

| Plugin | Version | Install path |
| --- | --- | --- |
| [Pastime](https://github.com/Bloem-Studios/bloem-plugins/releases/tag/pastime-v0.1.2) | 0.1.2 | Plugin catalog; explicit managed-tracking grant and backend setup required |
| [Bookwarehouse](https://github.com/Bloem-Studios/bloem-plugins/releases/tag/bookwarehouse-v0.2.2) | 0.2.2 | Native storage installer; exact-artifact approval required |

Each release includes Linux amd64, Linux arm64 and macOS arm64 executables, SHA-256 checksums, proprietary distribution terms and third-party notices. No proprietary plugin source is published here. GitHub’s generated source archives contain only this metadata repository.

[plugins.json](plugins.json) includes both releases, verified download checksums and installation requirements. [manifest.json](manifest.json) advertises Pastime to the ordinary plugin catalog. Bookwarehouse has no ordinary plugin capabilities and must use the native StorageProvider installation path; it is not inserted into the ordinary manifest with fabricated capabilities.

Feed URL:

```text
https://raw.githubusercontent.com/Bloem-Studios/bloem-plugins/main/manifest.json
```

Pastime installation alone does not enroll users. The matching host extension, backend-only provisioning configuration and explicit account/profile grants are required. Bookwarehouse requires native host approval matching the downloaded executable’s complete SHA-256 and manifest. The approval file in its release is an example, not an active grant.

See [Bookwarehouse installation](docs/bookwarehouse-installation.md) for native approval and cover cache setup.

## Development

The catalog code remains Apache-2.0. The proprietary license applies to separately packaged custom plugin binaries; third-party components retain their own licenses. Previously published open-source versions retain their licenses.

Go 1.26 or newer:

```sh
GOWORK=off go test -race ./...
GOWORK=off go vet ./...
GOWORK=off go build ./...
```

The existing generic release updater is restricted to an empty source-repository allowlist. Binary releases hosted here are maintained explicitly with verified checksums; this change does not grant automation access to private plugin source.
