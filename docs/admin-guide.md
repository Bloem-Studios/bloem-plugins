# Catalog maintenance

Keep this catalog limited to custom Bloem plugins. Shared Silo releases belong
in Silo's existing catalog and should be consumed directly.

Maintain `plugins.json` as the custom source inventory: stable plugin ID, public
repository, source version, SDK version, installation mode, status and prerequisites.
Keep it separate from the installable `manifest.json` feed. Neither Bookwarehouse
nor Pastime currently has an approved generic-installer release.

Bookwarehouse uses native StorageProvider admission with an approval that pins
the exact artifact version and SHA-256. Pastime requires the matching managed-tracking
host extension, enrollment grants and completed native tracker endpoints. Publishing
source is not production activation.

For a future generic plugin, review its host compatibility, presentation,
platforms and artifact packaging first. Update the updater's repository allowlist
and tests deliberately. Then use a stable published release with exactly three
`plugin-<os>-<arch>` binaries plus `checksums.txt`, and dispatch the source
repository/tag to the Update Catalog workflow. The workflow verifies every
binary, checks native manifest identity, checks deterministic output, runs tests
and commits only `manifest.json`.

CI uses public modules without credentials. The updater uses a read-only GitHub
token for public API reads. Configure `BLOEM_CATALOG_PUSH_TOKEN` only for the final
catalog push, with access limited to this repository. Never put tokens in source,
URLs, metadata, binaries or logs. No private static mirror is needed for public
GitHub catalog and release URLs.

Silo and Bloem fork release numbers are independent. A larger fork version can
still be based on older upstream code; compare source and behavior when deciding
whether to adopt a release. Removing a fork does not migrate an existing installed
plugin, its configuration, its database repository association or its binary.
Verify those separately during an actual server rollout.

## Binary distribution

Bookwarehouse and Pastime implementation repositories are private. Publish future
reviewed binaries from this public catalog repository, with exact checksums,
manifest/version identity, proprietary binary terms and all required dependency
license/source notices. The inventory uses `LicenseRef-Bloem-Proprietary`; it does
not promise available release assets. Earlier Apache/AGPL source revisions retain
their published terms. Bookwarehouse native approval and Pastime integration
acceptance remain separate release gates. Do not publish private source, credentials
or unfinished native integrations as install-ready releases.
