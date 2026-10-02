# Contributing

[← Back to the README](../README.md) · [Documentation index](../README.md#documentation)

Bug reports and pull requests are welcome at [wahh-22/nu11signal](https://github.com/wahh-22/nu11signal).

## Development

Try the UI without Apple Music or signing, then run the checks a pull request
should pass:

```sh
make demo                       # the UI against a simulated player
make test                       # go test -race, swift test in helper/, script tests
make vet && make fmt-check      # go vet; gofmt -l . must list nothing
```

Building the real helper needs the one-time signing setup in
[Building from source](building.md).

The UI's views are pinned by golden files in `internal/radio/testdata`. The
emblem icon and the screenshots under `docs/assets/` are rendered from the
in-app emblem and those goldens; after changing either, regenerate them:

```sh
go run ./tools/readmeart
```

`go test ./tools/readmeart` fails while a committed SVG is stale.

## Repository notes

- `spike/` is the historical proof of concept (authorize, search, play from a
  signed windowless app). It is kept for reference and not used by the build.
- License: MIT — see [LICENSE](../LICENSE).
