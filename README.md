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
list and `q` to quit. Press `:` for a command palette with navigation and common
actions. glance opens directly to Pull requests, searching repositories visible
to your GitHub account. The PR inbox defaults to the
GitHub filter `is:pr is:open author:@me`. Press `/` to edit it using GitHub search qualifiers
(for example, `repo:owner/name is:open`), `enter` to apply it, or `esc` to
cancel. While editing, use left/right or Home/End to move the cursor through
long queries; Backspace/Delete edit at the cursor. Press up/down or `j`/`k` to
browse results. On wider terminals,
each inbox has a compact selection rail beside a preview of the selected item;
on narrow terminals, items appear in a single column. The selected PR's
description (with line breaks), mergeability, checks, and review summary load
after a short pause and are cached while browsing. Press `r` to refresh the
current filter. Press `enter` on a PR to open its details and automatically
load all changed files and patches in the same continuous, scrollable view.
Use up/down or `j`/`k` to scroll through the details and diff, `PgUp`/`PgDn` to move by a
screen, and `esc` to return to the same inbox position. Diff additions,
deletions, hunks, old/new line numbers, and recognized source code are highlighted;
long lines wrap. PR descriptions render as Markdown, including lists and fenced
code. On wide terminals, mergeability, checks, and review activity are in a
right-hand panel; press `tab` to focus it and use `j`/`k` or `PgUp`/`PgDn` to
scroll it independently. The active pane is highlighted without a focus label.
On narrow terminals, checks and reviews appear below the diff. Merge and
navigation controls stay together in the pinned action area. Press `m` to select
an enabled merge method with up/down or `j`/`k`, press `enter`, then confirm
with `y`. glance sends the merge through the GitHub API using the loaded head
SHA and refreshes the dashboard afterwards.
The Issues section shows open issues across accessible repositories with labels
and comment counts. Notifications shows read and unread threads and their
reasons; use up/down or `j`/`k` to browse either inbox.

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
