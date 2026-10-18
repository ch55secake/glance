# Contributing

Contributions are welcome. For substantial changes, open an issue or discussion
first to agree on the approach.

## Development checks

Before opening a pull request, run:

```sh
gofmt -w .
go test ./...
go vet ./...
go build ./...
```

Pull requests should explain the user-facing change and include tests where
appropriate. Keep changes focused and avoid adding dependencies without a clear
need.
