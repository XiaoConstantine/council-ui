package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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
	Workspace    string
	ProjectRoots []string
	CouncilCmd   string
	Load         protocol.LoadOptions
	Refresh      time.Duration
}

type commandButton struct {
	Label  string
	Action string
}

type councilCommand struct {
	Label     string
	Workspace string
	Args      []string
}

type artifactRow struct {
	Key          string
	Label        string
	RelativePath string
	Path         string
	Exists       bool
	Bytes        int64
}

type uiLayout struct {
	headerHeight      int
	leftWidth         int
	leftOuterWidth    int
	rightWidth        int
	mainHeight        int
	sideHeight        int
	runBlockHeight    int
	runRows           int
	runFirstRowY      int
	paneBlockHeight   int
	paneRows          int
	paneFirstRowY     int
	artifactRows      int
	artifactFirstRowY int
}

type Model struct {
	opts                  Options
	tmux                  tmux.Client
	choosingProject       bool
	projects              []protocol.Project
	selectedProject       int
	runs                  []protocol.Run
	councils              []tmux.Council
	panes                 []tmux.Pane
	selectedRun           int
	runScroll             int
	focus                 string
	selectedArtifactIndex int
	selectedPaneIndex     int
	artifactListScroll    int
	paneScroll            int
	selectedAgent         int
	selectedSection       int
	artifactScroll        int
	artifactModal         bool
	modalScroll           int
	zoom                  bool
	zoomKind              string
	zoomScroll            int
	panePreview           []string
	panePreviewErr        string
	panePreviewFor        string
	confirmReset          bool
	actionLog             []string
	expanded              map[string]bool
	width                 int
	height                int
	filter                string
	filtering             bool
	enteringGoal          bool
	goalInput             string
	preview               string
	previewErr            error
	err                   error
	projectErr            error
	tmuxErr               error
	status                string
	loadedAt              time.Time
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
	previewFor string
	previewErr error
	err        error
	tmuxErr    error
	loadedAt   time.Time
}

type switchMsg struct {
	err error
}

type actionDoneMsg struct {
	action string
	output string
	err    error
}

type tickMsg struct{}

var commandButtons = []commandButton{
	{Label: "Start", Action: "start"},
	{Label: "Attach", Action: "attach"},
	{Label: "Resume", Action: "resume"},
	{Label: "Exec", Action: "exec"},
	{Label: "Cancel", Action: "cancel"},
	{Label: "Zoom", Action: "zoom"},
	{Label: "Reset", Action: "reset"},
	{Label: "Refresh", Action: "refresh"},
	{Label: "Quit", Action: "quit"},
}

func New(opts Options) Model {
	if opts.Refresh <= 0 {
		opts.Refresh = time.Second
	}
	if opts.CouncilCmd == "" {
		opts.CouncilCmd = "council"
	}
	return Model{
		opts:                  opts,
		tmux:                  tmux.Client{},
		choosingProject:       opts.Home == "",
		focus:                 "runs",
		selectedArtifactIndex: -1,
		selectedPaneIndex:     -1,
		selectedAgent:         0,
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
		m.panePreview = strings.Split(strings.ReplaceAll(msg.preview, "\r\n", "\n"), "\n")
		m.panePreviewFor = msg.previewFor
		m.panePreviewErr = ""
		if msg.previewErr != nil {
			m.panePreviewErr = msg.previewErr.Error()
		}
		m.err = msg.err
		m.tmuxErr = msg.tmuxErr
		m.loadedAt = msg.loadedAt
		if m.selectedRun >= len(m.visibleRuns()) {
			m.selectedRun = max(0, len(m.visibleRuns())-1)
		}
		m.reconcileSelections()
		return m, nil
	case switchMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
		} else {
			m.status = "switched to pane"
		}
		return m, nil
	case actionDoneMsg:
		line := msg.action + ": ok"
		if msg.err != nil {
			line = msg.action + ": " + msg.err.Error()
		}
		if strings.TrimSpace(msg.output) != "" {
			line += " | " + strings.Join(strings.Fields(msg.output), " ")
		}
		m.status = line
		m.actionLog = append([]string{line}, m.actionLog...)
		if len(m.actionLog) > 4 {
			m.actionLog = m.actionLog[:4]
		}
		m.confirmReset = false
		return m, m.refreshCmd()
	case tea.KeyMsg:
		if m.choosingProject {
			return m.updateProjectPicker(msg)
		}
		if m.zoom {
			return m.updateZoomKey(msg)
		}
		if m.enteringGoal {
			return m.updateGoalInput(msg)
		}
		if m.filtering {
			return m.updateFilter(msg)
		}
		if m.confirmReset {
			switch msg.String() {
			case "y", "Y":
				if cmd, ok := m.councilCommandForAction("reset"); ok {
					return m, m.actionCmd(cmd)
				}
				return m, nil
			case "n", "N", "esc":
				m.confirmReset = false
				m.status = "reset cancelled"
				return m, nil
			}
		}
		return m.updateDashboardKey(msg)
	case tea.MouseMsg:
		return m.updateMouse(msg)
	}
	return m, nil
}

func (m Model) updateZoomKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "z", "o":
		m.zoom = false
		m.zoomKind = ""
		m.zoomScroll = 0
	case "j", "down":
		m.zoomScroll++
	case "k", "up":
		m.zoomScroll = max(0, m.zoomScroll-1)
	case "pgdown", "ctrl+d", " ":
		m.zoomScroll += max(1, m.height-6)
	case "pgup", "ctrl+u":
		m.zoomScroll = max(0, m.zoomScroll-max(1, m.height-6))
	case "g", "home":
		m.zoomScroll = 0
	case "G", "end":
		m.zoomScroll = 1 << 30
	}
	return m, nil
}

func (m Model) updateGoalInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.enteringGoal = false
		m.goalInput = ""
		m.status = "start cancelled"
	case "enter":
		goal := strings.TrimSpace(m.goalInput)
		if goal == "" {
			m.status = "goal is required"
			return m, nil
		}
		m.enteringGoal = false
		m.goalInput = ""
		return m, m.actionCmd(m.councilRunCommand(goal))
	case "backspace", "ctrl+h":
		if len(m.goalInput) > 0 {
			m.goalInput = m.goalInput[:len(m.goalInput)-1]
		}
	case "ctrl+u":
		m.goalInput = ""
	case " ":
		m.goalInput += " "
	default:
		if msg.Type == tea.KeyRunes {
			m.goalInput += string(msg.Runes)
		}
	}
	return m, nil
}

func (m Model) updateDashboardKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "tab":
		m.focus = nextFocus(m.focus)
	case "/":
		m.filtering = true
	case "r":
		return m.triggerAction("resume")
	case "e":
		return m.triggerAction("exec")
	case "c":
		return m.triggerAction("cancel")
	case "s":
		return m.triggerAction("start")
	case "a":
		return m.triggerAction("attach")
	case "R":
		return m.triggerAction("reset")
	case "z", "o", "enter":
		return m.triggerAction("zoom")
	case "ctrl+r":
		return m.triggerAction("refresh")
	case "P":
		m.choosingProject = true
		m.status = ""
		return m, m.discoverProjectsCmd()
	case "j", "down":
		if m.focus == "runs" {
			m.selectRun(m.selectedRun + 1)
			return m, m.refreshCmd()
		}
		if m.focus == "panes" {
			m.selectPane(m.selectedPaneIndex + 1)
			return m, m.refreshCmd()
		}
		m.selectArtifact(m.selectedArtifactIndex + 1)
	case "k", "up":
		if m.focus == "runs" {
			m.selectRun(m.selectedRun - 1)
			return m, m.refreshCmd()
		}
		if m.focus == "panes" {
			m.selectPane(m.selectedPaneIndex - 1)
			return m, m.refreshCmd()
		}
		m.selectArtifact(m.selectedArtifactIndex - 1)
	case "g":
		if m.focus == "runs" {
			m.selectRun(0)
			return m, m.refreshCmd()
		}
		if m.focus == "panes" {
			m.selectPane(0)
			return m, m.refreshCmd()
		}
		m.selectArtifact(0)
	case "G":
		if m.focus == "runs" {
			m.selectRun(len(m.visibleRuns()) - 1)
			return m, m.refreshCmd()
		}
		if m.focus == "panes" {
			m.selectPane(len(m.visiblePanes()) - 1)
			return m, m.refreshCmd()
		}
		m.selectArtifact(len(m.visibleArtifacts()) - 1)
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
		m.opts.Workspace = project.Workspace
		m.choosingProject = false
		m.selectRun(0)
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
		m.selectRun(0)
		m.artifactScroll = 0
	case "backspace":
		if len(m.filter) > 0 {
			m.filter = m.filter[:len(m.filter)-1]
			m.selectRun(0)
		}
	case "ctrl+u":
		m.filter = ""
		m.selectRun(0)
	default:
		if len(msg.String()) == 1 {
			m.filter += msg.String()
			m.selectRun(0)
		}
	}
	return m, m.refreshCmd()
}

func (m Model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}
	if msg.Button == tea.MouseButtonWheelUp {
		if m.zoom {
			m.zoomScroll = max(0, m.zoomScroll-3)
		}
		return m, nil
	}
	if msg.Button == tea.MouseButtonWheelDown {
		if m.zoom {
			m.zoomScroll += 3
		}
		return m, nil
	}
	if msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	if m.zoom {
		m.zoom = false
		m.zoomScroll = 0
		return m, nil
	}
	if msg.Y == 1 {
		if action := m.commandButtonAt(msg.X); action != "" {
			return m.triggerAction(action)
		}
	}

	layout := m.layout()
	if msg.X < layout.leftOuterWidth {
		runOffset := scrollForSelection(m.runScroll, m.selectedRun, len(m.visibleRuns()), layout.runRows)
		idx := msg.Y - layout.runFirstRowY
		if idx >= 0 && idx < layout.runRows && runOffset+idx < len(m.visibleRuns()) {
			m.focus = "runs"
			m.selectRun(runOffset + idx)
			return m, m.refreshCmd()
		}
		paneOffset := scrollForSelection(m.paneScroll, m.selectedPaneIndex, len(m.visiblePanes()), layout.paneRows)
		idx = msg.Y - layout.paneFirstRowY
		if idx >= 0 && idx < layout.paneRows && paneOffset+idx < len(m.visiblePanes()) {
			m.focus = "panes"
			m.selectPane(paneOffset + idx)
			return m, m.refreshCmd()
		}
		return m, nil
	}

	artifactOffset := scrollForSelection(m.artifactListScroll, m.selectedArtifactIndex, len(m.visibleArtifacts()), layout.artifactRows)
	idx := msg.Y - layout.artifactFirstRowY
	if idx >= 0 && idx < layout.artifactRows && artifactOffset+idx < len(m.visibleArtifacts()) {
		m.focus = "artifacts"
		m.selectArtifact(artifactOffset + idx)
	}
	return m, nil
}

func (m Model) triggerAction(action string) (tea.Model, tea.Cmd) {
	switch action {
	case "start":
		m.enteringGoal = true
		m.goalInput = ""
		m.status = "enter goal, then press Enter"
	case "attach":
		if m.focus == "panes" {
			if pane := m.currentPane(); pane != nil {
				return m, m.switchCmd(*pane)
			}
		}
		if cmd, ok := m.councilCommandForAction(action); ok {
			return m, m.actionCmd(cmd)
		}
		m.status = "no live pane or council instance to attach"
	case "zoom":
		if m.focus == "panes" && m.currentPane() != nil {
			m.zoom = true
			m.zoomKind = "pane"
			m.zoomScroll = 0
		} else if m.currentArtifact() != nil {
			m.zoom = true
			m.zoomKind = "artifact"
			m.zoomScroll = 0
		}
	case "cancel":
		pane, ok := m.orchestratorPane()
		if !ok {
			m.status = "no orchestrator pane to cancel"
			return m, nil
		}
		return m, m.cancelCmd(pane)
	case "reset":
		m.confirmReset = true
	case "refresh":
		return m, m.refreshCmd()
	case "quit":
		return m, tea.Quit
	default:
		if cmd, ok := m.councilCommandForAction(action); ok {
			return m, m.actionCmd(cmd)
		}
	}
	return m, nil
}

func (m Model) actionCmd(cmd councilCommand) tea.Cmd {
	return func() tea.Msg {
		workspace := cmd.Workspace
		if workspace == "" {
			workspace = m.currentWorkspace()
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		process := exec.CommandContext(ctx, m.opts.CouncilCmd, cmd.Args...)
		if workspace != "" {
			process.Dir = workspace
		}
		process.Env = m.actionEnv()
		out, err := process.CombinedOutput()
		return actionDoneMsg{action: cmd.Label, output: string(out), err: err}
	}
}

func (m Model) actionEnv() []string {
	env := os.Environ()
	if m.opts.Home != "" {
		env = append(env, "MAESTRO_COUNCIL_HOME="+m.opts.Home)
	}
	return env
}

func (m Model) cancelCmd(pane tmux.Pane) tea.Cmd {
	return func() tea.Msg {
		err := m.tmux.SendKeys(context.Background(), pane.ID, "C-c")
		output := ""
		if err == nil {
			output = "sent Ctrl-C to " + paneDisplayName(pane)
		}
		return actionDoneMsg{action: "cancel", output: output, err: err}
	}
}

func (m Model) councilCommandForAction(action string) (councilCommand, bool) {
	instance := m.currentInstance()
	switch action {
	case "attach":
		return councilCommand{Label: "attach", Workspace: m.currentWorkspace(), Args: []string{"attach", "--instance", instance}}, true
	case "resume":
		run, ok := m.selectedRunSnapshot()
		if !ok {
			return councilCommand{}, false
		}
		return councilCommand{Label: "resume", Workspace: run.Workspace, Args: []string{"resume", run.ID}}, true
	case "exec":
		run, ok := m.selectedRunSnapshot()
		if !ok {
			return councilCommand{}, false
		}
		return councilCommand{Label: "exec", Workspace: run.Workspace, Args: []string{"exec", run.ID}}, true
	case "reset":
		return councilCommand{Label: "reset", Workspace: m.currentWorkspace(), Args: []string{"reset", "--instance", instance}}, true
	default:
		return councilCommand{}, false
	}
}

func (m Model) councilRunCommand(goal string) councilCommand {
	return councilCommand{
		Label:     "run",
		Workspace: m.currentWorkspace(),
		Args:      []string{"run", "--instance", m.currentInstance(), "--", goal},
	}
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
	if m.zoom {
		return m.zoomView()
	}

	header := m.headerBlockWithStateView()
	layout := m.layoutForHeader(header)

	left := borderStyle.Width(layout.leftWidth).Height(layout.mainHeight).Render(m.sideView(layout.leftWidth-2, layout.sideHeight))
	right := borderStyle.Width(layout.rightWidth).Height(layout.mainHeight).Render(m.detailControlView(layout.rightWidth-2, layout.sideHeight))
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	return lipgloss.JoinVertical(lipgloss.Left, header, body, m.footerControlView())
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
		next.reconcileSelections()
		if pane := next.currentPane(); pane != nil {
			capture, captureErr := client.CapturePane(context.Background(), pane.ID, 120)
			if captureErr == nil {
				preview = capture
			} else {
				previewErr = captureErr
			}
			return refreshMsg{
				runs:       runs,
				councils:   councils,
				panes:      panes,
				preview:    preview,
				previewFor: pane.ID,
				previewErr: previewErr,
				err:        err,
				tmuxErr:    tmuxErr,
				loadedAt:   time.Now(),
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

func (m Model) headerControlView() string {
	health := okStyle.Render("tmux idle")
	if len(m.panes) > 0 {
		health = okStyle.Render(fmt.Sprintf("%d pane(s)", len(m.panes)))
	}
	parts := []string{
		titleStyle.Render("council-ui"),
		"workspace " + mutedStyle.Render(m.currentWorkspace()),
		"instance " + keyStyle.Render(m.currentInstance()),
		health,
	}
	return truncate(strings.Join(parts, "  "), m.width)
}

func (m Model) headerBlockView() string {
	return m.headerControlView() + "\n" + m.renderCommandBar()
}

func (m Model) headerBlockWithStateView() string {
	header := m.headerBlockView()
	if m.filtering {
		header += "\n" + keyStyle.Render("filter: ") + m.filter
	}
	if m.enteringGoal {
		header += "\n" + keyStyle.Render("goal: ") + m.goalInput
	}
	if m.confirmReset {
		header += "\n" + warnStyle.Render("Reset selected instance? y/N")
	}
	if m.status != "" {
		header += "\n" + mutedStyle.Render(m.status)
	}
	if m.err != nil {
		header += "\n" + failStyle.Render(m.err.Error())
	}
	if m.tmuxErr != nil {
		header += "\n" + warnStyle.Render("tmux: "+m.tmuxErr.Error())
	}
	return header
}

func (m Model) layout() uiLayout {
	return m.layoutForHeader(m.headerBlockWithStateView())
}

func (m Model) layoutForHeader(header string) uiLayout {
	headerHeight := lipgloss.Height(header)
	leftWidth := max(34, m.width/3)
	rightWidth := max(40, m.width-leftWidth-3)
	mainHeight := max(12, m.height-headerHeight-5)
	sideHeight := max(1, mainHeight-2)
	runBlockHeight := max(4, sideHeight/2)
	if sideHeight > 4 && runBlockHeight > sideHeight-4 {
		runBlockHeight = sideHeight - 4
	}
	runBlockHeight = max(2, min(runBlockHeight, sideHeight))
	paneBlockHeight := max(1, sideHeight-runBlockHeight-1)
	return uiLayout{
		headerHeight:      headerHeight,
		leftWidth:         leftWidth,
		leftOuterWidth:    leftWidth + 2,
		rightWidth:        rightWidth,
		mainHeight:        mainHeight,
		sideHeight:        sideHeight,
		runBlockHeight:    runBlockHeight,
		runRows:           listRowsForBlock(runBlockHeight),
		runFirstRowY:      headerHeight + 2,
		paneBlockHeight:   paneBlockHeight,
		paneRows:          listRowsForBlock(paneBlockHeight),
		paneFirstRowY:     headerHeight + runBlockHeight + 2,
		artifactRows:      max(1, sideHeight/3),
		artifactFirstRowY: headerHeight + 10,
	}
}

func (m Model) renderTop() string {
	live := fmt.Sprintf("%d live panes", len(m.panes))
	if m.tmuxErr != nil {
		live = "tmux unavailable"
	}
	subtitle := fmt.Sprintf("%s  %s  %s", shortPath(m.opts.Home), live, m.loadedAt.Format("15:04:05"))
	title := titleBar.Width(m.width).Render(
		lipgloss.JoinHorizontal(
			lipgloss.Center,
			titleStyle.Render("council-ui"),
			" ",
			subtleStyle.Render(subtitle),
		),
	)
	return lipgloss.JoinVertical(lipgloss.Left, title, m.renderCommandBar())
}

func (m Model) renderCommandBar() string {
	return commandBar.Width(m.width).Render(truncate("Actions "+m.commandBarPlain(), m.width))
}

func (m Model) commandBarPlain() string {
	parts := make([]string, 0, len(commandButtons))
	for _, button := range commandButtons {
		parts = append(parts, "["+button.Label+"]")
	}
	return strings.Join(parts, " ")
}

func (m Model) commandButtonAt(x int) string {
	pos := len("Actions ")
	if x < pos {
		return ""
	}
	for _, button := range commandButtons {
		token := "[" + button.Label + "]"
		if x >= pos && x < pos+len(token) {
			return button.Action
		}
		pos += len(token) + 1
	}
	return ""
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
	mode := "j/k runs  h/l sections  space expand  tab agent  o/z zoom  enter switch  s start  a attach  u resume  e exec  R reset  r refresh  / filter  P projects  q quit"
	if m.filtering {
		mode = "filter: " + m.filter + "  enter apply  esc close  ctrl+u clear"
	} else if m.filter != "" {
		mode += "  filter: " + m.filter
	}
	if m.confirmReset {
		mode = "Reset selected instance? y/N"
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
		lines = append(lines, emptyStyle.Render("No project workspaces found."))
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

func (m Model) sideView(width, height int) string {
	runHeight := max(4, height/2)
	if height > 4 && runHeight > height-4 {
		runHeight = height - 4
	}
	runHeight = max(2, min(runHeight, height))
	paneHeight := max(1, height-runHeight-1)
	runBlock := fitStringLines(m.runsControlView(width, runHeight), runHeight)
	paneBlock := fitStringLines(m.panesControlView(width, paneHeight), paneHeight)
	return runBlock + "\n" + paneBlock
}

func (m Model) runsControlView(width, height int) string {
	var b strings.Builder
	runs := m.visibleRuns()
	limit := listRowsForBlock(height)
	offset := scrollForSelection(m.runScroll, m.selectedRun, len(runs), limit)
	b.WriteString(sectionTitle.Render(listTitle("Runs", m.focus == "runs", offset, limit, len(runs))))
	if m.filter != "" {
		b.WriteString(" " + mutedStyle.Render("filter="+m.filter))
	}
	b.WriteString("\n")
	if len(runs) == 0 {
		b.WriteString(emptyStyle.Render(truncate("No runs yet. Press s or Start to enter a goal.", width)))
		return b.String()
	}
	idWidth := min(20, max(10, width/2))
	statusWidth := 8
	taskWidth := max(0, width-idWidth-statusWidth-3)
	for row := 0; row < limit && offset+row < len(runs); row++ {
		i := offset + row
		run := runs[i]
		line := fmt.Sprintf("%-*s %-*s %s",
			idWidth,
			truncate(run.ID, idWidth),
			statusWidth,
			truncate(rawStatusText(run.Status), statusWidth),
			truncate(oneLine(run.Task), taskWidth),
		)
		if i == m.selectedRun {
			line = selectedItemStyle.Render(pad(line, width))
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

func (m Model) panesControlView(width, height int) string {
	var b strings.Builder
	panes := m.visiblePanes()
	limit := listRowsForBlock(height)
	offset := scrollForSelection(m.paneScroll, m.selectedPaneIndex, len(panes), limit)
	b.WriteString(sectionTitle.Render(listTitle("Panes", m.focus == "panes", offset, limit, len(panes))))
	b.WriteString("\n")
	if len(panes) == 0 {
		b.WriteString(emptyStyle.Render(truncate("No live council panes found.", width)))
		return b.String()
	}
	processWidth := min(8, max(4, width/5))
	sizeWidth := 7
	nameWidth := max(8, width-processWidth-sizeWidth-4)
	for row := 0; row < limit && offset+row < len(panes); row++ {
		i := offset + row
		pane := panes[i]
		active := " "
		if pane.Active {
			active = "*"
		}
		line := fmt.Sprintf("%s %-*s %-*s %*s",
			active,
			nameWidth,
			truncate(paneDisplayName(pane), nameWidth),
			processWidth,
			truncate(pane.Command, processWidth),
			sizeWidth,
			truncate(pane.Size, sizeWidth),
		)
		if i == m.selectedPaneIndex {
			line = selectedItemStyle.Render(pad(line, width))
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

func (m Model) detailControlView(width, height int) string {
	if m.focus == "panes" {
		return m.paneDetailControlView(width, height)
	}
	run, ok := m.selectedRunSnapshot()
	if !ok {
		var b strings.Builder
		b.WriteString(sectionTitle.Render("Selected Project"))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("Workspace: %s\n", truncate(m.currentWorkspace(), max(1, width-11))))
		b.WriteString(fmt.Sprintf("Home: %s\n\n", truncate(m.opts.Home, max(1, width-6))))
		b.WriteString(emptyStyle.Render(truncate("No runs yet. Use Start to enter a goal and create council panes.", width)))
		if len(m.actionLog) > 0 {
			b.WriteString("\n\n")
			b.WriteString(sectionTitle.Render("Action Log"))
			b.WriteString("\n")
			for _, line := range m.actionLog {
				b.WriteString(truncate(line, width))
				b.WriteString("\n")
			}
		}
		return b.String()
	}

	var b strings.Builder
	b.WriteString(sectionTitle.Render("Selected Run"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("Task: %s\n", truncate(oneLine(run.Task), width-6)))
	b.WriteString(fmt.Sprintf("Run: %s\n", keyStyle.Render(run.ID)))
	b.WriteString(fmt.Sprintf("Instance: %s\n", keyStyle.Render(run.Instance)))
	b.WriteString(fmt.Sprintf("Status: %s  Phase: %s\n", statusBadge(run.Status), truncate(run.Phase, max(1, width-18))))
	b.WriteString(fmt.Sprintf("Target: %s  Next: %s\n", truncate(run.Target, max(1, width/3)), keyStyle.Render(truncate(run.NextStage, max(1, width/2)))))
	b.WriteString(fmt.Sprintf("Action: %s\n\n", keyStyle.Render(truncate(nextRunAction(run), max(1, width-8)))))

	artifacts := m.visibleArtifacts()
	artifactRows := min(max(1, height/3), len(artifacts))
	artifactOffset := scrollForSelection(m.artifactListScroll, m.selectedArtifactIndex, len(artifacts), artifactRows)
	b.WriteString(sectionTitle.Render(listTitle("Artifacts", m.focus == "artifacts", artifactOffset, artifactRows, len(artifacts))))
	b.WriteString("\n")
	for row := 0; row < artifactRows && artifactOffset+row < len(artifacts); row++ {
		i := artifactOffset + row
		artifact := artifacts[i]
		markerText := "pending"
		marker := mutedStyle.Render(markerText)
		if artifact.Exists {
			markerText = fmt.Sprintf("%dB", artifact.Bytes)
			marker = okStyle.Render(markerText)
		}
		labelWidth := max(8, min(28, width-len(markerText)-1))
		line := fmt.Sprintf("%-*s %s", labelWidth, truncate(artifact.Label, labelWidth), marker)
		if i == m.selectedArtifactIndex {
			line = selectedItemStyle.Render(pad(line, width))
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(sectionTitle.Render("Preview"))
	b.WriteString("\n")
	preview := m.previewLines(max(1, height-lipgloss.Height(b.String())-2), width)
	if len(preview) == 0 {
		b.WriteString(emptyStyle.Render(truncate("Select an existing artifact and press z to zoom.", width)))
	} else {
		b.WriteString(strings.Join(preview, "\n"))
	}

	if len(m.actionLog) > 0 {
		b.WriteString("\n\n")
		b.WriteString(sectionTitle.Render("Action Log"))
		b.WriteString("\n")
		for _, line := range m.actionLog {
			b.WriteString(truncate(line, width))
			b.WriteString("\n")
		}
	}
	return b.String()
}

func (m Model) paneDetailControlView(width, height int) string {
	pane := m.currentPane()
	if pane == nil {
		return emptyStyle.Render(truncate("No pane selected.", width))
	}
	var b strings.Builder
	b.WriteString(sectionTitle.Render("Selected Pane"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("Name: %s  Target: %s\n", keyStyle.Render(paneDisplayName(*pane)), keyStyle.Render(pane.ID)))
	b.WriteString(fmt.Sprintf("Window: %s:%s  Size: %s  Process: %s\n", pane.Session, pane.Index, pane.Size, pane.Command))
	if pane.Workspace != "" {
		b.WriteString(fmt.Sprintf("Workspace: %s\n", truncate(pane.Workspace, width-11)))
	}
	b.WriteString(mutedStyle.Render(truncate("Press a to select this pane in tmux; c cancels via the orchestrator; z zooms output.", width)))
	b.WriteString("\n\n")
	if len(m.actionLog) > 0 {
		b.WriteString(sectionTitle.Render("Action Log"))
		b.WriteString("\n")
		for _, line := range m.actionLog[:min(2, len(m.actionLog))] {
			b.WriteString(truncate(line, width))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	b.WriteString(sectionTitle.Render("Pane Output"))
	b.WriteString("\n")
	if m.panePreviewErr != "" {
		b.WriteString(warnStyle.Render(truncate(m.panePreviewErr, width)))
		return b.String()
	}
	lines := m.panePreview
	if len(lines) == 0 {
		b.WriteString(emptyStyle.Render(truncate("No captured output.", width)))
		return b.String()
	}
	limit := max(1, height-lipgloss.Height(b.String())-1)
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	for _, line := range lines {
		b.WriteString(truncate(stripControlNoise(line), width))
		b.WriteString("\n")
	}
	return b.String()
}

func (m Model) footerControlView() string {
	if m.enteringGoal {
		return footer.Width(m.width).Render(truncate("Enter goal  enter run council  esc cancel  ctrl+u clear", m.width))
	}
	items := []string{
		keyStyle.Render("j/k") + " move",
		keyStyle.Render("tab") + " focus",
		keyStyle.Render("click") + " select",
		keyStyle.Render("z") + " zoom",
		keyStyle.Render("r") + " resume",
		keyStyle.Render("e") + " exec",
		keyStyle.Render("c") + " cancel",
		keyStyle.Render("s") + " start",
		keyStyle.Render("a") + " attach/select pane",
		keyStyle.Render("R") + " reset",
		keyStyle.Render("/") + " filter",
		keyStyle.Render("P") + " projects",
		keyStyle.Render("ctrl+r") + " refresh",
		keyStyle.Render("q") + " quit",
	}
	return footer.Width(m.width).Render(truncate(strings.Join(items, "  "), m.width))
}

func (m Model) renderRunList(width, height int) string {
	runs := m.visibleRuns()
	if len(runs) == 0 {
		return emptyStyle.Render("No council runs found.")
	}

	lines := make([]string, 0, height)
	visibleRows, hasRange := runListMetrics(height, len(runs))
	offset := scrollForSelection(m.runScroll, m.selectedRun, len(runs), visibleRows)
	if hasRange {
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("runs %d-%d/%d", offset+1, min(len(runs), offset+visibleRows), len(runs))))
	}
	for row := 0; row < visibleRows && offset+row < len(runs); row++ {
		i := offset + row
		run := runs[i]
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
	if len(m.actionLog) > 0 {
		lines = append(lines, "")
		lines = append(lines, sectionTitle.Render("Actions"))
		for _, line := range m.actionLog {
			lines = append(lines, mutedStyle.Render(truncate("  "+line, width-4)))
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
		return m.renderArtifactPreview(width, height)
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

func (m Model) renderArtifactPreview(width, height int) string {
	run, ok := m.selectedRunSnapshot()
	lines := []string{sectionTitle.Render("Artifact")}
	if !ok {
		lines = append(lines, "", emptyStyle.Render("No run selected."))
		return strings.Join(fitLines(lines, height-1), "\n")
	}

	doc := m.selectedArtifact(run)
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

func (m *Model) selectRun(index int) {
	runs := m.visibleRuns()
	if len(runs) == 0 {
		m.selectedRun = 0
		m.selectedArtifactIndex = -1
		m.runScroll = 0
		m.artifactListScroll = 0
		return
	}
	if index != m.selectedRun {
		m.selectedArtifactIndex = -1
		m.artifactListScroll = 0
	}
	m.selectedRun = clamp(index, 0, len(runs)-1)
	m.ensureArtifactSelection()
	m.keepRunVisible()
}

func (m *Model) keepRunVisible() {
	if m.height == 0 {
		return
	}
	layout := m.layout()
	m.runScroll = scrollForSelection(m.runScroll, m.selectedRun, len(m.visibleRuns()), layout.runRows)
	m.artifactListScroll = scrollForSelection(m.artifactListScroll, m.selectedArtifactIndex, len(m.visibleArtifacts()), layout.artifactRows)
	m.paneScroll = scrollForSelection(m.paneScroll, m.selectedPaneIndex, len(m.visiblePanes()), layout.paneRows)
}

func (m Model) currentInstance() string {
	if run, ok := m.selectedRunSnapshot(); ok && run.Instance != "" {
		return run.Instance
	}
	return "default"
}

func (m Model) currentWorkspace() string {
	if run, ok := m.selectedRunSnapshot(); ok && run.Workspace != "" {
		return run.Workspace
	}
	if m.opts.Workspace != "" {
		return m.opts.Workspace
	}
	if m.opts.Home != "" {
		if parent := filepath.Dir(m.opts.Home); filepath.Base(m.opts.Home) == "council-out" {
			return parent
		}
	}
	return ""
}

func (m *Model) reconcileSelections() {
	if m.selectedRun >= len(m.visibleRuns()) {
		m.selectedRun = len(m.visibleRuns()) - 1
	}
	if m.selectedRun < 0 && len(m.visibleRuns()) > 0 {
		m.selectedRun = 0
	}
	m.ensureArtifactSelection()
	m.ensurePaneSelection()
	m.keepRunVisible()
}

func (m *Model) ensureArtifactSelection() {
	artifacts := m.visibleArtifacts()
	if len(artifacts) == 0 {
		m.selectedArtifactIndex = -1
		return
	}
	if m.selectedArtifactIndex < 0 || m.selectedArtifactIndex >= len(artifacts) {
		for i, artifact := range artifacts {
			if artifact.Exists {
				m.selectedArtifactIndex = i
				return
			}
		}
		m.selectedArtifactIndex = 0
	}
}

func (m *Model) ensurePaneSelection() {
	panes := m.visiblePanes()
	if len(panes) == 0 {
		m.selectedPaneIndex = -1
		m.panePreview = nil
		m.panePreviewFor = ""
		return
	}
	if m.selectedPaneIndex < 0 || m.selectedPaneIndex >= len(panes) {
		for i, pane := range panes {
			if pane.Active {
				m.selectedPaneIndex = i
				return
			}
		}
		m.selectedPaneIndex = 0
	}
}

func (m *Model) selectArtifact(index int) {
	artifacts := m.visibleArtifacts()
	if len(artifacts) == 0 {
		m.selectedArtifactIndex = -1
		m.artifactListScroll = 0
		return
	}
	m.selectedArtifactIndex = clamp(index, 0, len(artifacts)-1)
	m.keepRunVisible()
}

func (m *Model) selectPane(index int) {
	panes := m.visiblePanes()
	if len(panes) == 0 {
		m.selectedPaneIndex = -1
		m.paneScroll = 0
		m.panePreview = nil
		m.panePreviewFor = ""
		return
	}
	m.selectedPaneIndex = clamp(index, 0, len(panes)-1)
	m.keepRunVisible()
}

func (m Model) currentPane() *tmux.Pane {
	panes := m.visiblePanes()
	if len(panes) == 0 || m.selectedPaneIndex < 0 || m.selectedPaneIndex >= len(panes) {
		return nil
	}
	return &panes[m.selectedPaneIndex]
}

func (m Model) orchestratorPane() (tmux.Pane, bool) {
	if m.focus == "panes" {
		if pane := m.currentPane(); pane != nil && pane.Role == "orchestrator" {
			return *pane, true
		}
	}

	instance := m.currentInstance()
	workspace := m.currentWorkspace()
	for _, pane := range m.panes {
		if pane.Role == "orchestrator" && pane.Instance == instance {
			if workspace == "" || pane.Workspace == "" || samePath(pane.Workspace, workspace) {
				return pane, true
			}
		}
	}
	for _, pane := range m.panes {
		if pane.Role == "orchestrator" && pane.Instance == instance {
			return pane, true
		}
	}
	return tmux.Pane{}, false
}

func (m Model) currentArtifact() *artifactRow {
	artifacts := m.visibleArtifacts()
	if len(artifacts) == 0 || m.selectedArtifactIndex < 0 || m.selectedArtifactIndex >= len(artifacts) {
		return nil
	}
	return &artifacts[m.selectedArtifactIndex]
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

func (m Model) visiblePanes() []tmux.Pane {
	return m.panes
}

func (m Model) visibleArtifacts() []artifactRow {
	run, ok := m.selectedRunSnapshot()
	if !ok {
		return nil
	}
	var out []artifactRow
	for _, artifact := range artifactsForRun(run, m.opts.Load.MaxReviewRounds) {
		if artifact.Exists || strings.HasPrefix(artifact.Key, "plan") || strings.HasPrefix(artifact.Key, "implementation") || strings.HasPrefix(artifact.Key, "reviews") {
			out = append(out, artifact)
		}
	}
	return out
}

func artifactsForRun(run protocol.Run, maxReviewRounds int) []artifactRow {
	if maxReviewRounds <= 0 {
		maxReviewRounds = protocol.DefaultMaxReviewRounds
	}
	artifact := func(key, label, rel string, exists bool) artifactRow {
		path := filepath.Join(run.Dir, rel)
		var bytes int64
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			bytes = info.Size()
			exists = exists || bytes > 0
		}
		return artifactRow{
			Key:          key,
			Label:        label,
			RelativePath: rel,
			Path:         path,
			Exists:       exists,
			Bytes:        bytes,
		}
	}
	rows := []artifactRow{
		artifact("plans.codex", "codex plan", "plans/codex.md", run.Artifacts.Plans["codex"]),
		artifact("plans.cc", "cc plan", "plans/cc.md", run.Artifacts.Plans["cc"]),
		artifact("plans.amp", "amp plan", "plans/amp.md", run.Artifacts.Plans["amp"]),
		artifact("critiques.codex", "codex critique", "critiques/codex.md", run.Artifacts.Critiques["codex"]),
		artifact("critiques.cc", "cc critique", "critiques/cc.md", run.Artifacts.Critiques["cc"]),
		artifact("critiques.amp", "amp critique", "critiques/amp.md", run.Artifacts.Critiques["amp"]),
		artifact("plan.final", "final plan", "plan.final.md", run.Artifacts.FinalPlan),
		artifact("implementation.codex", "implementation note", "implementation/codex.md", run.Artifacts.Implementation),
	}
	for round := 1; round <= maxReviewRounds; round++ {
		revisionExists := false
		for _, revisionRound := range run.Artifacts.RevisionRounds {
			if revisionRound == round {
				revisionExists = true
				break
			}
		}
		rows = append(rows, artifact(
			fmt.Sprintf("implementation.revise_round_%d", round),
			fmt.Sprintf("revision round %d", round),
			fmt.Sprintf("implementation/codex.revise-round-%d.md", round),
			revisionExists,
		))
		ccExists, ampExists := false, false
		for _, reviewRound := range run.Artifacts.ReviewRounds {
			if reviewRound.Round == round {
				ccExists = reviewRound.CC
				ampExists = reviewRound.AMP
			}
		}
		rows = append(rows,
			artifact(fmt.Sprintf("reviews.cc.round_%d", round), fmt.Sprintf("cc review round %d", round), fmt.Sprintf("reviews/cc.round-%d.md", round), ccExists),
			artifact(fmt.Sprintf("reviews.amp.round_%d", round), fmt.Sprintf("amp review round %d", round), fmt.Sprintf("reviews/amp.round-%d.md", round), ampExists),
		)
	}
	return rows
}

func (m Model) previewLines(limit, width int) []string {
	artifact := m.currentArtifact()
	if artifact == nil || !artifact.Exists {
		return nil
	}
	lines := readArtifactLines(*artifact)
	if len(lines) > limit {
		lines = lines[:limit]
	}
	for i := range lines {
		lines[i] = truncate(lines[i], width)
	}
	return lines
}

func readArtifactLines(artifact artifactRow) []string {
	if !artifact.Exists {
		return []string{"Artifact is pending."}
	}
	data, err := os.ReadFile(artifact.Path)
	if err != nil {
		return []string{err.Error()}
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return []string{emptyStyle.Render("empty artifact")}
	}
	return lines
}

func (m Model) zoomView() string {
	title := "Zoom"
	var lines []string
	switch m.zoomKind {
	case "pane":
		pane := m.currentPane()
		if pane == nil {
			lines = []string{"No pane selected."}
		} else {
			title = "Zoom: " + paneDisplayName(*pane) + "  " + pane.ID
			lines = m.panePreview
			if m.panePreviewErr != "" {
				lines = []string{m.panePreviewErr}
			}
		}
	default:
		artifact := m.currentArtifact()
		if artifact == nil {
			lines = []string{"No artifact selected."}
		} else {
			title = "Zoom: " + artifact.Label + "  " + artifact.Path
			lines = readArtifactLines(*artifact)
		}
	}
	bodyHeight := max(1, m.height-3)
	if m.zoomScroll > max(0, len(lines)-1) {
		m.zoomScroll = max(0, len(lines)-1)
	}
	end := min(len(lines), m.zoomScroll+bodyHeight)
	body := strings.Join(lines[m.zoomScroll:end], "\n")
	header := titleStyle.Render(truncate(title, max(20, m.width-2))) + "\n" + mutedStyle.Render("q/esc/z exits, j/k scroll, click exits")
	return header + "\n" + body
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

func rawStatusText(status string) string {
	if status == "" {
		return "UNKNOWN"
	}
	return strings.ToUpper(status)
}

func nextRunAction(run protocol.Run) string {
	if strings.EqualFold(run.Status, "SUCCESS") || run.NextStage == "complete" {
		return "complete"
	}
	return "council resume " + run.ID
}

func paneDisplayName(pane tmux.Pane) string {
	if pane.Label != "" {
		return pane.Label
	}
	if pane.Window != "" {
		return pane.Window
	}
	return pane.ID
}

func nextFocus(focus string) string {
	switch focus {
	case "runs":
		return "artifacts"
	case "artifacts":
		return "panes"
	default:
		return "runs"
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

func pad(value string, width int) string {
	if lipgloss.Width(value) >= width {
		return value
	}
	return value + strings.Repeat(" ", width-lipgloss.Width(value))
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

func fitStringLines(value string, height int) string {
	if height <= 0 {
		return ""
	}
	lines := strings.Split(strings.TrimRight(value, "\n"), "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
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

func runRowsForHeight(height int) int {
	return max(1, (max(1, height-1))/3)
}

func listRowsForBlock(height int) int {
	return max(1, height-1)
}

func listTitle(label string, focused bool, offset, limit, total int) string {
	title := label
	if focused {
		title += " *"
	}
	if total > limit && limit > 0 {
		title += fmt.Sprintf(" %d-%d/%d", offset+1, min(total, offset+limit), total)
	}
	return title
}

func runListMetrics(height, total int) (visibleRows int, hasRange bool) {
	visibleRows = runRowsForHeight(height)
	hasRange = total > visibleRows
	if hasRange {
		visibleRows = max(1, (max(1, height-2))/3)
	}
	return visibleRows, hasRange
}

func scrollForSelection(scroll, selected, total, visible int) int {
	if total <= 0 || visible <= 0 || total <= visible {
		return 0
	}
	maxScroll := total - visible
	scroll = clamp(scroll, 0, maxScroll)
	if selected < scroll {
		return selected
	}
	if selected >= scroll+visible {
		return min(maxScroll, selected-visible+1)
	}
	return scroll
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
	commandBar = lipgloss.NewStyle().
			Foreground(lipgloss.Color("110")).
			Background(lipgloss.Color("235"))
	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("240"))
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("230"))
	keyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("110"))
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
