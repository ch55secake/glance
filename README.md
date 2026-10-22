# glance

`glance` is a Go-based GitHub dashboard, intended to keep common work in the
dashboard instead of sending you away to run interactive `gh` commands. The
pull request screen lists open pull requests across repositories visible to the
authenticated account. Pull request details and actions are still in progress.

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

Use the left/right arrow keys or `h`/`l` to switch sections, and `q` to quit.
The Pull requests section loads open PRs from repositories you own, collaborate
on, or can access through an organization. Use up/down or `j`/`k` to move through
the list, and `r` to refresh. The Overview shows the authenticated user and the
number of loaded pull requests.

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
