// Command acorn is the Go port of acorn-cli. See ../../README.md.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yumlevi/acorn-cli/go/internal/app"
	"github.com/yumlevi/acorn-cli/go/internal/config"
)

func main() {
	var (
		serverURL = flag.String("server", "", "SPORE server URL (overrides config)")
		sessionID = flag.String("session", "", "resume a specific session id")
		showVer   = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVer {
		fmt.Println("acorn (go port) 0.1.0")
		return
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot read cwd:", err)
		os.Exit(1)
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config load failed:", err)
		os.Exit(1)
	}
	if *serverURL != "" {
		cfg.ServerURL = *serverURL
	}
	if cfg.ServerURL == "" {
		fmt.Fprintln(os.Stderr, "no server url — set `server = \"wss://your-spore/ws\"` in ~/.acorn/config.toml or pass --server")
		os.Exit(1)
	}
	if cfg.TeamKey == "" {
		fmt.Fprintln(os.Stderr, "no team key — set `team_key = \"...\"` in ~/.acorn/config.toml")
		os.Exit(1)
	}

	// Ensure .acorn/plans/ exists in cwd so plan-save doesn't fail later.
	_ = os.MkdirAll(filepath.Join(cwd, ".acorn", "plans"), 0o755)

	m := app.New(cfg, cwd, *sessionID)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}
