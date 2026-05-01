package protocol

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const DefaultMaxReviewRounds = 2

type Run struct {
	ID          string
	Dir         string
	Task        string
	Workspace   string
	Instance    string
	Status      string
	Phase       string
	Target      string
	Stage       string
	NextStage   string
	UpdatedAt   time.Time
	Progress    []ProgressEvent
	Artifacts   Artifacts
	Verdicts    Verdicts
	Missing     []string
	HasSnapshot bool
}

type ProgressEvent struct {
	Time  string
	Stage string
}

type Artifacts struct {
	Plans          map[string]bool
	Critiques      map[string]bool
	FinalPlan      bool
	Implementation bool
	ReviewRounds   []ReviewRound
	RevisionRounds []int
}

type ReviewRound struct {
	Round int
	CC    bool
	AMP   bool
}

type Verdicts struct {
	CC  string
	AMP string
}

type LoadOptions struct {
	MaxReviewRounds int
	Limit           int
}

func LoadRuns(home string, opts LoadOptions) ([]Run, error) {
	if home == "" {
		return nil, errors.New("council home is empty")
	}
	if opts.MaxReviewRounds <= 0 {
		opts.MaxReviewRounds = DefaultMaxReviewRounds
	}

	runsDir := filepath.Join(home, "runs")
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read runs dir: %w", err)
	}

	runs := make([]Run, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		run, err := LoadRun(filepath.Join(runsDir, entry.Name()), opts)
		if err != nil {
			continue
		}
		runs = append(runs, run)
	}

	sort.Slice(runs, func(i, j int) bool {
		if runs[i].UpdatedAt.Equal(runs[j].UpdatedAt) {
			return runs[i].ID > runs[j].ID
		}
		return runs[i].UpdatedAt.After(runs[j].UpdatedAt)
	})

	if opts.Limit > 0 && len(runs) > opts.Limit {
		runs = runs[:opts.Limit]
	}
	return runs, nil
}

func LoadRun(dir string, opts LoadOptions) (Run, error) {
	if opts.MaxReviewRounds <= 0 {
		opts.MaxReviewRounds = DefaultMaxReviewRounds
	}

	info, err := os.Stat(dir)
	if err != nil {
		return Run{}, err
	}
	if !info.IsDir() {
		return Run{}, fmt.Errorf("%s is not a directory", dir)
	}

	run := Run{
		ID:       filepath.Base(dir),
		Dir:      dir,
		Instance: readText(filepath.Join(dir, "instance.txt"), "default"),
		Status:   readText(filepath.Join(dir, "status.txt"), "-"),
		Target:   readText(filepath.Join(dir, "target.txt"), "complete"),
		Task:     readText(filepath.Join(dir, "task.txt"), ""),
		Workspace: readText(
			filepath.Join(dir, "workspace.txt"),
			"",
		),
		UpdatedAt: latestModTime(dir, info.ModTime()),
	}

	run.Artifacts = scanArtifacts(dir, opts.MaxReviewRounds)
	run.Verdicts = scanVerdicts(dir, opts.MaxReviewRounds)
	run.Progress = readProgress(filepath.Join(dir, "progress.log"))
	run.HasSnapshot = fileNonEmpty(filepath.Join(dir, "workspace.snapshot.txt"))
	run.Stage = readText(filepath.Join(dir, "stage.txt"), "")
	run.NextStage = inferNextStage(dir, run.Target, opts.MaxReviewRounds)
	run.Phase = readText(filepath.Join(dir, "phase.txt"), inferPhase(run))
	run.Missing = missingForNextStage(run)

	return run, nil
}

func CouncilHome(workspace string) string {
	if env := os.Getenv("MAESTRO_COUNCIL_HOME"); env != "" {
		return env
	}
	if workspace == "" {
		workspace = "."
	}
	return filepath.Join(workspace, "council-out")
}

func scanArtifacts(dir string, maxReviewRounds int) Artifacts {
	artifacts := Artifacts{
		Plans:     map[string]bool{},
		Critiques: map[string]bool{},
	}
	for _, role := range []string{"codex", "cc", "amp"} {
		artifacts.Plans[role] = fileNonEmpty(filepath.Join(dir, "plans", role+".md"))
		artifacts.Critiques[role] = fileNonEmpty(filepath.Join(dir, "critiques", role+".md"))
	}
	artifacts.FinalPlan = fileNonEmpty(filepath.Join(dir, "plan.final.md"))
	artifacts.Implementation = fileNonEmpty(filepath.Join(dir, "implementation", "codex.md"))

	for round := 1; round <= maxReviewRounds; round++ {
		artifacts.ReviewRounds = append(artifacts.ReviewRounds, ReviewRound{
			Round: round,
			CC:    fileNonEmpty(filepath.Join(dir, "reviews", fmt.Sprintf("cc.round-%d.md", round))),
			AMP:   fileNonEmpty(filepath.Join(dir, "reviews", fmt.Sprintf("amp.round-%d.md", round))),
		})
		if fileNonEmpty(filepath.Join(dir, "implementation", fmt.Sprintf("codex.revise-round-%d.md", round))) {
			artifacts.RevisionRounds = append(artifacts.RevisionRounds, round)
		}
	}
	return artifacts
}

func scanVerdicts(dir string, maxReviewRounds int) Verdicts {
	var verdicts Verdicts
	for round := maxReviewRounds; round >= 1; round-- {
		cc := reviewVerdict(filepath.Join(dir, "reviews", fmt.Sprintf("cc.round-%d.md", round)))
		amp := reviewVerdict(filepath.Join(dir, "reviews", fmt.Sprintf("amp.round-%d.md", round)))
		if cc != "" || amp != "" {
			verdicts.CC = cc
			verdicts.AMP = amp
			return verdicts
		}
	}
	return verdicts
}

func inferNextStage(dir, target string, maxReviewRounds int) string {
	if !allNonEmpty(filepath.Join(dir, "plans", "codex.md"), filepath.Join(dir, "plans", "cc.md"), filepath.Join(dir, "plans", "amp.md")) {
		return "plans"
	}
	if !allNonEmpty(filepath.Join(dir, "critiques", "codex.md"), filepath.Join(dir, "critiques", "cc.md"), filepath.Join(dir, "critiques", "amp.md")) {
		return "critiques"
	}
	if !fileNonEmpty(filepath.Join(dir, "plan.final.md")) {
		return "final-plan"
	}
	if target == "plan" {
		return "complete"
	}
	if !fileNonEmpty(filepath.Join(dir, "implementation", "codex.md")) {
		return "implementation"
	}

	for round := 1; round <= maxReviewRounds; round++ {
		ccReview := filepath.Join(dir, "reviews", fmt.Sprintf("cc.round-%d.md", round))
		ampReview := filepath.Join(dir, "reviews", fmt.Sprintf("amp.round-%d.md", round))
		if !allNonEmpty(ccReview, ampReview) {
			return fmt.Sprintf("reviews-round-%d", round)
		}

		ccVerdict := reviewVerdict(ccReview)
		ampVerdict := reviewVerdict(ampReview)
		if ccVerdict == "LGTM" && ampVerdict == "LGTM" {
			return "complete"
		}
		if round >= maxReviewRounds {
			return "complete"
		}
		if !fileNonEmpty(filepath.Join(dir, "implementation", fmt.Sprintf("codex.revise-round-%d.md", round))) {
			return fmt.Sprintf("revise-round-%d", round)
		}
	}

	return "complete"
}

func inferPhase(run Run) string {
	if run.Artifacts.Implementation {
		if run.NextStage == "complete" {
			return "complete"
		}
		return "execution"
	}
	if run.Artifacts.FinalPlan {
		return "plan-complete"
	}
	return "planning"
}

func missingForNextStage(run Run) []string {
	var missing []string
	switch run.NextStage {
	case "plans":
		for _, role := range []string{"codex", "cc", "amp"} {
			if !run.Artifacts.Plans[role] {
				missing = append(missing, "plans/"+role+".md")
			}
		}
	case "critiques":
		for _, role := range []string{"codex", "cc", "amp"} {
			if !run.Artifacts.Critiques[role] {
				missing = append(missing, "critiques/"+role+".md")
			}
		}
	case "final-plan":
		missing = append(missing, "plan.final.md")
	case "implementation":
		missing = append(missing, "implementation/codex.md")
	default:
		if strings.HasPrefix(run.NextStage, "reviews-round-") {
			round := strings.TrimPrefix(run.NextStage, "reviews-round-")
			missing = append(missing, "reviews/cc.round-"+round+".md", "reviews/amp.round-"+round+".md")
		}
		if strings.HasPrefix(run.NextStage, "revise-round-") {
			round := strings.TrimPrefix(run.NextStage, "revise-round-")
			missing = append(missing, "implementation/codex.revise-round-"+round+".md")
		}
	}
	return missing
}

func readProgress(path string) []ProgressEvent {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	var events []ProgressEvent
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		stage := strings.TrimPrefix(parts[1], "phase:")
		events = append(events, ProgressEvent{Time: parts[0], Stage: stage})
	}
	return events
}

func reviewVerdict(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.ToUpper(strings.TrimSpace(scanner.Text()))
		if strings.HasPrefix(line, "VERDICT:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return fields[1]
			}
		}
	}
	return ""
}

func latestModTime(dir string, fallback time.Time) time.Time {
	latest := fallback
	_ = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err == nil && info.ModTime().After(latest) {
			latest = info.ModTime()
		}
		return nil
	})
	return latest
}

func readText(path, fallback string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return fallback
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return fallback
	}
	return value
}

func allNonEmpty(paths ...string) bool {
	for _, path := range paths {
		if !fileNonEmpty(path) {
			return false
		}
	}
	return true
}

func fileNonEmpty(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}
