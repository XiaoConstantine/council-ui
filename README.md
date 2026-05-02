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

The default build uses a plain preview renderer so the UI works without native
terminal-emulation libraries. To use `go-libghostty`, install/build
`libghostty-vt`, set `PKG_CONFIG_PATH`, then build with:

```bash
go build -tags libghostty ./cmd/council-ui
```

The pane header shows the active renderer, for example `renderer: plain` or
`renderer: go-libghostty`.

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
- `Enter`: switch tmux focus to the selected live pane
- `/`: filter by run id, task, workspace, instance, phase, or stage
- `P`: return to project picker
- `r`: refresh immediately
- `q`: quit

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

Pane rendering goes through `internal/termview`. The default renderer is plain
text; `-tags libghostty` enables the `go-libghostty` adapter for real
`libghostty-vt` terminal interpretation. Keeping that behind an adapter matters
because `go-libghostty` is cgo-based and the Go API is still settling.
