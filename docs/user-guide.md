# Using plugin catalogs in Bloem

Use Bloem's existing **Silo maintained** catalog for ordinary Silo plugins. Its
published binaries use the same v1 plugin protocol. Enable the approved-community
channel where a plugin has moved there. Runtime compatibility does not imply
identical configuration or provider behavior between an older fork and a newer
upstream release.

For custom Bloem plugins, the public metadata repository is:

https://github.com/Bloem-Studios/bloem-plugins

The optional generic feed URL is:

https://raw.githubusercontent.com/Bloem-Studios/bloem-plugins/main/manifest.json

It is currently empty. The separate `plugins.json` inventory describes custom
source projects and prerequisites; it is not an installer feed.

- **Bookwarehouse:** native EPUB/PDF StorageProvider. Read the plugin's native
  installation documentation and approve the exact packaged artifact before use.
- **Pastime:** household profile tracking. Complete the matching server/backend
  integration and enrollment acceptance before activation.

Catalog publication does not change live installed plugins or customer accounts.
Keep secrets in protected backend configuration, never catalog metadata.
