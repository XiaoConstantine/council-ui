package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/XiaoConstantine/council-ui/internal/protocol"
	"github.com/XiaoConstantine/council-ui/internal/tmux"
)

type Options struct {
	Home         string
	ProjectRoots []string
	Load         protocol.LoadOptions
	Refresh      time.Duration
}

type Model struct {
	opts            Options
	tmux            tmux.Client
	choosingProject bool
	projects        []protocol.Project
	selectedProject int
	runs            []protocol.Run
	councils        []tmux.Council
	panes           []tmux.Pane
	selectedRun     int
	selectedAgent   int
	selectedSection int
	artifactScroll  int
	artifactModal   bool
	modalScroll     int
	expanded        map[string]bool
	width           int
	height          int
	filter          string
	filtering       bool
	preview         string
	previewErr      error
	err             error
	projectErr      error
	tmuxErr         error
	status          string
	loadedAt        time.Time
}

type projectsMsg struct {
	projects []protocol.Project
	err      error
}

type refreshMsg struct {
	runs       []protocol.Run
	councils   []tmux.Council
	panes      []tmux.Pane
	preview    string
	previewErr error
	err        error
	tmuxErr    error
	loadedAt   time.Time
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
		opts:            opts,
		tmux:            tmux.Client{},
		choosingProject: opts.Home == "",
		selectedAgent:   0,
		expanded: map[string]bool{
			"plan":      true,
			"execution": true,
			"reviews":   true,
		},
	}
}

func (m Model) Init() tea.Cmd {
	if m.choosingProject {
		return m.discoverProjectsCmd()
	}
	return tea.Batch(m.refreshCmd(), tick(m.opts.Refresh))
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tickMsg:
		if m.choosingProject {
			return m, nil
		}
		return m, tea.Batch(m.refreshCmd(), tick(m.opts.Refresh))
	case projectsMsg:
		m.projects = msg.projects
		m.projectErr = msg.err
		if m.selectedProject >= len(m.projects) {
			m.selectedProject = max(0, len(m.projects)-1)
		}
		return m, nil
	case refreshMsg:
		m.runs = msg.runs
		m.councils = msg.councils
		m.panes = msg.panes
		m.preview = msg.preview
		m.previewErr = msg.previewErr
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
		if m.choosingProject {
			return m.updateProjectPicker(msg)
		}
		if m.artifactModal {
			return m.updateArtifactModal(msg)
		}
		if m.filtering {
			return m.updateFilter(msg)
		}
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "j", "down":
			if m.selectedRun < len(m.visibleRuns())-1 {
				m.selectedRun++
				m.artifactScroll = 0
			}
			return m, m.refreshCmd()
		case "k", "up":
			if m.selectedRun > 0 {
				m.selectedRun--
				m.artifactScroll = 0
			}
			return m, m.refreshCmd()
		case "tab":
			m.selectedAgent = (m.selectedAgent + 1) % len(agentOrder)
			m.artifactScroll = 0
			return m, m.refreshCmd()
		case "shift+tab":
			m.selectedAgent--
			if m.selectedAgent < 0 {
				m.selectedAgent = len(agentOrder) - 1
			}
			m.artifactScroll = 0
			return m, m.refreshCmd()
		case "right", "l":
			m.selectedSection = min(m.selectedSection+1, len(sectionOrder)-1)
			m.artifactScroll = 0
			return m, nil
		case "left", "h":
			m.selectedSection = max(0, m.selectedSection-1)
			m.artifactScroll = 0
			return m, nil
		case "pgdown", "ctrl+d":
			m.artifactScroll += 10
			return m, nil
		case "pgup", "ctrl+u":
			m.artifactScroll = max(0, m.artifactScroll-10)
			return m, nil
		case "home":
			m.artifactScroll = 0
			return m, nil
		case "end":
			m.artifactScroll = 1 << 30
			return m, nil
		case "o":
			m.artifactModal = true
			m.modalScroll = m.artifactScroll
			return m, nil
		case " ":
			section := sectionOrder[m.selectedSection]
			m.expanded[section] = !m.expanded[section]
			return m, nil
		case "enter":
			pane, ok := m.selectedPane()
			if !ok {
				m.status = "no live pane for selection"
				return m, nil
			}
			return m, m.switchCmd(pane)
		case "r":
			return m, m.refreshCmd()
		case "P":
			m.choosingProject = true
			m.status = ""
			return m, m.discoverProjectsCmd()
		case "/":
			m.filtering = true
			return m, nil
		}
	}
	return m, nil
}

func (m Model) updateProjectPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "j", "down":
		if m.selectedProject < len(m.projects)-1 {
			m.selectedProject++
		}
	case "k", "up":
		if m.selectedProject > 0 {
			m.selectedProject--
		}
	case "r":
		return m, m.discoverProjectsCmd()
	case "enter":
		if len(m.projects) == 0 {
			return m, nil
		}
		project := m.projects[m.selectedProject]
		m.opts.Home = project.Home
		m.choosingProject = false
		m.selectedRun = 0
		m.artifactScroll = 0
		m.filter = ""
		m.filtering = false
		m.status = "loaded " + project.Name
		return m, tea.Batch(m.refreshCmd(), tick(m.opts.Refresh))
	}
	return m, nil
}

func (m Model) updateArtifactModal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "o":
		m.artifactModal = false
		m.artifactScroll = m.modalScroll
	case "j", "down":
		m.modalScroll++
	case "k", "up":
		m.modalScroll = max(0, m.modalScroll-1)
	case "pgdown", "ctrl+d":
		m.modalScroll += max(8, m.height/2)
	case "pgup", "ctrl+u":
		m.modalScroll = max(0, m.modalScroll-max(8, m.height/2))
	case "home", "g":
		m.modalScroll = 0
	case "end", "G":
		m.modalScroll = 1 << 30
	case "right", "l":
		m.selectedSection = min(m.selectedSection+1, len(sectionOrder)-1)
		m.modalScroll = 0
	case "left", "h":
		m.selectedSection = max(0, m.selectedSection-1)
		m.modalScroll = 0
	case "tab":
		m.selectedAgent = (m.selectedAgent + 1) % len(agentOrder)
		m.modalScroll = 0
	case "shift+tab":
		m.selectedAgent--
		if m.selectedAgent < 0 {
			m.selectedAgent = len(agentOrder) - 1
		}
		m.modalScroll = 0
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
		m.artifactScroll = 0
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

	if m.choosingProject {
		top := m.renderProjectTop()
		bottom := footer.Width(m.width).Render("j/k move  enter load  r rescan  q quit")
		bodyHeight := max(8, m.height-lipgloss.Height(top)-lipgloss.Height(bottom))
		body := projectBox.Width(m.width).Height(bodyHeight).Render(m.renderProjectPicker(m.width, bodyHeight))
		return lipgloss.JoinVertical(lipgloss.Left, top, body, bottom)
	}
	if m.artifactModal {
		return m.renderArtifactModal(m.width, m.height)
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
		var previewErr error
		if pane, ok := next.selectedPane(); ok {
			capture, captureErr := client.CapturePane(context.Background(), pane.ID, 120)
			if captureErr == nil {
				preview = capture
			} else {
				previewErr = captureErr
			}
		}

		return refreshMsg{
			runs:       runs,
			councils:   councils,
			panes:      panes,
			preview:    preview,
			previewErr: previewErr,
			err:        err,
			tmuxErr:    tmuxErr,
			loadedAt:   time.Now(),
		}
	}
}

func (m Model) discoverProjectsCmd() tea.Cmd {
	return func() tea.Msg {
		projects, err := protocol.DiscoverProjects(m.opts.ProjectRoots)
		return projectsMsg{projects: projects, err: err}
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

func (m Model) renderProjectTop() string {
	subtitle := "choose a project with council-out"
	if len(m.opts.ProjectRoots) > 0 {
		subtitle = "scan " + strings.Join(m.opts.ProjectRoots, ", ")
	}
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
	mode := "j/k runs  h/l sections  space expand  tab agent  o open artifact  ctrl+d/u scroll  enter switch  / filter  P projects  q quit"
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

func (m Model) renderProjectPicker(width, height int) string {
	var lines []string
	lines = append(lines, sectionTitle.Render("Projects"))
	lines = append(lines, subtleStyle.Render("Select the workspace whose council-out should be loaded."))
	lines = append(lines, "")

	if m.projectErr != nil {
		lines = append(lines, warnStyle.Render("Some scan paths could not be read: "+m.projectErr.Error()))
		lines = append(lines, "")
	}
	if len(m.projects) == 0 {
		lines = append(lines, emptyStyle.Render("No projects with council-out/runs found."))
		lines = append(lines, "")
		lines = append(lines, "Run from a parent directory, or pass:")
		lines = append(lines, mutedStyle.Render("  council-ui --projects-root /path/to/repos"))
		lines = append(lines, mutedStyle.Render("  council-ui --workspace /path/to/project"))
		lines = append(lines, mutedStyle.Render("  council-ui --home /path/to/project/council-out"))
		return strings.Join(fitLines(lines, height-1), "\n")
	}

	for i, project := range m.projects {
		prefix := "  "
		style := listItemStyle
		if i == m.selectedProject {
			prefix = "▸ "
			style = selectedItemStyle
		}
		title := fmt.Sprintf("%s%s  %d runs  %s", prefix, project.Name, project.Runs, project.UpdatedAt.Format("2006-01-02 15:04"))
		lines = append(lines, style.Width(width-4).Render(truncate(title, width-6)))
		lines = append(lines, mutedStyle.Render("    "+truncate(project.Workspace, width-8)))
		lines = append(lines, "")
	}

	return strings.Join(fitLines(lines, height-1), "\n")
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
	lines = append(lines, "")
	lines = append(lines, m.renderArtifactSummary(run, width)...)
	lines = append(lines, "")
	lines = append(lines, m.renderSections(run, width)...)
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
	reviewDone := false
	if len(run.Artifacts.ReviewRounds) > 0 {
		for _, round := range run.Artifacts.ReviewRounds {
			if round.CC && round.AMP {
				reviewDone = true
			}
		}
	}
	return []string{
		sectionTitle.Render("Progress"),
		truncate(fmt.Sprintf("%s plans %d/3   %s critiques %d/3   %s final",
			mark(plansDone == 3), plansDone,
			mark(critiquesDone == 3), critiquesDone,
			mark(run.Artifacts.FinalPlan),
		), width-4),
		truncate(fmt.Sprintf("%s implementation   %s review",
			mark(run.Artifacts.Implementation),
			mark(reviewDone),
		), width-4),
	}
}

func (m Model) renderArtifactSummary(run protocol.Run, width int) []string {
	reviewStatus := "pending"
	if run.Verdicts.CC != "" || run.Verdicts.AMP != "" {
		reviewStatus = fmt.Sprintf("cc=%s amp=%s", fallback(run.Verdicts.CC, "-"), fallback(run.Verdicts.AMP, "-"))
	}

	return []string{
		truncate(fmt.Sprintf("%s  %s final plan   %s implementation",
			sectionTitle.Render("Artifacts"),
			plainMark(run.Artifacts.FinalPlan),
			plainMark(run.Artifacts.Implementation),
		), width-4),
		truncate(fmt.Sprintf("           %s reviews %s", plainMark(reviewStatus != "pending"), reviewStatus), width-4),
	}
}

func (m Model) renderSections(run protocol.Run, width int) []string {
	lines := []string{sectionTitle.Render("Sections")}
	for i, section := range sectionOrder {
		style := sectionRowStyle
		cursor := "  "
		if i == m.selectedSection {
			style = selectedSectionStyle
			cursor = "▸ "
		}
		icon := "▸"
		if m.expanded[section] {
			icon = "▾"
		}
		line := fmt.Sprintf("%s%s %s  %s", cursor, icon, sectionLabel(section), sectionStatus(run, section))
		lines = append(lines, style.Width(width-4).Render(truncate(line, width-6)))
		if m.expanded[section] {
			lines = append(lines, sectionChildren(run, section, width)...)
		}
	}
	return lines
}

func sectionChildren(run protocol.Run, section string, width int) []string {
	var rows []string
	switch section {
	case "plan":
		for _, row := range []struct {
			label string
			done  bool
		}{
			{"codex plan", run.Artifacts.Plans["codex"]},
			{"cc plan", run.Artifacts.Plans["cc"]},
			{"amp plan", run.Artifacts.Plans["amp"]},
			{"codex critique", run.Artifacts.Critiques["codex"]},
			{"cc critique", run.Artifacts.Critiques["cc"]},
			{"amp critique", run.Artifacts.Critiques["amp"]},
			{"final plan", run.Artifacts.FinalPlan},
		} {
			rows = append(rows, mutedStyle.Render(truncate(fmt.Sprintf("     %s %s", plainMark(row.done), row.label), width-8)))
		}
	case "execution":
		rows = append(rows, mutedStyle.Render(truncate(fmt.Sprintf("     %s implementation", plainMark(run.Artifacts.Implementation)), width-8)))
		if len(run.Artifacts.RevisionRounds) > 0 {
			for _, round := range run.Artifacts.RevisionRounds {
				rows = append(rows, mutedStyle.Render(truncate(fmt.Sprintf("     ● revise round %d", round), width-8)))
			}
		}
	case "reviews":
		if len(run.Artifacts.ReviewRounds) == 0 {
			rows = append(rows, mutedStyle.Render("     ○ no review rounds"))
			break
		}
		for _, round := range run.Artifacts.ReviewRounds {
			if !round.CC && !round.AMP {
				continue
			}
			rows = append(rows, mutedStyle.Render(truncate(fmt.Sprintf("     round %d  cc=%s  amp=%s", round.Round, plainMark(round.CC), plainMark(round.AMP)), width-8)))
		}
		if run.Verdicts.CC != "" || run.Verdicts.AMP != "" {
			rows = append(rows, mutedStyle.Render(truncate(fmt.Sprintf("     verdicts  cc=%s  amp=%s", fallback(run.Verdicts.CC, "-"), fallback(run.Verdicts.AMP, "-")), width-8)))
		}
	case "progress":
		start := max(0, len(run.Progress)-4)
		if len(run.Progress) == 0 {
			rows = append(rows, mutedStyle.Render("     no progress log"))
			break
		}
		for _, event := range run.Progress[start:] {
			rows = append(rows, mutedStyle.Render(truncate(fmt.Sprintf("     %s  %s", event.Time, event.Stage), width-8)))
		}
	}
	return rows
}

func sectionStatus(run protocol.Run, section string) string {
	switch section {
	case "plan":
		return fmt.Sprintf("%d/3 plans, %d/3 critiques, final %s",
			countDone(run.Artifacts.Plans),
			countDone(run.Artifacts.Critiques),
			yesNo(run.Artifacts.FinalPlan),
		)
	case "execution":
		if run.Artifacts.Implementation {
			return "implementation complete"
		}
		return "implementation pending"
	case "reviews":
		if run.Verdicts.CC != "" || run.Verdicts.AMP != "" {
			return fmt.Sprintf("cc=%s amp=%s", fallback(run.Verdicts.CC, "-"), fallback(run.Verdicts.AMP, "-"))
		}
		return "pending"
	case "progress":
		if len(run.Progress) == 0 {
			return "no events"
		}
		return run.Progress[len(run.Progress)-1].Stage
	default:
		return ""
	}
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
		return m.renderArtifactPreview(lines, width, height)
	}
	lines = append(lines, "")
	if m.previewErr != nil {
		lines = append(lines, warnStyle.Render("Pane preview error: "+m.previewErr.Error()))
		lines = append(lines, "")
	}
	if strings.TrimSpace(m.preview) == "" {
		lines = append(lines, emptyStyle.Render("Pane preview is empty."))
		return strings.Join(fitLines(lines, height-1), "\n")
	}
	for _, line := range tailLines(m.preview, max(3, height-5)) {
		lines = append(lines, truncate(stripControlNoise(line), width-4))
	}
	return strings.Join(fitLines(lines, height-1), "\n")
}

func (m Model) renderArtifactPreview(lines []string, width, height int) string {
	run, ok := m.selectedRunSnapshot()
	if !ok {
		lines = append(lines, "", emptyStyle.Render("No run selected."))
		return strings.Join(fitLines(lines, height-1), "\n")
	}

	doc := m.selectedArtifact(run)
	lines[0] = sectionTitle.Render("Artifact")
	lines = append(lines, m.renderSectionTabs(width))
	lines = append(lines, m.renderAgentTabs(width))
	lines = append(lines, mutedStyle.Render("o open popup  h/l section  tab agent  ctrl+d/u scroll"))
	lines = append(lines, "")
	if doc.Path == "" {
		lines = append(lines, emptyStyle.Render("No live pane and no artifact for this tab."))
		return strings.Join(fitLines(lines, height-1), "\n")
	}
	if doc.Err != nil {
		lines = append(lines, emptyStyle.Render("No live pane. Could not read "+doc.Label+"."))
		return strings.Join(fitLines(lines, height-1), "\n")
	}

	lines = append(lines, strongStyle.Render(doc.Label))
	lines = append(lines, mutedStyle.Render(shortPath(doc.Path)))
	lines = append(lines, "")
	visibleHeight := max(3, height-len(lines)-1)
	view, scroll, total := scrollLines(doc.Lines, m.artifactScroll, visibleHeight)
	lines = append(lines, mutedStyle.Render(fmt.Sprintf("lines %d-%d of %d", min(scroll+1, total), min(scroll+len(view), total), total)))
	for _, line := range view {
		lines = append(lines, truncate(line, width-4))
	}
	return strings.Join(fitLines(lines, height-1), "\n")
}

func (m Model) renderArtifactModal(width, height int) string {
	run, ok := m.selectedRunSnapshot()
	if !ok {
		return centerBox(width, height, modalBox.Width(max(20, width-4)).Height(max(8, height-3)).Render(emptyStyle.Render("No run selected.")))
	}
	doc := m.selectedArtifact(run)
	modalWidth := clamp(width-8, min(44, width), max(20, width-2))
	modalHeight := clamp(height-4, min(16, height), max(8, height-2))
	contentHeight := max(3, modalHeight-10)

	var lines []string
	title := fmt.Sprintf("Artifact · %s · %s · %s", sectionLabel(sectionOrder[m.selectedSection]), agentTabLabel(agentOrder[m.selectedAgent]), run.ID)
	lines = append(lines, titleStyle.Render(truncate(title, modalWidth-6)))
	lines = append(lines, m.renderSectionTabs(modalWidth-4))
	lines = append(lines, m.renderAgentTabs(modalWidth-4))
	lines = append(lines, mutedStyle.Render("esc/q close  h/l section  tab agent  j/k or ctrl+d/u scroll  g/G top/bottom"))
	lines = append(lines, "")

	if doc.Path == "" {
		lines = append(lines, emptyStyle.Render("No artifact for this section and agent."))
		return centerBox(width, height, modalBox.Width(modalWidth).Height(modalHeight).Render(strings.Join(lines, "\n")))
	}
	if doc.Err != nil {
		lines = append(lines, warnStyle.Render("Could not read "+doc.Label+": "+doc.Err.Error()))
		return centerBox(width, height, modalBox.Width(modalWidth).Height(modalHeight).Render(strings.Join(lines, "\n")))
	}

	lines = append(lines, strongStyle.Render(doc.Label))
	lines = append(lines, mutedStyle.Render(doc.Path))
	lines = append(lines, "")
	view, scroll, total := scrollLines(doc.Lines, m.modalScroll, contentHeight)
	lines = append(lines, mutedStyle.Render(fmt.Sprintf("lines %d-%d of %d", min(scroll+1, total), min(scroll+len(view), total), total)))
	for _, line := range view {
		lines = append(lines, truncate(line, modalWidth-6))
	}

	return centerBox(width, height, modalBox.Width(modalWidth).Height(modalHeight).Render(strings.Join(lines, "\n")))
}

func artifactPreviewPath(run protocol.Run, role string) (path string, label string) {
	switch role {
	case "codex":
		if run.Artifacts.Implementation {
			return filepath.Join(run.Dir, "implementation", "codex.md"), "implementation/codex.md"
		}
		if run.Artifacts.FinalPlan {
			return filepath.Join(run.Dir, "plan.final.md"), "plan.final.md"
		}
		if run.Artifacts.Plans["codex"] {
			return filepath.Join(run.Dir, "plans", "codex.md"), "plans/codex.md"
		}
	case "cc":
		if reviewPath := latestReviewPath(run, "cc"); reviewPath != "" {
			return reviewPath, strings.TrimPrefix(reviewPath, run.Dir+"/")
		}
		if run.Artifacts.Critiques["cc"] {
			return filepath.Join(run.Dir, "critiques", "cc.md"), "critiques/cc.md"
		}
		if run.Artifacts.Plans["cc"] {
			return filepath.Join(run.Dir, "plans", "cc.md"), "plans/cc.md"
		}
	case "amp":
		if reviewPath := latestReviewPath(run, "amp"); reviewPath != "" {
			return reviewPath, strings.TrimPrefix(reviewPath, run.Dir+"/")
		}
		if run.Artifacts.Critiques["amp"] {
			return filepath.Join(run.Dir, "critiques", "amp.md"), "critiques/amp.md"
		}
		if run.Artifacts.Plans["amp"] {
			return filepath.Join(run.Dir, "plans", "amp.md"), "plans/amp.md"
		}
	case "orchestrator":
		progress := filepath.Join(run.Dir, "progress.log")
		if fileExists(progress) {
			return progress, "progress.log"
		}
	}
	return "", ""
}

type artifactDocument struct {
	Path  string
	Label string
	Lines []string
	Err   error
}

func (m Model) selectedArtifact(run protocol.Run) artifactDocument {
	path, label := m.artifactPreviewPath(run)
	if path == "" {
		return artifactDocument{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return artifactDocument{Path: path, Label: label, Err: err}
	}
	return artifactDocument{
		Path:  path,
		Label: label,
		Lines: meaningfulLines(string(data)),
	}
}

func (m Model) artifactPreviewPath(run protocol.Run) (path string, label string) {
	section := sectionOrder[m.selectedSection]
	switch section {
	case "plan":
		if run.Artifacts.FinalPlan {
			return filepath.Join(run.Dir, "plan.final.md"), "plan.final.md"
		}
		if run.Artifacts.Critiques["codex"] {
			return filepath.Join(run.Dir, "critiques", "codex.md"), "critiques/codex.md"
		}
		return artifactPreviewPath(run, agentOrder[m.selectedAgent])
	case "execution":
		if run.Artifacts.Implementation {
			return filepath.Join(run.Dir, "implementation", "codex.md"), "implementation/codex.md"
		}
		return filepath.Join(run.Dir, "plan.final.md"), "plan.final.md"
	case "reviews":
		if reviewPath := latestReviewPath(run, agentOrder[m.selectedAgent]); reviewPath != "" {
			return reviewPath, strings.TrimPrefix(reviewPath, run.Dir+"/")
		}
		if reviewPath := latestReviewPath(run, "cc"); reviewPath != "" {
			return reviewPath, strings.TrimPrefix(reviewPath, run.Dir+"/")
		}
		if reviewPath := latestReviewPath(run, "amp"); reviewPath != "" {
			return reviewPath, strings.TrimPrefix(reviewPath, run.Dir+"/")
		}
	case "progress":
		progress := filepath.Join(run.Dir, "progress.log")
		if fileExists(progress) {
			return progress, "progress.log"
		}
	}
	return artifactPreviewPath(run, agentOrder[m.selectedAgent])
}

func latestReviewPath(run protocol.Run, role string) string {
	for i := len(run.Artifacts.ReviewRounds) - 1; i >= 0; i-- {
		round := run.Artifacts.ReviewRounds[i]
		if role == "cc" && round.CC {
			return filepath.Join(run.Dir, "reviews", fmt.Sprintf("cc.round-%d.md", round.Round))
		}
		if role == "amp" && round.AMP {
			return filepath.Join(run.Dir, "reviews", fmt.Sprintf("amp.round-%d.md", round.Round))
		}
	}
	return ""
}

func (m Model) renderAgentTabs(width int) string {
	var parts []string
	parts = append(parts, mutedStyle.Render("agent"))
	for i, role := range agentOrder {
		style := tabStyle
		if i == m.selectedAgent {
			style = activeTabStyle
		}
		parts = append(parts, style.Render(agentTabLabel(role)))
	}
	return strings.Join(parts, " ")
}

func (m Model) renderSectionTabs(width int) string {
	var parts []string
	parts = append(parts, mutedStyle.Render("section"))
	for i, section := range sectionOrder {
		style := tabStyle
		if i == m.selectedSection {
			style = activeTabStyle
		}
		parts = append(parts, style.Render(sectionLabel(section)))
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

func plainMark(done bool) string {
	if done {
		return "●"
	}
	return "○"
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

func headMeaningfulLines(value string, count int) []string {
	lines := meaningfulLines(value)
	if len(lines) <= count {
		return lines
	}
	return lines[:count]
}

func meaningfulLines(value string) []string {
	raw := strings.Split(strings.TrimRight(value, "\n"), "\n")
	lines := make([]string, 0, len(raw))
	blank := false
	for _, line := range raw {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if blank || len(lines) == 0 {
				continue
			}
			blank = true
			lines = append(lines, "")
		} else {
			blank = false
			lines = append(lines, line)
		}
	}
	return lines
}

func scrollLines(lines []string, offset int, height int) ([]string, int, int) {
	total := len(lines)
	if height <= 0 || total == 0 {
		return nil, 0, total
	}
	maxOffset := max(0, total-height)
	offset = clamp(offset, 0, maxOffset)
	end := min(total, offset+height)
	return lines[offset:end], offset, total
}

func centerBox(width, height int, content string) string {
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content, lipgloss.WithWhitespaceChars(" "))
}

func stripControlNoise(value string) string {
	return strings.ReplaceAll(value, "\x1b[?25h", "")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
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
var sectionOrder = []string{"plan", "execution", "reviews", "progress"}

func agentTabLabel(role string) string {
	if role == "orchestrator" {
		return "orch"
	}
	return role
}

func sectionLabel(section string) string {
	switch section {
	case "plan":
		return "Plan"
	case "execution":
		return "Execution"
	case "reviews":
		return "Reviews"
	case "progress":
		return "Progress"
	default:
		return section
	}
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

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
	projectBox = lipgloss.NewStyle().
			Padding(2, 4)
	detailBox = lipgloss.NewStyle().
			Padding(1, 1)
	previewBox = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(lipgloss.Color("238")).
			Padding(1, 1)
	modalBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("81")).
			Background(lipgloss.Color("235")).
			Padding(1, 2)
	listItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))
	selectedItemStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("238"))
	sectionRowStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))
	selectedSectionStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("24"))
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
