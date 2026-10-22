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
make run
```

Use left/right or `h`/`l` to switch sections; press `?` for the full shortcut
list and `q` to quit. The Pull requests section loads open PRs from repositories
you own, collaborate on, or can access through an organization. Navigate with
up/down or `j`/`k`, search with `/`, and filter with `a` (all), `m` (authored by
you), or `d` (drafts). Press `esc` to clear the active search and filter, or `r`
to refresh. The Overview shows the authenticated user and PR counts. PR details
and actions are not yet available.

Build the executable with:

```sh
make build
```

## Checks

```sh
make check
```

`make lint` runs the pinned golangci-lint version, and `make test-race` runs the
race-enabled test suite.

GitHub Actions reuses the shared Go build/test, lint, and vulnerability
workflows from `ch55secake/cheesecake-factory`. A shared release workflow
publishes builds on pushes to `main` and can also be run manually.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for the local development workflow.
