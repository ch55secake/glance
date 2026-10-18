# glance

`glance` is a Go-based GitHub dashboard, intended to keep common work in the
dashboard instead of sending you away to run interactive `gh` commands. The
project is at its initial scaffold stage; product features are not implemented
yet.

## Requirements

- Go 1.26 or newer

## Run

```sh
go run ./cmd/glance
```

The initial TUI is a placeholder shell. Use the left/right arrow keys or `h`/`l`
to switch sections, and `q` to quit. GitHub data loading is not implemented yet.

Build the executable with:

```sh
go build -o bin/glance ./cmd/glance
```

## Checks

```sh
gofmt -w .
go test ./...
go vet ./...
```

The GitHub Actions workflow runs formatting, tests, vet, and build checks on
pull requests to `main` and pushes to `main`.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for the local development workflow.
