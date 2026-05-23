package protocol

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverProjectsFindsImmediateChildren(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "alpha", "council-out", "runs", "run-1", "status.txt"), "FAILED")
	write(t, filepath.Join(root, "beta", "README.md"), "no council here")

	projects, err := DiscoverProjects([]string{root})
	if err != nil {
		t.Fatal(err)
	}

	if len(projects) != 1 {
		t.Fatalf("len = %d, want 1", len(projects))
	}
	if projects[0].Name != "alpha" || projects[0].Runs != 1 {
		t.Fatalf("project = %#v", projects[0])
	}
}

func TestDiscoverProjectsIncludesFreshProjectWithoutCouncilOut(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "fresh", "go.mod"), "module example.com/fresh\n")

	projects, err := DiscoverProjects([]string{root})
	if err != nil {
		t.Fatal(err)
	}

	if len(projects) != 1 {
		t.Fatalf("len = %d, want 1", len(projects))
	}
	if projects[0].Name != "fresh" || projects[0].Runs != 0 {
		t.Fatalf("project = %#v", projects[0])
	}
	if projects[0].Home != filepath.Join(root, "fresh", "council-out") {
		t.Fatalf("home = %q", projects[0].Home)
	}
}

func TestDiscoverProjectsFindsRootWorkspace(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "council-out", "runs", "run-1", "status.txt"), "SUCCESS")

	projects, err := DiscoverProjects([]string{root})
	if err != nil {
		t.Fatal(err)
	}

	if len(projects) != 1 {
		t.Fatalf("len = %d, want 1", len(projects))
	}
	if projects[0].Workspace != root {
		t.Fatalf("workspace = %q, want %q", projects[0].Workspace, root)
	}
}

func TestCouncilHomeUsesEnvButProjectDiscoveryDoesNot(t *testing.T) {
	t.Setenv("MAESTRO_COUNCIL_HOME", "/tmp/custom")
	if got := CouncilHome("/workspace"); got != "/tmp/custom" {
		t.Fatalf("CouncilHome = %q", got)
	}
	if got := CouncilHomeNoEnv("/workspace"); got != filepath.Join("/workspace", "council-out") {
		t.Fatalf("CouncilHomeNoEnv = %q", got)
	}
}

func TestUniqueAbsPathsDropsEmptyAndDuplicates(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	paths := uniqueAbsPaths([]string{"", ".", cwd})
	if len(paths) != 1 {
		t.Fatalf("paths = %#v", paths)
	}
}
