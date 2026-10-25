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

### Finding your way around

glance opens directly to Pull requests. On wider terminals, framed inbox and
preview panes show the selection, focus, and scroll position. Roomier inboxes
include author/activity details and cached check summaries alongside each item.
Narrow terminals use a single column. Shortcut hints stay pinned at the bottom.

| Key | Action |
| --- | --- |
| `1` / `2` / `3`, left/right or `h`/`l` | Switch inbox sections |
| Up/down or `j`/`k` | Select an item, or scroll the focused preview |
| `tab` / `shift+tab` | Switch between list and preview on wide terminals |
| `PgUp` / `PgDn`, `ctrl+u` / `ctrl+d` | Move a screen in the active pane |
| `Home` / `End` | First/last item or line |
| `esc` | Return focus from the preview to the list |
| `enter` | Open the selected PR's review and diff |
| `:` | Search the command palette |
| `?` | Open scrollable keyboard help, including during PR review |
| `r` / `q` | Refresh / quit |

The palette matches words in command names and descriptions. Use up/down,
`tab`/`shift+tab`, or `ctrl+n`/`ctrl+p` to select a command, `enter` to run it,
and `esc` to return to the same position. Command and file pickers overlay the
dimmed workspace, keeping your context visible. The palette also shows each
command's shortcut.

The selected PR's description, mergeability, checks, and review activity load
after a short pause and are cached while browsing. Focus its preview to scroll
through the full content. PR and issue descriptions render Markdown, including
lists and code, while preserving authored line breaks. Issues include labels
and comment counts; notifications explain why each thread is in your inbox.

### GitHub filters

The PR inbox defaults to `is:pr is:open author:@me`. Press `/` to edit it using
GitHub search qualifiers (for example, `repo:owner/name is:open`), `enter` to
apply, or `esc` to cancel. Editing only fetches results when you submit.
The command palette includes **My pull requests**, **Needs my review**, and
**All open pull requests** for switching directly between common GitHub queries.

Use left/right to move the cursor, Home/End or `ctrl+a`/`ctrl+e` to jump to an
edge, and `ctrl+left`/`ctrl+right` or `alt+b`/`alt+f` to move by a word.
Backspace/Delete edit at the cursor, `ctrl+w` deletes the preceding word, and
`ctrl+u` clears the query. Long queries scroll horizontally with the cursor.

### Reviewing and merging

Press `enter` on a PR to open its details and automatically load all changed
files and patches in the same continuous, scrollable view.
Use up/down or `j`/`k` to scroll through the details and diff, `PgUp`/`PgDn` to move by a
screen, and `esc` to return to the same inbox position. Diff additions,
deletions, hunks, old/new line numbers, and recognized source code are highlighted;
long lines wrap. PR descriptions render as Markdown, including lists and fenced
code. A sticky file header keeps the current path visible as you scroll.

| Key | Review navigation |
| --- | --- |
| `f` | Search changed paths, including previous names of renamed files |
| `enter` in the file picker | Jump directly to that file's patch |
| `[` / `]` | Previous / next changed file |
| `d` | Jump to the first file in the diff |
| `v` | Toggle the current file as viewed locally |

The file picker shows change counts and viewed markers, including files whose
patches GitHub cannot provide. Viewed progress belongs to the current review;
it clears for changed or removed files on refresh and when opening a PR anew.
These markers are local navigation aids and are not submitted to GitHub.

On wide terminals, mergeability, checks, and review activity are in a
right-hand panel; press `tab` to focus it and use `j`/`k` or `PgUp`/`PgDn` to
scroll it independently. The active pane is highlighted without a focus label.
On narrow terminals, checks and reviews appear below the diff. Merge and
navigation controls stay together in the pinned action area. Press `m` to select
an enabled merge method with up/down or `j`/`k`, press `enter`, then confirm
with `y`. glance sends the merge through the GitHub API using the loaded head
SHA and refreshes the dashboard afterwards.

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
