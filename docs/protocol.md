# Council Protocol

`council-ui` consumes council state rather than owning it. That boundary keeps
the UX project independent from orchestration changes.

## Durable State

The durable protocol is a run directory:

```text
council-out/runs/<run-id>/
```

At startup, `council-ui` discovers projects by scanning candidate workspace
directories for that `council-out/runs` shape. Passing `--home` or
`--workspace` bypasses discovery.

Required identity files:

- `task.txt`: original task text
- `workspace.txt`: target workspace
- `instance.txt`: council instance, defaulting to `default`

Lifecycle files:

- `status.txt`: terminal status such as `SUCCESS`, `FAILED`, or `CANCELLED`
- `phase.txt`: coarse state, for example `planning`, `plan-complete`,
  `execution`, or `complete`
- `target.txt`: stop boundary, either `plan` or `complete`
- `stage.txt`: latest active stage marker
- `progress.log`: timestamped stage and phase entries

Artifact files:

- `plans/codex.md`, `plans/cc.md`, `plans/amp.md`
- `critiques/codex.md`, `critiques/cc.md`, `critiques/amp.md`
- `plan.final.md`
- `implementation/codex.md`
- `implementation/codex.revise-round-N.md`
- `reviews/cc.round-N.md`, `reviews/amp.round-N.md`

## Next Stage Rules

The UI mirrors the existing shell runner:

1. Missing any plan means `plans`.
2. Missing any critique means `critiques`.
3. Missing `plan.final.md` means `final-plan`.
4. If `target.txt` is `plan`, final plan completion means `complete`.
5. Missing `implementation/codex.md` means `implementation`.
6. Missing review files means `reviews-round-N`.
7. If both review verdicts are `LGTM`, the run is `complete`.
8. If max review rounds are exhausted, the run is `complete`.
9. Otherwise, missing a revision artifact means `revise-round-N`.

## Live State

Live state comes from tmux, not the run directory. Panes are matched by names:

- `council-codex`
- `council-cc`
- `council-amp`
- `council-orchestrator`

Named instances append `-<instance>`, for example
`council-codex-feature-a`.

The UI treats tmux as optional. If tmux is unavailable, run progress still
renders from disk and pane preview/switching is disabled.

## Action Boundary

`council-ui` does not execute orchestration logic itself. Dashboard actions call
the configured `council` executable from the selected project or run workspace:

- `council run --instance <instance> -- <goal>`
- `council attach --instance <instance>`
- `council resume <run-id>`
- `council exec <run-id>`
- `council reset --instance <instance>`

Reset is confirmation-gated in the UI. After each action, the UI reloads durable
run state and live tmux pane state from the protocol sources above.
