# Bloem Plugin Catalog

Public metadata and official proprietary binary releases from Bloem Studios. Custom plugin source remains private. Shared metadata, autoscan, marker and request plugins come from [Silo’s catalog](https://github.com/Silo-Server/silo-plugins).

## Releases

| Plugin | Version | Install path |
| --- | --- | --- |
| [Pastime](https://github.com/Bloem-Studios/bloem-plugins/releases/tag/pastime-v0.1.12) | 0.1.12 | Plugin catalog; explicit managed-tracking grant and backend setup required |
| [Bookwarehouse](https://github.com/Bloem-Studios/bloem-plugins/releases/tag/bookwarehouse-v0.2.3) | 0.2.3 | Native storage installer; exact-artifact approval required |
| [S3 storage](https://github.com/Bloem-Studios/bloem-plugins/releases/tag/s3-v0.2.1) | 0.2.1 | Native storage installer; exact-artifact approval and source configuration required |
| [Bookwarehouse ebook requests](https://github.com/Bloem-Studios/bloem-plugins/releases/tag/requests-bookwarehouse-v0.1.1) | 0.1.1 | Plugin catalog; connection, organization policy and native ebook library binding required |

S3 supports Linux amd64 and arm64. The other releases also include macOS arm64. Releases contain SHA-256 checksums, proprietary distribution terms and third-party notices. No proprietary plugin source is published here. GitHub’s generated source archives contain only this metadata repository.

[plugins.json](plugins.json) includes all four releases, verified download checksums and installation requirements. [manifest.json](manifest.json) advertises Pastime and Bookwarehouse ebook requests to the ordinary plugin catalog. Native Bookwarehouse storage and S3 have no ordinary plugin capabilities and must use the native StorageProvider installation path; neither is inserted into the ordinary manifest with fabricated capabilities.

Ebook requests are disabled by default. Audiobook requests have no automatic acquisition backend. S3 requires immutable object versions by default; checked ETag reads require explicit opt-in and cannot prove unchanged bytes.

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
