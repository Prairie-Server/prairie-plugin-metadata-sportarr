# Prairie Sportarr Plugin

First-party Prairie metadata plugin backed by [Sportarr](https://sportarr.net). Provides sports league metadata as TV shows, with series, seasons, and episodes mapped from leagues, seasons, and events.

## Dependency Model

This repository consumes `github.com/prairie-server/prairie-plugin-sdk` as a normal Go module dependency. CI and release builds run with `GOWORK=off` and resolve the SDK version pinned in `go.mod` (a release tag or a pseudo-version of the SDK's `main` branch) without local overrides.

For local multi-repo development, use a temporary `replace` or a local `go.work` that points at `dev/github/prairie-plugin-sdk`. Do not commit machine-local filesystem replaces as the supported release path.

## Development

```sh
GOWORK=off go test ./...
GOWORK=off go build .
golangci-lint run ./...
GOWORK=off go test ./... -count=1 -covermode=atomic -coverprofile=coverage.out
./scripts/check-coverage.sh coverage.out
```

CI runs golangci-lint v2.14.0 and enforces a 95% statement coverage floor
(`scripts/check-coverage.sh`); the last three commands reproduce those checks.

## Attribution

Metadata provided by [Sportarr](https://sportarr.net).

## License

`prairie-plugin-metadata-sportarr` is licensed under `AGPL-3.0-or-later`. See [LICENSE](LICENSE).
