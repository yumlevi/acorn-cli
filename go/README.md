# acorn — Go port

Go / Bubble Tea rewrite of the Python acorn CLI. Distribution is a single
static binary per OS/arch — no Python, no Node, no runtime dependencies.

## Why

The Python implementation uses Textual, which glitches on older Windows
terminals and requires a working Python install + pip/pipx setup. This
port targets:

- **Zero-install**: download one binary, put it in `$PATH`, run from any
  directory.
- **Robust Windows**: Bubble Tea uses the Windows virtual-terminal API
  directly and handles cmd.exe fallback gracefully.
- **Fast startup**: compiled Go boots in tens of ms vs Python's ~1s.

## Build

Requires Go 1.22+.

```sh
cd go
go mod tidy
make build          # ./acorn
make install        # ~/.local/bin/acorn
make release        # cross-compile for all OS/arch into dist/
```

## Install (once binaries are published)

```sh
# Linux/mac
curl -sSL https://github.com/yumlevi/acorn-cli/releases/latest/download/acorn-$(uname -s | tr A-Z a-z)-$(uname -m) \
  -o ~/.local/bin/acorn && chmod +x ~/.local/bin/acorn

# Windows (PowerShell, Scoop/winget later)
iwr -Uri https://.../acorn-windows-amd64.exe -OutFile acorn.exe
```

## Configure

Global config at `~/.acorn/config.toml`:

```toml
server = "wss://spore.hyrule.vip/ws"
team_key = "<your-acorn-team-key>"
user = "yam"
theme = "default"
```

Per-project overrides in `./.acorn/config.toml` — same shape. Project
settings win.

## Launch

Run `acorn` from any project directory. The CLI:

1. Loads `~/.acorn/config.toml` + `./.acorn/config.toml`.
2. Opens a WebSocket to the configured server with your team key.
3. Creates `./.acorn/plans/` if needed so plan-save can't fail silently.
4. Opens the TUI.

## Keybindings

| Key         | Action |
|-------------|--------|
| Enter       | Send message |
| Alt+Enter   | Newline in input (multi-line drafting) |
| Shift+Tab   | Toggle plan / execute mode |
| PgUp/PgDn   | Scroll chat history |
| Esc         | Close modal |
| Ctrl+C      | Quit |

## Slash commands

| Command                | What it does |
|------------------------|--------------|
| `/help`                | Show the list |
| `/new`                 | Fresh session in current cwd |
| `/clear`               | Clear local view (server history unchanged) |
| `/resume <sessionId>`  | Resume a specific session |
| `/quit`                | Exit |

## Feature parity vs Python acorn

Ported (MVP):

- Connection + authentication
- Chat send/receive, streaming deltas
- Status bar with tool activity
- Plan mode toggle + plan approval modal + plan-save to `.acorn/plans/`
- Prose-based `QUESTIONS:` parser + picker (parity with `acorn/questions.py`)
- Structured `ask_user` tool modal (new SPORE server capability)
- Session management: `/new`, `/clear`, `/resume`

Not yet ported (stubs, contributions welcome):

- Companion-app observer protocol
- Local session log (parity with `~/.acorn/logs/`)
- Permission prompts for dangerous tool calls (per-call allow/deny modal)
- Delegation policies (`delegate:config` UI)
- Code-viewer panel for `read_file` / `edit_file` output
- Subagent activity panel

## Layout

```
go/
  cmd/acorn/main.go           # entry point
  internal/
    config/config.go          # TOML config loader
    conn/ws.go                # WebSocket client + read/write loops
    proto/messages.go         # server protocol types
    app/
      model.go                # Bubble Tea Model struct + init
      update.go               # Update() — keystrokes + inbound frames
      view.go                 # View() — chat rendering, header, footer
      questions.go            # QUESTIONS: parser + picker modal
      plan.go                 # Plan approval modal + save
```
