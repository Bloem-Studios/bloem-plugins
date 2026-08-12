# Private staging catalog

The materializer is a one-shot operator command. It reads the reviewed private
catalog, authenticates GitHub API reads with `GITHUB_TOKEN`, verifies each of
the six retained releases, and atomically replaces a local static tree.

```sh
GITHUB_TOKEN="$VONDEL_CATALOG_SOURCE_TOKEN" \
  GOWORK=off go run ./cmd/materialize-private-catalog \
  -catalog manifest.json \
  -output /srv/vondel-plugin-staging
```

The output contains `catalog.json`, one `checksums.txt` per plugin version, and
the three filename-preserving executable assets. Catalog asset URLs are
relative to the static root. Serve the completed tree only on the intended
private network, for example:

```sh
cd /srv/vondel-plugin-staging
python3 -m http.server 8080 --bind 127.0.0.1
```

Credentials are process-boundary input only. Static output and server
repository or catalog URLs must not contain credentials, bearer tokens, query
tokens, or authenticated GitHub URLs. Do not configure Vondel Server with a
credential-bearing repository URL; point it at the anonymously readable local
`catalog.json` endpoint instead.
