# Experience Notes

The product should feel like a focused operations console, not another
orchestrator. The main job is to answer:

- What councils exist?
- Which run is blocked or moving?
- Which agent is responsible for the current stage?
- What changed recently?
- How do I jump into the right pane quickly?

## UI Priorities

- Dense scanability over decoration.
- One-keystroke switching into the selected live pane.
- Clear distinction between durable run progress and live pane health.
- Fast filtering across task text, run id, workspace, instance, and stage.
- Startup should ask which project to inspect instead of assuming the current
  repo is the council workspace.
- Pane previews that are useful but never treated as source of truth.

## Near-Term Improvements

- Add actions for `council resume <run-id>` and `council exec <run-id>`.
- Add confirmation flow for disruptive actions like reset.
- Add a richer pane preview renderer behind `internal/termview`.
- Add structured `events.jsonl` support when `maestro-council` produces it.
- Group runs by workspace and instance once there are enough active councils.
