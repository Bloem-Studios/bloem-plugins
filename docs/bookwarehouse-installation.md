# Install Bookwarehouse 0.2.2

Download the executable for your server from the [official binary release](https://github.com/Bloem-Studios/bloem-plugins/releases/tag/bookwarehouse-v0.2.2), plus `checksums.txt`, the license and third-party notices. Verify the SHA-256 before making the executable runnable. Linux amd64 additionally includes its extracted manifest and an example approval map.

Bookwarehouse is a native StorageProvider, not an ordinary metadata plugin. Do not install it through the ordinary plugin catalog or invent public capabilities for it.

## Host approval

An operator must review the exact executable, its `manifest` output, SHA-256, OS and architecture. The included example map does not grant approval automatically. Set `BLOEM_NATIVE_STORAGE_APPROVALS` to a protected absolute JSON file containing the reviewed records. The loader expects artifact key → `{manifest, checksum, os, arch}`. Use a regular non-symlink file with mode 0400, owned and readable by the server user or root; protect its parent directories. Restart the server after updating this map.

For Linux arm64 or macOS arm64, extract the manifest by running the downloaded executable's `manifest` command on the matching platform and use its exact manifest and SHA-256 in the operator-reviewed map.

## Install through native storage administration

Obtain a current signed administrative context from `POST /api/bloem/v1/admin/session`. Use `/api/bloem/v1/admin/organization/native-storage` or the corresponding platform scope. `GET /artifacts` lists approvals available to that server process.

Send `POST /installations` with exactly two multipart parts: `binary` containing the raw executable and `request` containing JSON with `artifact_key`, `provider_source_id`, `root_entry_id`, `enabled`, and `config`. The protected connection object belongs under `config.connection`; it contains the Bookwarehouse `base_url`, `api_key`, `source_id` and `source_name`. Keep credentials in backend configuration; never place them in catalog metadata or release files. `provider_source_id` must match the configured source ID.

After installation, create a library referring to the returned native `source_key`, rather than filesystem paths. Installation alone neither creates a library nor scans it. Backend acceptance must verify real listing, metadata, cover and download behavior.

## Ebook cover cache

Configure a dedicated Redis through `BLOEM_STORAGE_COVER_CACHE_URL`. Give that Redis a memory limit and LRU eviction. Do not reuse the main application Redis. The production lib.strm.cafe deployment now uses a separate 512 MiB cache with allkeys-lru, no persistence and no published port. Covers expire after 30 days and can be refetched after eviction.
