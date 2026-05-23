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
- Clickable command bar for high-confidence actions without memorizing keys.
- One-keystroke switching into the selected live pane.
- Clear distinction between durable run progress and live pane health.
- Fast filtering across task text, run id, workspace, instance, and stage.
- Startup should ask which project to inspect instead of assuming the current
  repo is the council workspace.
- Runs, artifacts, and panes should be first-class selectable regions so the UI
  feels like a control surface rather than a report browser.
- Pane previews that are useful but never treated as source of truth.

## Near-Term Improvements

- Use the comparable-tools research in `docs/comparable-tools.md` to guide UI
  priorities.
- Show run-list timestamps and sort mode, with toggles for updated time, status,
  workspace, and active panes.
- Add contextual `?` help and an action palette for discoverability.
- Expand the zoom view into an artifact browser with search, file picker,
  copy path, and open-in-editor actions.
- Improve live pane preview cleanup and truncation.
- Add structured `events.jsonl` support when `maestro-council` produces it.
- Group runs by workspace and instance once there are enough active councils.
