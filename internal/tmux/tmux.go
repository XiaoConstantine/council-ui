package tmux

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return out, fmt.Errorf("%s: %w", msg, err)
		}
		return out, err
	}
	return out, nil
}

type Client struct {
	Runner Runner
}

type Pane struct {
	ID        string
	Session   string
	WindowID  string
	Window    string
	Index     string
	Size      string
	Command   string
	Label     string
	Workspace string
	Role      string
	Instance  string
}

type Council struct {
	Instance  string
	Window    string
	Workspace string
	Panes     []Pane
}

func (c Client) ListPanes(ctx context.Context) ([]Pane, error) {
	runner := c.Runner
	if runner == nil {
		runner = ExecRunner{}
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	out, err := runner.Run(ctx, "tmux", "list-panes", "-a", "-F", "#{pane_id}\t#{session_name}\t#{window_id}\t#{window_index}\t#{window_name}\t#{pane_width}x#{pane_height}\t#{pane_current_command}\t#{@name}\t#{@maestro_council_workspace}")
	if err != nil {
		return nil, err
	}

	var panes []Pane
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		for len(fields) < 9 {
			fields = append(fields, "")
		}
		pane := Pane{
			ID:        fields[0],
			Session:   fields[1],
			WindowID:  fields[2],
			Index:     fields[3],
			Window:    fields[4],
			Size:      fields[5],
			Command:   fields[6],
			Label:     fields[7],
			Workspace: fields[8],
		}
		pane.Role, pane.Instance = ParseCouncilLabel(pane.Label)
		if pane.Role == "" {
			pane.Role, pane.Instance = ParseCouncilWindow(pane.Window)
		}
		if pane.Role != "" || strings.HasPrefix(pane.Window, "maestro-council") {
			panes = append(panes, pane)
		}
	}
	return panes, nil
}

func (c Client) Councils(ctx context.Context) ([]Council, error) {
	panes, err := c.ListPanes(ctx)
	if err != nil {
		return nil, err
	}

	byKey := map[string]*Council{}
	for _, pane := range panes {
		if pane.Instance == "" {
			continue
		}
		key := pane.Window + "\x00" + pane.Instance
		council := byKey[key]
		if council == nil {
			council = &Council{
				Instance:  pane.Instance,
				Window:    pane.Window,
				Workspace: pane.Workspace,
			}
			byKey[key] = council
		}
		if council.Workspace == "" && pane.Workspace != "" {
			council.Workspace = pane.Workspace
		}
		council.Panes = append(council.Panes, pane)
	}

	councils := make([]Council, 0, len(byKey))
	for _, council := range byKey {
		sort.Slice(council.Panes, func(i, j int) bool {
			return roleRank(council.Panes[i].Role) < roleRank(council.Panes[j].Role)
		})
		councils = append(councils, *council)
	}
	sort.Slice(councils, func(i, j int) bool {
		return councils[i].Instance < councils[j].Instance
	})
	return councils, nil
}

func (c Client) CapturePane(ctx context.Context, paneID string, lines int) (string, error) {
	if paneID == "" {
		return "", errors.New("pane id is empty")
	}
	if lines <= 0 {
		lines = 120
	}

	runner := c.Runner
	if runner == nil {
		runner = ExecRunner{}
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	out, err := runner.Run(ctx, "tmux", "capture-pane", "-e", "-J", "-p", "-S", fmt.Sprintf("-%d", lines), "-t", paneID)
	return string(out), err
}

func (c Client) SelectPane(ctx context.Context, pane Pane) error {
	if pane.ID == "" {
		return errors.New("pane id is empty")
	}

	runner := c.Runner
	if runner == nil {
		runner = ExecRunner{}
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if pane.Session != "" && pane.Index != "" {
		_, _ = runner.Run(ctx, "tmux", "select-window", "-t", pane.Session+":"+pane.Index)
	}
	_, err := runner.Run(ctx, "tmux", "select-pane", "-t", pane.ID)
	return err
}

func ParseCouncilLabel(label string) (role string, instance string) {
	switch {
	case label == "council-codex":
		return "codex", "default"
	case label == "council-cc":
		return "cc", "default"
	case label == "council-amp":
		return "amp", "default"
	case label == "council-orchestrator":
		return "orchestrator", "default"
	case strings.HasPrefix(label, "council-codex-"):
		return "codex", strings.TrimPrefix(label, "council-codex-")
	case strings.HasPrefix(label, "council-cc-"):
		return "cc", strings.TrimPrefix(label, "council-cc-")
	case strings.HasPrefix(label, "council-amp-"):
		return "amp", strings.TrimPrefix(label, "council-amp-")
	case strings.HasPrefix(label, "council-orchestrator-"):
		return "orchestrator", strings.TrimPrefix(label, "council-orchestrator-")
	default:
		return "", ""
	}
}

func ParseCouncilWindow(window string) (role string, instance string) {
	switch {
	case window == "maestro-council":
		return "window", "default"
	case strings.HasPrefix(window, "maestro-council-"):
		return "window", strings.TrimPrefix(window, "maestro-council-")
	default:
		return "", ""
	}
}

func roleRank(role string) int {
	switch role {
	case "orchestrator":
		return 0
	case "codex":
		return 1
	case "cc":
		return 2
	case "amp":
		return 3
	default:
		return 9
	}
}
