package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/XiaoConstantine/council-ui/internal/protocol"
	"github.com/XiaoConstantine/council-ui/internal/ui"
)

func main() {
	var (
		home            string
		workspace       string
		limit           int
		maxReviewRounds int
		refresh         time.Duration
	)

	flag.StringVar(&home, "home", "", "path to council-out")
	flag.StringVar(&workspace, "workspace", "", "workspace containing council-out")
	flag.IntVar(&limit, "limit", 40, "maximum runs to show")
	flag.IntVar(&maxReviewRounds, "max-review-rounds", protocol.DefaultMaxReviewRounds, "maximum council review rounds")
	flag.DurationVar(&refresh, "refresh", time.Second, "refresh interval")
	flag.Parse()

	if home == "" {
		home = protocol.CouncilHome(workspace)
	}

	model := ui.New(ui.Options{
		Home: home,
		Load: protocol.LoadOptions{
			Limit:           limit,
			MaxReviewRounds: maxReviewRounds,
		},
		Refresh: refresh,
	})

	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithContext(context.Background()))
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "council-ui: %v\n", err)
		os.Exit(1)
	}
}
