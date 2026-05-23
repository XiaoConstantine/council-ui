package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/XiaoConstantine/council-ui/internal/protocol"
)

func TestArtifactPreviewPathUsesCompletedArtifacts(t *testing.T) {
	run := protocol.Run{
		Dir: "/tmp/run",
		Artifacts: protocol.Artifacts{
			Plans:          map[string]bool{"codex": true, "cc": true, "amp": true},
			Critiques:      map[string]bool{"codex": true, "cc": true, "amp": true},
			FinalPlan:      true,
			Implementation: true,
			ReviewRounds: []protocol.ReviewRound{
				{Round: 1, CC: true, AMP: true},
			},
		},
	}

	path, label := artifactPreviewPath(run, "codex")
	if path != filepath.Join("/tmp/run", "implementation", "codex.md") || label != "implementation/codex.md" {
		t.Fatalf("codex path=%q label=%q", path, label)
	}

	path, label = artifactPreviewPath(run, "cc")
	if path != filepath.Join("/tmp/run", "reviews", "cc.round-1.md") || label != "reviews/cc.round-1.md" {
		t.Fatalf("cc path=%q label=%q", path, label)
	}
}

func TestSectionArtifactPreviewPathFollowsSelectedSection(t *testing.T) {
	run := protocol.Run{
		Dir: "/tmp/run",
		Artifacts: protocol.Artifacts{
			Plans:          map[string]bool{"codex": true, "cc": true, "amp": true},
			Critiques:      map[string]bool{"codex": true, "cc": true, "amp": true},
			FinalPlan:      true,
			Implementation: true,
			ReviewRounds: []protocol.ReviewRound{
				{Round: 1, CC: true, AMP: true},
			},
		},
	}

	model := New(Options{Home: "/tmp"})
	model.selectedSection = 0
	path, label := model.artifactPreviewPath(run)
	if path != filepath.Join("/tmp/run", "plan.final.md") || label != "plan.final.md" {
		t.Fatalf("plan path=%q label=%q", path, label)
	}

	model.selectedSection = 1
	path, label = model.artifactPreviewPath(run)
	if path != filepath.Join("/tmp/run", "implementation", "codex.md") || label != "implementation/codex.md" {
		t.Fatalf("execution path=%q label=%q", path, label)
	}

	model.selectedSection = 2
	model.selectedAgent = 2
	path, label = model.artifactPreviewPath(run)
	if path != filepath.Join("/tmp/run", "reviews", "amp.round-1.md") || label != "reviews/amp.round-1.md" {
		t.Fatalf("review path=%q label=%q", path, label)
	}
}

func TestSectionStatusSummarizesRunAreas(t *testing.T) {
	run := protocol.Run{
		Artifacts: protocol.Artifacts{
			Plans:     map[string]bool{"codex": true, "cc": true, "amp": false},
			Critiques: map[string]bool{"codex": true, "cc": false, "amp": false},
			FinalPlan: false,
		},
	}

	got := sectionStatus(run, "plan")
	if got != "2/3 plans, 1/3 critiques, final no" {
		t.Fatalf("status = %q", got)
	}
}

func TestHeadMeaningfulLinesCompactsLeadingAndRepeatedBlanks(t *testing.T) {
	lines := headMeaningfulLines("\n\none\n\n\ntwo\nthree\n", 4)
	want := []string{"one", "", "two", "three"}
	if len(lines) != len(want) {
		t.Fatalf("lines=%#v", lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("lines[%d]=%q, want %q", i, lines[i], want[i])
		}
	}
}

func TestScrollLinesClampsOffset(t *testing.T) {
	lines := []string{"a", "b", "c", "d", "e"}
	view, offset, total := scrollLines(lines, 99, 2)
	if total != 5 || offset != 3 {
		t.Fatalf("offset=%d total=%d", offset, total)
	}
	if len(view) != 2 || view[0] != "d" || view[1] != "e" {
		t.Fatalf("view=%#v", view)
	}
}

func TestRenderSectionTabsIncludesVisibleLabels(t *testing.T) {
	model := New(Options{Home: "/tmp"})
	model.selectedSection = 1
	tabs := model.renderSectionTabs(120)
	for _, label := range []string{"Plan", "Execution", "Reviews", "Progress"} {
		if !strings.Contains(tabs, label) {
			t.Fatalf("tabs %q missing %q", tabs, label)
		}
	}
}

func TestArtifactFallbackDoesNotDuplicateAgentTabs(t *testing.T) {
	model := New(Options{Home: "/tmp"})
	model.runs = []protocol.Run{{ID: "run-1", Dir: "/tmp/run-1"}}

	view := model.renderPreview(120, 20)
	if got := strings.Count(view, "agent"); got != 2 {
		t.Fatalf("agent label count = %d, want 2\n%s", got, view)
	}
}

func TestSelectedArtifactReadsMeaningfulLines(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "implementation"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "implementation", "codex.md")
	if err := os.WriteFile(path, []byte("\n# Title\n\n\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	model := New(Options{Home: "/tmp"})
	model.selectedSection = 1
	doc := model.selectedArtifact(protocol.Run{
		Dir: dir,
		Artifacts: protocol.Artifacts{
			Implementation: true,
		},
	})

	if doc.Err != nil {
		t.Fatalf("doc err = %v", doc.Err)
	}
	if doc.Path != path || doc.Label != "implementation/codex.md" {
		t.Fatalf("path=%q label=%q", doc.Path, doc.Label)
	}
	want := []string{"# Title", "", "body"}
	if strings.Join(doc.Lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("lines=%#v", doc.Lines)
	}
}

func TestArtifactModalScrollsAndCopiesOffsetOnClose(t *testing.T) {
	model := New(Options{Home: "/tmp"})
	model.artifactModal = true
	model.height = 30

	next, _ := model.updateArtifactModal(tea.KeyMsg{Type: tea.KeyCtrlD})
	updated := next.(Model)
	if updated.modalScroll != 15 {
		t.Fatalf("modalScroll after ctrl+d = %d", updated.modalScroll)
	}

	next, _ = updated.updateArtifactModal(tea.KeyMsg{Type: tea.KeyEsc})
	closed := next.(Model)
	if closed.artifactModal {
		t.Fatal("modal remained open")
	}
	if closed.artifactScroll != 15 {
		t.Fatalf("artifactScroll after close = %d", closed.artifactScroll)
	}
}

func TestCommandButtonAtMapsVisibleButtons(t *testing.T) {
	model := New(Options{Home: "/tmp"})

	if got := model.commandButtonAt(len("Actions ")); got != "start" {
		t.Fatalf("first action = %q, want start", got)
	}

	zoomX := len("Actions [Start] [Attach] [Resume] [Exec] ")
	if got := model.commandButtonAt(zoomX); got != "zoom" {
		t.Fatalf("zoom action = %q, want zoom", got)
	}

	if got := model.commandButtonAt(0); got != "" {
		t.Fatalf("prefix action = %q, want empty", got)
	}
}

func TestCouncilCommandForActionUsesSelectedRun(t *testing.T) {
	model := New(Options{Home: "/tmp/project/council-out", Workspace: "/tmp/project"})
	model.runs = []protocol.Run{{
		ID:        "20260522-120000-1",
		Workspace: "/work",
		Instance:  "blue",
	}}

	cmd, ok := model.councilCommandForAction("resume")
	if !ok {
		t.Fatal("resume command missing")
	}
	if cmd.Workspace != "/work" {
		t.Fatalf("workspace = %q, want /work", cmd.Workspace)
	}
	if strings.Join(cmd.Args, " ") != "resume 20260522-120000-1" {
		t.Fatalf("args = %#v", cmd.Args)
	}

	cmd = model.councilRunCommand("ship feature")
	if cmd.Workspace != "/work" {
		t.Fatalf("run workspace = %q, want /work", cmd.Workspace)
	}
	if strings.Join(cmd.Args, " ") != "run --instance blue -- ship feature" {
		t.Fatalf("run args = %#v", cmd.Args)
	}
}

func TestRunCommandWorksWithoutRuns(t *testing.T) {
	model := New(Options{Home: "/tmp/fresh/council-out", Workspace: "/tmp/fresh"})

	cmd := model.councilRunCommand("build dashboard")
	if cmd.Workspace != "/tmp/fresh" {
		t.Fatalf("workspace = %q, want /tmp/fresh", cmd.Workspace)
	}
	if strings.Join(cmd.Args, " ") != "run --instance default -- build dashboard" {
		t.Fatalf("args = %#v", cmd.Args)
	}
}

func TestStartActionPromptsForGoal(t *testing.T) {
	next, cmd := New(Options{Home: "/tmp"}).triggerAction("start")
	model := next.(Model)

	if !model.enteringGoal {
		t.Fatal("start should enter goal input mode")
	}
	if cmd != nil {
		t.Fatal("start should not run before a goal is entered")
	}
}

func TestGoalInputRunsCouncilRun(t *testing.T) {
	model := New(Options{Home: "/tmp/fresh/council-out", Workspace: "/tmp/fresh"})
	model.enteringGoal = true
	model.goalInput = "ship ui"

	next, cmd := model.updateGoalInput(tea.KeyMsg{Type: tea.KeyEnter})
	updated := next.(Model)

	if updated.enteringGoal {
		t.Fatal("goal input mode should close after enter")
	}
	if cmd == nil {
		t.Fatal("entering a goal should run a command")
	}
}

func TestResetActionRequiresConfirmation(t *testing.T) {
	next, cmd := New(Options{Home: "/tmp"}).triggerAction("reset")
	model := next.(Model)

	if !model.confirmReset {
		t.Fatal("reset should set confirmReset")
	}
	if cmd != nil {
		t.Fatal("reset should not run a command before confirmation")
	}
}

func TestSelectRunKeepsSelectionVisible(t *testing.T) {
	model := New(Options{Home: "/tmp"})
	model.width = 100
	model.height = 18
	for i := 0; i < 10; i++ {
		model.runs = append(model.runs, protocol.Run{ID: string(rune('a' + i))})
	}

	model.selectRun(9)
	visibleRows, _ := runListMetrics(max(8, model.height-lipgloss.Height(model.renderTop())-lipgloss.Height(model.renderBottom())), len(model.runs))

	if model.runScroll != len(model.runs)-visibleRows {
		t.Fatalf("runScroll = %d, want %d", model.runScroll, len(model.runs)-visibleRows)
	}
}

func TestMouseClickSelectsRenderedRun(t *testing.T) {
	model := New(Options{Home: "/tmp"})
	model.width = 120
	model.height = 30
	model.runs = []protocol.Run{
		{ID: "first"},
		{ID: "second"},
	}

	next, _ := model.updateMouse(tea.MouseMsg{
		X:      2,
		Y:      model.layout().runFirstRowY + 1,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	got := next.(Model)

	if got.selectedRun != 1 {
		t.Fatalf("selectedRun = %d, want 1", got.selectedRun)
	}
}
