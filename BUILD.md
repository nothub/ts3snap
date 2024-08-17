# Build

## Local

To build binaries, run:

```sh
goreleaser build --clean --snapshot
```

To build a local snapshot release, run:

```sh
goreleaser release --clean --snapshot
```

## Publish

To build and publish a full release, push a semver tag (with `v` prefix) to the `trunk` branch on GitHub.
