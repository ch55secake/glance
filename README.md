# glance

`glance` is a Go-based GitHub dashboard, intended to keep common work in the
dashboard instead of sending you away to run interactive `gh` commands. The
pull request screen lists open pull requests across repositories visible to the
authenticated account. Open a PR in the TUI to review its description, branches,
reviewers, reviews, checks, and mergeability; merge it without leaving glance.

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
to refresh. Press `enter` on a PR to open its details and automatically load all
changed files and patches in the same continuous, scrollable view. Use up/down
or `j`/`k` to scroll through the details and diff, `PgUp`/`PgDn` to move by a
screen, and `esc` to return to the same inbox position. Diff additions,
deletions, hunks, old/new line numbers, and recognized source code are highlighted;
long lines wrap. PR descriptions render as Markdown, including lists and fenced
code. On wide terminals, mergeability and all checks stay in a right-hand panel;
press `tab` to focus it and use `j`/`k` or `PgUp`/`PgDn` to scroll its checks
independently. On narrow terminals, checks appear below the diff. The merge
action stays visible while scrolling. Press `m` to select an enabled merge
method, then confirm with `y`. glance sends the merge through the GitHub API
using the loaded head SHA and refreshes the dashboard afterwards.
The Issues section lists open issues across accessible repositories with labels
and comment counts. Notifications shows read and unread threads and their
reasons; use up/down or `j`/`k` to navigate either list. Overview shows counts
for PRs, issues, and unread notifications.

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
