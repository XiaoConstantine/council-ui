package ui

import (
	"path/filepath"
	"testing"

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
