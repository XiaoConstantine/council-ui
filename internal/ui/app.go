package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/XiaoConstantine/council-ui/internal/protocol"
	"github.com/XiaoConstantine/council-ui/internal/tmux"
)

type Options struct {
	Home    string
	Load    protocol.LoadOptions
	Refresh time.Duration
}

type Model struct {
	opts          Options
	tmux          tmux.Client
	runs          []protocol.Run
	councils      []tmux.Council
	panes         []tmux.Pane
	selectedRun   int
	selectedAgent int
	width         int
	height        int
	filter        string
	filtering     bool
	preview       string
	err           error
	tmuxErr       error
	status        string
	loadedAt      time.Time
}

type refreshMsg struct {
	runs     []protocol.Run
	councils []tmux.Council
	panes    []tmux.Pane
	preview  string
	err      error
	tmuxErr  error
	loadedAt time.Time
}

type switchMsg struct {
	err error
}

type tickMsg struct{}

func New(opts Options) Model {
	if opts.Refresh <= 0 {
		opts.Refresh = time.Second
	}
	return Model{
		opts:          opts,
		tmux:          tmux.Client{},
		selectedAgent: 0,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.refreshCmd(), tick(m.opts.Refresh))
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tickMsg:
		return m, tea.Batch(m.refreshCmd(), tick(m.opts.Refresh))
	case refreshMsg:
		m.runs = msg.runs
		m.councils = msg.councils
		m.panes = msg.panes
		m.preview = msg.preview
		m.err = msg.err
		m.tmuxErr = msg.tmuxErr
		m.loadedAt = msg.loadedAt
		if m.selectedRun >= len(m.visibleRuns()) {
			m.selectedRun = max(0, len(m.visibleRuns())-1)
		}
		return m, nil
	case switchMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
		} else {
			m.status = "switched to pane"
		}
		return m, nil
	case tea.KeyMsg:
		if m.filtering {
			return m.updateFilter(msg)
		}
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "j", "down":
			if m.selectedRun < len(m.visibleRuns())-1 {
				m.selectedRun++
			}
			return m, m.refreshCmd()
		case "k", "up":
			if m.selectedRun > 0 {
				m.selectedRun--
			}
			return m, m.refreshCmd()
		case "tab":
			m.selectedAgent = (m.selectedAgent + 1) % len(agentOrder)
			return m, m.refreshCmd()
		case "shift+tab":
			m.selectedAgent--
			if m.selectedAgent < 0 {
				m.selectedAgent = len(agentOrder) - 1
			}
			return m, m.refreshCmd()
		case "enter":
			pane, ok := m.selectedPane()
			if !ok {
				m.status = "no live pane for selection"
				return m, nil
			}
			return m, m.switchCmd(pane)
		case "r":
			return m, m.refreshCmd()
		case "/":
			m.filtering = true
			return m, nil
		}
	}
	return m, nil
}

func (m Model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.filtering = false
	case "enter":
		m.filtering = false
		m.selectedRun = 0
	case "backspace":
		if len(m.filter) > 0 {
			m.filter = m.filter[:len(m.filter)-1]
			m.selectedRun = 0
		}
	case "ctrl+u":
		m.filter = ""
		m.selectedRun = 0
	default:
		if len(msg.String()) == 1 {
			m.filter += msg.String()
			m.selectedRun = 0
		}
	}
	return m, m.refreshCmd()
}

func (m Model) View() string {
	if m.width == 0 {
		return "loading..."
	}

	top := m.renderTop()
	bottom := m.renderBottom()
	bodyHeight := max(8, m.height-lipgloss.Height(top)-lipgloss.Height(bottom))
	listWidth := clamp(38, 34, max(34, m.width/3))
	detailWidth := max(40, m.width-listWidth-1)

	left := listBox.Width(listWidth).Height(bodyHeight).Render(m.renderRunList(listWidth, bodyHeight))
	right := lipgloss.JoinVertical(
		lipgloss.Left,
		detailBox.Width(detailWidth).Height(max(12, bodyHeight/2)).Render(m.renderDetail(detailWidth, max(12, bodyHeight/2))),
		previewBox.Width(detailWidth).Height(max(8, bodyHeight-max(12, bodyHeight/2)-1)).Render(m.renderPreview(detailWidth, max(8, bodyHeight-max(12, bodyHeight/2)-1))),
	)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	return lipgloss.JoinVertical(lipgloss.Left, top, body, bottom)
}

func (m Model) refreshCmd() tea.Cmd {
	return func() tea.Msg {
		runs, err := protocol.LoadRuns(m.opts.Home, m.opts.Load)
		client := m.tmux
		councils, tmuxErr := client.Councils(context.Background())
		panes := flattenPanes(councils)

		next := m
		next.runs = runs
		next.councils = councils
		next.panes = panes
		preview := ""
		if pane, ok := next.selectedPane(); ok {
			capture, captureErr := client.CapturePane(context.Background(), pane.ID, 120)
			if captureErr == nil {
				preview = capture
			}
		}

		return refreshMsg{
			runs:     runs,
			councils: councils,
			panes:    panes,
			preview:  preview,
			err:      err,
			tmuxErr:  tmuxErr,
			loadedAt: time.Now(),
		}
	}
}

func (m Model) switchCmd(pane tmux.Pane) tea.Cmd {
	return func() tea.Msg {
		return switchMsg{err: m.tmux.SelectPane(context.Background(), pane)}
	}
}

func tick(refresh time.Duration) tea.Cmd {
	return tea.Tick(refresh, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m Model) renderTop() string {
	live := fmt.Sprintf("%d live panes", len(m.panes))
	if m.tmuxErr != nil {
		live = "tmux unavailable"
	}
	subtitle := fmt.Sprintf("%s  %s  %s", shortPath(m.opts.Home), live, m.loadedAt.Format("15:04:05"))
	return titleBar.Width(m.width).Render(
		lipgloss.JoinHorizontal(
			lipgloss.Center,
			titleStyle.Render("council-ui"),
			" ",
			subtleStyle.Render(subtitle),
		),
	)
}

func (m Model) renderBottom() string {
	mode := "j/k move  tab panes  enter switch  / filter  q quit"
	if m.filtering {
		mode = "filter: " + m.filter + "  enter apply  esc close  ctrl+u clear"
	} else if m.filter != "" {
		mode += "  filter: " + m.filter
	}
	if m.status != "" {
		mode += "  " + m.status
	}
	if m.err != nil {
		mode += "  " + m.err.Error()
	}
	return footer.Width(m.width).Render(mode)
}

func (m Model) renderRunList(width, height int) string {
	runs := m.visibleRuns()
	if len(runs) == 0 {
		return emptyStyle.Render("No council runs found.")
	}

	lines := make([]string, 0, height)
	for i, run := range runs {
		prefix := "  "
		style := listItemStyle
		if i == m.selectedRun {
			prefix = "▸ "
			style = selectedItemStyle
		}
		status := statusBadge(run.Status)
		line := fmt.Sprintf("%s%s %s %s", prefix, status, run.Instance, run.ID)
		lines = append(lines, style.Width(width-2).Render(truncate(line, width-4)))
		task := "    " + truncate(oneLine(run.Task), width-8)
		lines = append(lines, subtleStyle.Render(task))
		stage := fmt.Sprintf("    %s → %s", run.Phase, run.NextStage)
		lines = append(lines, mutedStyle.Render(truncate(stage, width-8)))
	}

	return strings.Join(fitLines(lines, height-1), "\n")
}

func (m Model) renderDetail(width, height int) string {
	run, ok := m.selectedRunSnapshot()
	if !ok {
		return emptyStyle.Render("Select a run.")
	}

	var lines []string
	lines = append(lines, sectionTitle.Render("Run"))
	lines = append(lines, fmt.Sprintf("%s  %s  %s", strongStyle.Render(run.ID), statusBadge(run.Status), run.Instance))
	lines = append(lines, subtleStyle.Render(truncate(oneLine(run.Task), width-4)))
	lines = append(lines, "")
	lines = append(lines, kv("workspace", shortPath(run.Workspace)))
	lines = append(lines, kv("phase", run.Phase))
	lines = append(lines, kv("target", run.Target))
	lines = append(lines, kv("next", run.NextStage))
	if run.Verdicts.CC != "" || run.Verdicts.AMP != "" {
		lines = append(lines, kv("reviews", fmt.Sprintf("cc=%s amp=%s", fallback(run.Verdicts.CC, "-"), fallback(run.Verdicts.AMP, "-"))))
	}
	lines = append(lines, "")
	lines = append(lines, m.renderPipeline(run, width)...)
	if len(run.Missing) > 0 {
		lines = append(lines, "")
		lines = append(lines, sectionTitle.Render("Waiting On"))
		for _, missing := range run.Missing {
			lines = append(lines, "  "+warnStyle.Render(missing))
		}
	}
	if len(run.Progress) > 0 {
		lines = append(lines, "")
		lines = append(lines, sectionTitle.Render("Recent"))
		start := max(0, len(run.Progress)-5)
		for _, event := range run.Progress[start:] {
			lines = append(lines, fmt.Sprintf("  %s  %s", mutedStyle.Render(event.Time), event.Stage))
		}
	}

	return strings.Join(fitLines(lines, height-1), "\n")
}

func (m Model) renderPipeline(run protocol.Run, width int) []string {
	plansDone := countDone(run.Artifacts.Plans)
	critiquesDone := countDone(run.Artifacts.Critiques)
	implementation := mark(run.Artifacts.Implementation)
	finalPlan := mark(run.Artifacts.FinalPlan)
	review := "○"
	if len(run.Artifacts.ReviewRounds) > 0 {
		for _, round := range run.Artifacts.ReviewRounds {
			if round.CC && round.AMP {
				review = "●"
			}
		}
	}
	line := fmt.Sprintf("%s plans %d/3   %s critiques %d/3   %s final   %s impl   %s review",
		mark(plansDone == 3), plansDone,
		mark(critiquesDone == 3), critiquesDone,
		finalPlan,
		implementation,
		review,
	)
	return []string{sectionTitle.Render("Progress"), truncate(line, width-4)}
}

func (m Model) renderPreview(width, height int) string {
	pane, ok := m.selectedPane()
	header := sectionTitle.Render("Pane")
	if ok {
		header = sectionTitle.Render(fmt.Sprintf("Pane %s · %s · %s", pane.ID, pane.Role, pane.Command))
	}

	var lines []string
	lines = append(lines, header)
	lines = append(lines, m.renderAgentTabs(width))
	if !ok {
		lines = append(lines, "")
		lines = append(lines, emptyStyle.Render("No live pane for this run or instance."))
		return strings.Join(fitLines(lines, height-1), "\n")
	}
	lines = append(lines, "")
	if strings.TrimSpace(m.preview) == "" {
		lines = append(lines, emptyStyle.Render("Pane preview is empty."))
		return strings.Join(fitLines(lines, height-1), "\n")
	}
	for _, line := range tailLines(m.preview, max(3, height-5)) {
		lines = append(lines, truncate(stripControlNoise(line), width-4))
	}
	return strings.Join(fitLines(lines, height-1), "\n")
}

func (m Model) renderAgentTabs(width int) string {
	var parts []string
	for i, role := range agentOrder {
		style := tabStyle
		if i == m.selectedAgent {
			style = activeTabStyle
		}
		parts = append(parts, style.Render(role))
	}
	return truncate(strings.Join(parts, " "), width-4)
}

func (m Model) selectedRunSnapshot() (protocol.Run, bool) {
	runs := m.visibleRuns()
	if len(runs) == 0 || m.selectedRun < 0 || m.selectedRun >= len(runs) {
		return protocol.Run{}, false
	}
	return runs[m.selectedRun], true
}

func (m Model) selectedPane() (tmux.Pane, bool) {
	run, ok := m.selectedRunSnapshot()
	if !ok {
		return tmux.Pane{}, false
	}
	role := agentOrder[m.selectedAgent]
	for _, pane := range m.panes {
		if pane.Instance == run.Instance && pane.Role == role {
			if run.Workspace == "" || pane.Workspace == "" || samePath(pane.Workspace, run.Workspace) {
				return pane, true
			}
		}
	}
	for _, pane := range m.panes {
		if pane.Instance == run.Instance && pane.Role == role {
			return pane, true
		}
	}
	return tmux.Pane{}, false
}

func (m Model) visibleRuns() []protocol.Run {
	if m.filter == "" {
		return m.runs
	}
	filter := strings.ToLower(m.filter)
	var runs []protocol.Run
	for _, run := range m.runs {
		haystack := strings.ToLower(strings.Join([]string{
			run.ID, run.Task, run.Workspace, run.Instance, run.Status, run.Phase, run.NextStage,
		}, " "))
		if strings.Contains(haystack, filter) {
			runs = append(runs, run)
		}
	}
	return runs
}

func flattenPanes(councils []tmux.Council) []tmux.Pane {
	var panes []tmux.Pane
	for _, council := range councils {
		panes = append(panes, council.Panes...)
	}
	return panes
}

func statusBadge(status string) string {
	switch strings.ToUpper(status) {
	case "SUCCESS":
		return okStyle.Render("SUCCESS")
	case "FAILED":
		return failStyle.Render("FAILED")
	case "CANCELLED":
		return warnStyle.Render("CANCELLED")
	case "-", "":
		return mutedStyle.Render("PENDING")
	default:
		return subtleStyle.Render(strings.ToUpper(status))
	}
}

func kv(key, value string) string {
	return fmt.Sprintf("%s %s", mutedStyle.Width(10).Render(key), value)
}

func mark(done bool) string {
	if done {
		return okStyle.Render("●")
	}
	return mutedStyle.Render("○")
}

func countDone(values map[string]bool) int {
	var count int
	for _, done := range values {
		if done {
			count++
		}
	}
	return count
}

func samePath(a, b string) bool {
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return a == b
	}
	return aa == bb
}

func shortPath(path string) string {
	if path == "" {
		return "-"
	}
	home := "" // keep deterministic without os.UserHomeDir errors in tests
	if strings.HasPrefix(path, home) && home != "" {
		return "~" + strings.TrimPrefix(path, home)
	}
	parent := filepath.Base(filepath.Dir(path))
	base := filepath.Base(path)
	if parent == "." || parent == "/" {
		return path
	}
	return filepath.Join(parent, base)
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func truncate(value string, width int) string {
	if width <= 1 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	return string(runes[:width-1]) + "…"
}

func fitLines(lines []string, height int) []string {
	if height <= 0 {
		return nil
	}
	if len(lines) <= height {
		return lines
	}
	return lines[:height]
}

func tailLines(value string, count int) []string {
	lines := strings.Split(strings.TrimRight(value, "\n"), "\n")
	if len(lines) <= count {
		return lines
	}
	return lines[len(lines)-count:]
}

func stripControlNoise(value string) string {
	return strings.ReplaceAll(value, "\x1b[?25h", "")
}

func fallback(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func clamp(value, minValue, maxValue int) int {
	return min(max(value, minValue), maxValue)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var agentOrder = []string{"codex", "cc", "amp", "orchestrator"}

var (
	titleBar = lipgloss.NewStyle().
			Padding(0, 1).
			Background(lipgloss.Color("236"))
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("230"))
	strongStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("230"))
	subtleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("246"))
	mutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
	emptyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")).
			Italic(true)
	sectionTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("81"))
	listBox = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, true, false, false).
		BorderForeground(lipgloss.Color("238")).
		Padding(1, 1)
	detailBox = lipgloss.NewStyle().
			Padding(1, 1)
	previewBox = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(lipgloss.Color("238")).
			Padding(1, 1)
	listItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))
	selectedItemStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("238"))
	okStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("42")).
		Bold(true)
	warnStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			Bold(true)
	failStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("203")).
			Bold(true)
	tabStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Padding(0, 1)
	activeTabStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("24")).
			Padding(0, 1)
	footer = lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Background(lipgloss.Color("236")).
		Padding(0, 1)
)
