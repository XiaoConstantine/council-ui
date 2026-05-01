package protocol

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRunInfersPlanComplete(t *testing.T) {
	home := t.TempDir()
	runDir := filepath.Join(home, "runs", "20260501-120000-1")
	write(t, filepath.Join(runDir, "task.txt"), "ship a better monitor")
	write(t, filepath.Join(runDir, "workspace.txt"), "/tmp/work")
	write(t, filepath.Join(runDir, "target.txt"), "plan")
	for _, role := range []string{"codex", "cc", "amp"} {
		write(t, filepath.Join(runDir, "plans", role+".md"), "plan")
		write(t, filepath.Join(runDir, "critiques", role+".md"), "critique")
	}
	write(t, filepath.Join(runDir, "plan.final.md"), "final")

	run, err := LoadRun(runDir, LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if run.NextStage != "complete" {
		t.Fatalf("NextStage = %q, want complete", run.NextStage)
	}
	if run.Phase != "plan-complete" {
		t.Fatalf("Phase = %q, want plan-complete", run.Phase)
	}
}

func TestLoadRunInfersRevisionRound(t *testing.T) {
	home := t.TempDir()
	runDir := filepath.Join(home, "runs", "20260501-120000-2")
	write(t, filepath.Join(runDir, "target.txt"), "complete")
	for _, role := range []string{"codex", "cc", "amp"} {
		write(t, filepath.Join(runDir, "plans", role+".md"), "plan")
		write(t, filepath.Join(runDir, "critiques", role+".md"), "critique")
	}
	write(t, filepath.Join(runDir, "plan.final.md"), "final")
	write(t, filepath.Join(runDir, "implementation", "codex.md"), "implementation")
	write(t, filepath.Join(runDir, "reviews", "cc.round-1.md"), "VERDICT: REVISE\n")
	write(t, filepath.Join(runDir, "reviews", "amp.round-1.md"), "VERDICT: LGTM\n")

	run, err := LoadRun(runDir, LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if run.NextStage != "revise-round-1" {
		t.Fatalf("NextStage = %q, want revise-round-1", run.NextStage)
	}
	if run.Verdicts.CC != "REVISE" || run.Verdicts.AMP != "LGTM" {
		t.Fatalf("verdicts = %#v", run.Verdicts)
	}
}

func write(t *testing.T, path string, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
		t.Fatal(err)
	}
}
