# council-ui

`council-ui` is a UI-first client for
[`maestro-council`](../maestro-council). It treats council as a protocol:
run artifacts on disk plus live tmux pane metadata. The existing shell runner
continues to own orchestration, agent dispatch, resume, review rounds, and
failure handling.

The first version is a focused terminal dashboard:

- scans `council-out/runs` for planning and execution progress
- shows phase, target, next stage, missing artifacts, reviewer verdicts, and
  recent progress log entries
- discovers live council panes from tmux labels
- previews the selected agent pane with `tmux capture-pane`
- switches to the selected live pane with `Enter`

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
current directory and its parent for workspaces that contain
`council-out/runs`:

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

The interactive picker intentionally scans project workspaces instead of
silently loading a global home. Use `--home` when a project stores council
artifacts outside its workspace.

## Keys

Project picker:

- `j` / `k`: move through projects
- `Enter`: load selected project
- `r`: rescan
- `q`: quit

Dashboard:

- `j` / `k`: move through runs
- `h` / `l`: move through Plan, Execution, Reviews, and Progress sections
- `Space`: expand or collapse the selected section
- `Tab`: cycle codex, cc, amp, orchestrator pane preview
- `o`: open the selected artifact in a modal viewer
- `Ctrl+D` / `Ctrl+U`: scroll artifact preview down or up
- `PageDown` / `PageUp`: scroll artifact preview down or up
- `Enter`: switch tmux focus to the selected live pane
- `/`: filter by run id, task, workspace, instance, phase, or stage
- `P`: return to project picker
- `r`: refresh immediately
- `q`: quit

Artifact modal:

- `j` / `k`: scroll line by line
- `Ctrl+D` / `Ctrl+U`: scroll by half a screen
- `PageDown` / `PageUp`: scroll by half a screen
- `g` / `G`: jump to top or bottom
- `h` / `l`: switch artifact section
- `Tab` / `Shift+Tab`: switch artifact agent
- `Esc`, `q`, or `o`: close the modal

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

The initial repo intentionally does not reimplement `council run`,
`council plan`, `council exec`, or `council resume`. Those remain protocol
producers.

## Direction

Live pane preview uses `tmux capture-pane` text directly. Durable council
artifacts remain the source of truth for plans, implementation notes, reviews,
and progress history.
