# Comparable Tools Research

Research date: 2026-05-02

`council-ui` sits between a terminal operations console and an agent run
inspector. The closest products are not direct clones; they share pieces of the
job:

- TUI operations dashboards make dense state navigable.
- Terminal session managers preserve and re-enter live work contexts.
- Coding-agent clients supervise long-running agent work.
- Agent observability tools explain what happened inside a run.

This document uses representative, current tools from each category and turns
their strongest patterns into `council-ui` improvements.

## Comparable Tools

| Tool | Category | What it does well | Lesson for `council-ui` |
| --- | --- | --- | --- |
| lazygit | TUI ops console | Panel-focused keyboard navigation, contextual help, filtering, command log, undo/redo, external editor/browser actions, GitHub PR state in-list. | Add `?` contextual help, command log, richer filters, copy/open actions, and status badges that encode external state without leaving the run list. |
| lazydocker | TUI ops console | Shows containers, compose services, logs, stats, and actions in one terminal UI. | Put durable artifact state, live pane health, and latest event age on one screen; make common operations one-key but confirmed. |
| k9s | TUI ops console | Watches resources continuously, supports navigation, customization, skins, command-driven resource selection, and live logs. | Treat run refresh as a first-class live view: status age, blocked resource, sort mode, and theme/config should be visible and controllable. |
| Zellij | Terminal/session manager | Floating panes, session manager, persisted context, command panes that can be re-run. | Use modal/floating patterns for artifacts and actions; make `resume`, `exec`, and `review` command panes/actions explicit and repeatable. |
| tmuxinator/tmuxp | tmux workspace managers | Declarative project layouts and the ability to reconstruct sessions from config or existing sessions. | Generate or consume a council session manifest so live panes can be reconstructed, not just discovered. |
| WezTerm multiplexing | Terminal/session manager | Domains, remote/local multiplexing, native mouse/clipboard/scrollback integration. | Keep tmux optional at the protocol boundary; design live-pane discovery as an adapter so other multiplexers can be added later. |
| Codex CLI / Codex app | Coding-agent client | Local terminal agent, image inputs, local review, subagents, web search, cloud tasks, approvals, and a broader command-center model for parallel agents. | Add a run supervisor view: active agents, approvals needed, artifacts produced, review result, and follow-up actions across runs. |
| Claude Code | Coding-agent client | Multi-agent teams, CLI scripting, recurring tasks, session handoff across terminal, desktop, web, mobile, CI, and chat. | Make council runs portable and resumable across surfaces by keeping protocol state durable and adding external actions around it. |
| Aider | Coding-agent client | Codebase map, automatic git commits, lint/test loop, multi-model support, images/web pages as context. | Surface test/lint evidence and commit/change summaries as artifacts, not only agent notes. |
| Cline / Cursor / Windsurf / Roo Code | IDE agent surfaces | Mode separation such as ask/plan/act, background agents, checkpoints, code/chat modes, and task sidebars. | Add explicit phase/mode controls and a checkpoint/history view. Make "planning" and "execution" impossible to confuse. |
| AutoGen Studio | Multi-agent workflow UI | Declarative skills/models/agents/workflows, interactive testing, artifact review, inner monologue, profiling, export/deploy. | Add a run anatomy view: agents, model/settings metadata, artifacts, turns, tool calls, cost/duration if available, and exportable run summaries. |
| LangSmith Studio | Agent development/debug UI | Local agent server, visual execution trace, prompts, tool calls, results, intermediate state, exceptions, latency/token metrics, hot reload. | Add structured event capture and a timeline/trace view; plain `progress.log` is not enough for debugging multi-agent work. |
| CrewAI | Multi-agent framework/platform | Agents, crews, flows, guardrails, memory, knowledge, observability. | Keep `council-ui` protocol-first, but model "guardrail failed", "human needed", and "memory/context changed" as first-class states. |
| OpenHands | Agent platform | Terminal/headless agents plus collaborative web UI for planning, running, and reviewing work. | Preserve a clean split: CLI/TUI for fast local operations, but artifacts should be portable enough for a future web review surface. |
| Phoenix / LangSmith / Sentry AI Performance / OpenTelemetry GenAI | Observability | Trace model calls, tool use, retrieval/custom logic, tokens, latency, exceptions, and agent spans using common telemetry concepts. | Introduce `events.jsonl` or trace export with run, phase, agent, tool, artifact, duration, status, and error fields. |

## Product Gaps

`council-ui` already has the right foundation: it reads durable artifacts,
discovers live panes, shows phase progress, supports project selection, and can
open artifacts in a modal. The gaps are mostly around explainability and
actions.

1. The run list is under-informative. It sorts by latest file modification time,
   but the UI does not show the timestamp, sort mode, or why a run moved.
2. Keybindings are footer-only. Users need a contextual `?` help overlay and a
   stable command palette.
3. Artifact browsing is a preview, not yet an inspector. It lacks search,
   file switching, open-in-editor, copy path, line numbers, and diff views.
4. Live pane state is shallow. The UI knows a pane exists but not whether the
   agent is idle, waiting for approval, producing output, failed, or stale.
5. Progress is log-shaped. It should become event-shaped so the UI can build a
   timeline, trace, summaries, and filters without scraping text.
6. Execution actions are outside the UI. `resume`, `exec`, `review`, and
   cancellation/reset flows should be available with confirmation.
7. Multi-agent review state is compressed into a small verdict string. It needs
   round history, reviewer disagreement, blocking critique, and "needs human"
   cues.
8. There is no persistent user configuration for keybindings, colors, default
   sort, project roots, or external editor commands.

## Improvement Backlog

### P0: Make The Current Dashboard Understandable

- Show sort mode and timestamp in the run list: `updated 16:15`, `created`, or
  `status age`.
- Add sort toggles: updated time, run id, status, workspace, and active panes.
- Add `?` contextual help with the current view's keys.
- Add a small command log/status drawer for the last refresh, tmux command, pane
  switch, and artifact read error.
- Keep selected run stable by run ID across refreshes, instead of only by list
  index.

### P1: Make Artifacts Feel Like Files

- Add artifact file picker per run with sections: plans, critiques, final plan,
  implementation, reviews, progress.
- Add search inside artifact modal with `/`.
- Add line numbers and `g/G` plus `n/N` search navigation.
- Add `y` copy path, `e` open in `$EDITOR`, and `O` open with system default.
- Add diffs: critique versus final plan, implementation versus review, and
  revision rounds.

### P1: Add Safe Actions

- Add action palette (`:` or `a`) with context-specific commands.
- Add confirmed actions for `council resume <run-id>`, `council exec <run-id>`,
  `council review <run-id>`, and cancellation/reset once supported upstream.
- Show dry-run command text before executing.
- Write an action log entry to the run so actions are auditable.

### P2: Add Structured Events

Add a protocol file such as `events.jsonl` next to `progress.log`. Suggested
minimum fields:

```json
{"time":"2026-05-02T10:15:30Z","run":"20260412-161558-5680","phase":"planning","agent":"codex","event":"artifact.write","artifact":"plan.final.md","status":"ok","duration_ms":1200}
```

Useful event types:

- `run.start`, `run.complete`, `run.fail`
- `phase.start`, `phase.complete`, `phase.blocked`
- `agent.start`, `agent.idle`, `agent.waiting`, `agent.fail`
- `artifact.write`, `artifact.update`, `artifact.missing`
- `review.verdict`, `review.disagreement`
- `tool.start`, `tool.complete`, `tool.fail`
- `human.approval.requested`, `human.approval.resolved`

This gives the UI enough structure to build timeline views and eventually emit
OpenTelemetry-compatible spans for external observability tools.

### P2: Make Multi-Agent Work Supervised

- Add a "Needs attention" queue across projects.
- Add an agent matrix: rows are runs, columns are codex/cc/amp/orchestrator,
  cells show live, stale, waiting, failed, or complete.
- Add reviewer disagreement view with links to exact critique/review artifacts.
- Add phase-mode clarity: planning, critique, execution, review, complete should
  be visually distinct and never inferred only from a free-text stage.

### P3: Configuration And Extensibility

- Add config file for project roots, default sort, theme, editor command, and
  keybindings.
- Add skin/theme support after layout stabilizes.
- Keep tmux as one live-pane provider, but define a provider interface so
  Zellij, WezTerm, or future council-native runtimes can be added without
  changing protocol parsing.
- Add export: run summary markdown, JSON, or issue/PR body.

## Recommended Next Steps

1. Implement run-list timestamp and sort controls first. This answers the
   immediate "why did this move?" question.
2. Implement contextual `?` help and command palette. This reduces keybinding
   ambiguity as the UI grows.
3. Expand the artifact modal into a real artifact browser with file picker and
   search.
4. Add `events.jsonl` support in read-only mode, then update `maestro-council`
   to produce it.
5. Add safe `resume` and `exec` actions only after confirmation flow and action
   logging exist.

## Sources

- lazygit: https://github.com/jesseduffield/lazygit
- lazydocker: https://github.com/jesseduffield/lazydocker
- k9s: https://github.com/derailed/k9s
- Zellij: https://zellij.dev/features/
- tmuxinator: https://github.com/tmuxinator/tmuxinator
- tmuxp: https://tmuxp.git-pull.com/about/
- WezTerm multiplexing: https://wezterm.org/multiplexing.html
- OpenAI Codex CLI: https://developers.openai.com/codex/cli
- OpenAI Codex product: https://openai.com/codex/
- Claude Code: https://code.claude.com/docs/en/overview
- Aider: https://aider.chat/
- Cline Plan and Act: https://docs.cline.bot/features/plan-and-act
- Cursor background agents: https://docs.cursor.com/en/background-agents
- Windsurf Cascade: https://docs.windsurf.com/windsurf/cascade/cascade
- Roo Code: https://docs.roocode.com/
- AutoGen Studio: https://autogenhub.github.io/autogen/docs/autogen-studio/usage/
- LangSmith Studio: https://docs.langchain.com/oss/javascript/langgraph/studio
- CrewAI: https://docs.crewai.com/index
- OpenHands: https://openhands.dev/product
- Phoenix: https://arize.com/docs/phoenix
- Sentry AI Performance: https://docs.sentry.io/product/insights/ai/
- OpenTelemetry GenAI agent spans:
  https://opentelemetry.io/docs/specs/semconv/gen-ai/gen-ai-agent-spans/
