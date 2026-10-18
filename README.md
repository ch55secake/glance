# glance

`glance` is a Go-based GitHub dashboard, intended to keep common work in the
dashboard instead of sending you away to run interactive `gh` commands. The
project is at its initial scaffold stage; pull request listing and actions are
still in progress.

## Requirements

- Go 1.26 or newer

## Run

Set a GitHub personal access token in `GLANCE_GITHUB_TOKEN`:

```sh
export GLANCE_GITHUB_TOKEN="<your-token>"
```

Use a fine-grained token scoped to the repositories glance should access, with
only the permissions needed for those repositories.
The token is held in process memory and is not written to disk or logged. Do not
commit it to the repository.

```sh
go run ./cmd/glance
```

The initial TUI is a placeholder shell. Use the left/right arrow keys or `h`/`l`
to switch sections, and `q` to quit. The Overview checks the token with the
GitHub API and shows the authenticated user; PR listing and actions are not
implemented yet. glance does not invoke `gh`.

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

GitHub Actions reuses the shared Go build/test, lint, and vulnerability
workflows from `ch55secake/cheesecake-factory`. A shared release workflow
publishes builds on pushes to `main` and can also be run manually.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for the local development workflow.
