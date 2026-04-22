package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// slashHandler is the signature every slash command implements. The
// rest of the args (already split on whitespace) is passed verbatim;
// the original full text is available via the model if needed.
type slashHandler func(m *Model, args []string) (tea.Model, tea.Cmd)

// slashCmd is what /help renders.
type slashCmd struct {
	Name    string
	Aliases []string
	Help    string
	Handler slashHandler
}

// slashRegistry is the full command catalog. Populated in init() so
// tests + main code share the same map and any new command added here
// shows up in /help and the autocomplete dropdown automatically.
var slashRegistry = map[string]*slashCmd{}
var slashOrder []*slashCmd // stable display order for /help

// register adds a command to the registry. Called from init() blocks.
func register(e *slashCmd) {
	slashRegistry[e.Name] = e
	for _, a := range e.Aliases {
		slashRegistry[a] = e
	}
	slashOrder = append(slashOrder, e)
}

// dispatchSlash looks up the leading token, falls back to nil if
// unknown. update.go's handleSlashCommand wraps this to keep the
// existing call shape intact.
func dispatchSlash(m *Model, text string) (tea.Model, tea.Cmd, bool) {
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return m, nil, false
	}
	e, ok := slashRegistry[parts[0]]
	if !ok {
		return m, nil, false
	}
	mm, c := e.Handler(m, parts[1:])
	return mm, c, true
}

// SlashCatalog returns command names in display order — used by the
// autocomplete dropdown.
func SlashCatalog() []string {
	out := make([]string, 0, len(slashOrder))
	for _, e := range slashOrder {
		out = append(out, e.Name)
	}
	return out
}

// SlashHelpFromRegistry renders the /help body straight from the
// registry — guaranteed in sync with what's actually wired.
func SlashHelpFromRegistry() string {
	// Stable sort by name for the help block. Display-order in dropdown
	// keeps insertion order; help block alphabetises so it's scannable.
	names := make([]string, 0, len(slashOrder))
	maxLen := 0
	for _, e := range slashOrder {
		names = append(names, e.Name)
		if l := len(e.Name); l > maxLen {
			maxLen = l
		}
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		e := slashRegistry[n]
		fmt.Fprintf(&b, "%-*s  %s\n", maxLen, n, e.Help)
	}
	return strings.TrimRight(b.String(), "\n")
}

// ── Built-in handlers that don't already live in update.go ──────────
//
// /context — show the gathered project context block + offer to refresh.
// /tree    — print a depth-limited file tree.
// /init    — write ACORN.md template + add .acorn/ to .gitignore.
// /help    — overrides update.go's static /help with the registry view.

func cmdContext(m *Model, args []string) (tea.Model, tea.Cmd) {
	ctx := GatherContext(m.cwd)
	m.pushChat("system", "── Project context ──\n"+ctx)
	if len(args) > 0 && args[0] == "refresh" {
		m.contextSent = false
		m.pushChat("system", "(context will be re-sent on next message)")
	}
	return m, nil
}

func cmdTree(m *Model, args []string) (tea.Model, tea.Cmd) {
	depth := 3
	if len(args) > 0 {
		if d, err := strconv.Atoi(args[0]); err == nil && d > 0 && d < 8 {
			depth = d
		}
	}
	m.pushChat("system", "── Project tree (depth "+strconv.Itoa(depth)+") ──\n"+treeString(m.cwd, depth, 200))
	return m, nil
}

func cmdInit(m *Model, args []string) (tea.Model, tea.Cmd) {
	path := filepath.Join(m.cwd, "ACORN.md")
	if _, err := os.Stat(path); err == nil {
		m.pushChat("system", "ACORN.md already exists at "+path)
		return m, nil
	}
	body := "# Project Instructions for Acorn\n\n" +
		"<!-- Add project-specific context here. Acorn sends this to the agent. -->\n\n" +
		"## Overview\n\n## Conventions\n\n## Important files\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		m.pushChat("system", "Failed to write ACORN.md: "+err.Error())
		return m, nil
	}
	m.pushChat("system", "Created "+path)
	gi := filepath.Join(m.cwd, ".gitignore")
	if _, err := os.Stat(gi); err == nil {
		if cur, err := os.ReadFile(gi); err == nil && !strings.Contains(string(cur), ".acorn/") {
			f, err := os.OpenFile(gi, os.O_WRONLY|os.O_APPEND, 0o644)
			if err == nil {
				_, _ = f.WriteString("\n# Acorn local data\n.acorn/\n")
				_ = f.Close()
				m.pushChat("system", "Added .acorn/ to .gitignore")
			}
		}
	}
	return m, nil
}

func cmdHelp(m *Model, _ []string) (tea.Model, tea.Cmd) {
	m.pushChat("system", SlashHelpFromRegistry())
	return m, nil
}

// treeString — minimal, allocation-light port of acorn/context.py:_tree.
// Skips hidden dirs, common build/cache trees, files over 100 KB.
func treeString(root string, maxDepth, maxEntries int) string {
	skipDirs := map[string]struct{}{
		".git": {}, "node_modules": {}, ".venv": {}, "venv": {},
		"__pycache__": {}, "dist": {}, "build": {}, ".acorn": {},
		"target": {}, ".next": {}, ".cache": {},
	}
	var b strings.Builder
	count := 0
	var walk func(dir, prefix string, depth int)
	walk = func(dir, prefix string, depth int) {
		if depth > maxDepth || count >= maxEntries {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		// Sort: dirs first then files, both alpha.
		sort.SliceStable(entries, func(i, j int) bool {
			a, b := entries[i], entries[j]
			if a.IsDir() != b.IsDir() {
				return a.IsDir()
			}
			return a.Name() < b.Name()
		})
		n := len(entries)
		for i, e := range entries {
			if count >= maxEntries {
				b.WriteString(prefix + "└── …\n")
				return
			}
			name := e.Name()
			if strings.HasPrefix(name, ".") && name != ".env" && name != ".gitignore" {
				continue
			}
			if _, skip := skipDirs[name]; skip {
				continue
			}
			isLast := i == n-1
			branch := "├── "
			nextPrefix := prefix + "│   "
			if isLast {
				branch = "└── "
				nextPrefix = prefix + "    "
			}
			b.WriteString(prefix + branch + name + "\n")
			count++
			if e.IsDir() {
				walk(filepath.Join(dir, name), nextPrefix, depth+1)
			}
		}
	}
	b.WriteString(filepath.Base(root) + "/\n")
	walk(root, "", 1)
	return b.String()
}

func init() {
	register(&slashCmd{
		Name:    "/context",
		Help:    "Show the project context block (add 'refresh' to re-send next turn)",
		Handler: cmdContext,
	})
	register(&slashCmd{
		Name:    "/tree",
		Help:    "Print the project file tree (optional depth, default 3)",
		Handler: cmdTree,
	})
	register(&slashCmd{
		Name:    "/init",
		Help:    "Create ACORN.md template + add .acorn/ to .gitignore",
		Handler: cmdInit,
	})
}
