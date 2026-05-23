package protocol

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Project struct {
	Name      string
	Workspace string
	Home      string
	Runs      int
	UpdatedAt time.Time
}

func DefaultProjectRoots() []string {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}

	roots := []string{cwd}
	if parent := filepath.Dir(cwd); parent != cwd {
		roots = append(roots, parent)
	}

	if env := os.Getenv("COUNCIL_UI_PROJECT_ROOTS"); env != "" {
		roots = append(roots, filepath.SplitList(env)...)
	}

	return uniqueAbsPaths(roots)
}

func DiscoverProjects(roots []string) ([]Project, error) {
	if len(roots) == 0 {
		roots = DefaultProjectRoots()
	}

	seen := map[string]bool{}
	var projects []Project
	var errs []error

	for _, root := range uniqueAbsPaths(roots) {
		rootProjects, err := discoverProjectsInRoot(root)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, project := range rootProjects {
			if seen[project.Workspace] {
				continue
			}
			seen[project.Workspace] = true
			projects = append(projects, project)
		}
	}

	sort.Slice(projects, func(i, j int) bool {
		if projects[i].UpdatedAt.Equal(projects[j].UpdatedAt) {
			return projects[i].Name < projects[j].Name
		}
		return projects[i].UpdatedAt.After(projects[j].UpdatedAt)
	})

	return projects, errors.Join(errs...)
}

func discoverProjectsInRoot(root string) ([]Project, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}

	var projects []Project
	if project, ok := projectForWorkspace(root); ok {
		projects = append(projects, project)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return projects, err
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		workspace := filepath.Join(root, entry.Name())
		if project, ok := projectForWorkspace(workspace); ok {
			projects = append(projects, project)
		}
	}

	return projects, nil
}

func projectForWorkspace(workspace string) (Project, bool) {
	workspace, _ = filepath.Abs(workspace)
	workspaceInfo, err := os.Stat(workspace)
	if err != nil || !workspaceInfo.IsDir() {
		return Project{}, false
	}

	home := CouncilHomeNoEnv(workspace)
	runsDir := filepath.Join(home, "runs")
	var runCount int
	var updatedAt time.Time
	if entries, err := os.ReadDir(runsDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			runCount++
			info, err := entry.Info()
			if err == nil && info.ModTime().After(updatedAt) {
				updatedAt = info.ModTime()
			}
		}
	}

	if runCount == 0 && !looksLikeProjectWorkspace(workspace) {
		return Project{}, false
	}
	if updatedAt.IsZero() {
		updatedAt = latestProjectMarkerTime(workspace, workspaceInfo.ModTime())
	}

	return Project{
		Name:      filepath.Base(workspace),
		Workspace: workspace,
		Home:      home,
		Runs:      runCount,
		UpdatedAt: updatedAt,
	}, true
}

func looksLikeProjectWorkspace(workspace string) bool {
	for _, marker := range projectMarkers() {
		if _, err := os.Stat(filepath.Join(workspace, marker)); err == nil {
			return true
		}
	}
	return false
}

func latestProjectMarkerTime(workspace string, fallback time.Time) time.Time {
	latest := fallback
	for _, marker := range projectMarkers() {
		info, err := os.Stat(filepath.Join(workspace, marker))
		if err == nil && info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}
	return latest
}

func projectMarkers() []string {
	return []string{
		".git",
		".maestro-council.conf",
		"go.mod",
		"package.json",
		"pyproject.toml",
		"Cargo.toml",
		"mix.exs",
		"pom.xml",
		"build.gradle",
	}
}

func CouncilHomeNoEnv(workspace string) string {
	if workspace == "" {
		workspace = "."
	}
	return filepath.Join(workspace, "council-out")
}

func uniqueAbsPaths(paths []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, path := range paths {
		if path == "" {
			continue
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			abs = path
		}
		if seen[abs] {
			continue
		}
		seen[abs] = true
		out = append(out, abs)
	}
	return out
}
