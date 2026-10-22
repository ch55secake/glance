# glance

`glance` is a Go-based GitHub dashboard, intended to keep common work in the
dashboard instead of sending you away to run interactive `gh` commands. The
project is at its initial scaffold stage; pull request listing and actions are
still in progress.

## Requirements

- Go 1.26 or newer
- GitHub CLI (`gh`), authenticated with `gh auth login`

## Run

Install the GitHub CLI and authenticate once before starting glance:

```sh
gh auth login
```

glance uses non-interactive `gh api` calls and relies on the CLI's existing
authentication. It does not persist credentials or launch interactive GitHub
commands while you use the TUI.

```sh
go run ./cmd/glance
```

The initial TUI is a placeholder shell. Use the left/right arrow keys or `h`/`l`
to switch sections, and `q` to quit. The Overview checks your GitHub CLI
authentication with `gh api user` and shows the authenticated user; PR listing
and actions are not implemented yet.

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
