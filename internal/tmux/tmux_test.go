package tmux

import (
	"context"
	"strings"
	"testing"
)

type fakeRunner struct {
	out   string
	calls []string
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	return []byte(f.out), nil
}

func TestParseCouncilLabel(t *testing.T) {
	role, instance := ParseCouncilLabel("council-codex-feature-a")
	if role != "codex" || instance != "feature-a" {
		t.Fatalf("got %q/%q", role, instance)
	}
	role, instance = ParseCouncilLabel("council-amp")
	if role != "amp" || instance != "default" {
		t.Fatalf("got %q/%q", role, instance)
	}
}

func TestListPanes(t *testing.T) {
	runner := &fakeRunner{out: "%1\ts\t@1\t2\tmaestro-council-feature-a\t80x24\tcodex\tcouncil-codex-feature-a\t/tmp/work\n"}
	client := Client{Runner: runner}

	panes, err := client.ListPanes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(panes) != 1 {
		t.Fatalf("len = %d, want 1", len(panes))
	}
	if panes[0].Role != "codex" || panes[0].Instance != "feature-a" {
		t.Fatalf("pane = %#v", panes[0])
	}
}

func TestSelectPaneUsesWindowThenPane(t *testing.T) {
	runner := &fakeRunner{}
	client := Client{Runner: runner}

	err := client.SelectPane(context.Background(), Pane{ID: "%7", Session: "s", Index: "3"})
	if err != nil {
		t.Fatal(err)
	}

	if len(runner.calls) != 2 {
		t.Fatalf("calls = %#v", runner.calls)
	}
	if !strings.Contains(runner.calls[0], "select-window -t s:3") {
		t.Fatalf("first call = %q", runner.calls[0])
	}
	if !strings.Contains(runner.calls[1], "select-pane -t %7") {
		t.Fatalf("second call = %q", runner.calls[1])
	}
}
