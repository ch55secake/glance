# glance

`glance` is a Go-based GitHub dashboard, intended to keep common work in the
dashboard instead of sending you away to run interactive `gh` commands. The
project is at its initial scaffold stage; product features are not implemented
yet.

## Requirements

- Go 1.26 or newer

## Build and run

```sh
go run ./cmd/glance
```

Build all packages with:

```sh
go build ./...
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
