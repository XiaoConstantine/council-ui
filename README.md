# council-ui

`council-ui` is a UI-first client for
[`maestro-council`](../maestro-council). It treats council as a protocol:
run artifacts on disk plus live tmux pane metadata. The existing shell runner
continues to own orchestration, agent dispatch, resume, review rounds, and
failure handling.

The first version is a focused terminal control surface:

- scans `council-out/runs` for planning and execution progress
- shows phase, target, next stage, missing artifacts, reviewer verdicts, and
  recent progress log entries
- discovers live council panes from tmux labels
- previews selected run artifacts and selected council pane output
- switches to the selected live pane from the panes list
- exposes clickable actions for start, attach, resume, exec, zoom, reset,
  refresh, and quit while keeping orchestration in `maestro-council`

## Install

```bash
go install github.com/XiaoConstantine/council-ui/cmd/council-ui@latest
```

From a local checkout:

```bash
go build ./cmd/council-ui
```

## Run

Run without flags to choose a project interactively. The picker scans the
current directory and its parent for workspaces. Projects with no `council-out`
yet are included when they look like normal project workspaces, such as Git or
Go/Node/Python/Rust projects:

```bash
council-ui
```

To scan a specific parent directory:

```bash
council-ui --projects-root /path/to/repos
```

To skip the picker, point the UI at a workspace that contains `council-out`:

```bash
council-ui --workspace /path/to/workspace
```

Or pass the council home directly:

```bash
council-ui --home /path/to/workspace/council-out
```

If the `council` executable is not on `PATH`, point actions at it explicitly:

```bash
council-ui --workspace /path/to/workspace --council /path/to/maestro-council/bin/council
```

The interactive picker intentionally scans project workspaces instead of
silently loading a global home. Use `--home` when a project stores council
artifacts outside its workspace.

For a fresh project with no runs yet, select the project and press `s` or click
`Start` to create the council panes in that workspace.

## Keys

Project picker:

- `j` / `k`: move through projects
- `Enter`: load selected project
- `r`: rescan
- `q`: quit

Dashboard:

- `Tab`: cycle Runs, Artifacts, and Panes focus
- `j` / `k`: move within the focused list
- `g` / `G`: jump to first or last row in the focused list
- `z` / `o`: zoom the selected artifact or pane output
- `s`: run `council start --instance <instance>`
- `a`: attach/switch to the selected pane, or run `council attach`
- `r`: run `council resume <run-id>`
- `e`: run `council exec <run-id>`
- `R`: confirm and run `council reset --instance <instance>`
- `Ctrl+R`: refresh immediately
- `/`: filter by run id, task, workspace, instance, phase, or stage
- `P`: return to project picker
- `q`: quit

The command bar is clickable in terminals with mouse reporting:

```text
[Start] [Attach] [Resume] [Exec] [Zoom] [Reset] [Refresh] [Quit]
```

Clicking a run, artifact, or pane selects it. Mouse wheel scrolls zoomed
artifact or pane content.

Zoom view:

- `j` / `k`: scroll line by line
- `Ctrl+D` / `Ctrl+U`: scroll by a screen
- `PageDown` / `PageUp`: scroll by a screen
- `g` / `G`: jump to top or bottom
- `Esc`, `q`, `o`, or `z`: close zoom

## Protocol Boundary

The UI reads:

- `task.txt`
- `workspace.txt`
- `instance.txt`
- `status.txt`
- `phase.txt`
- `target.txt`
- `stage.txt`
- `progress.log`
- `plans/*.md`
- `critiques/*.md`
- `plan.final.md`
- `implementation/*.md`
- `reviews/*.md`

It also reads live tmux state with:

```bash
tmux list-panes -a -F '#{pane_id} ... #{@name} #{@maestro_council_workspace}'
```

The UI intentionally does not reimplement `council run`, `council plan`,
`council exec`, or `council resume`. Those remain protocol producers invoked
through the configured `council` executable.

## Direction

Live pane preview uses `tmux capture-pane` text directly. Durable council
artifacts remain the source of truth for plans, implementation notes, reviews,
and progress history.
